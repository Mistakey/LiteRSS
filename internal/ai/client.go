// Package ai provides universal AI client with automatic format detection
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"LiteRSS/internal/utils/httputil"
)

// ClientConfig holds the configuration for the AI client
type ClientConfig struct {
	APIKey        string
	Endpoint      string
	Model         string
	SystemPrompt  string
	CustomHeaders string
	Timeout       time.Duration
}

// Client represents a universal AI client that supports multiple API formats
type Client struct {
	config ClientConfig
	client *http.Client
}

// NewClient creates a new universal AI client
func NewClient(config ClientConfig) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		config: config,
		client: httputil.CreateHTTPClient(config.Timeout),
	}
}

// NewClientWithHTTPClient creates a new AI client with a custom HTTP client
func NewClientWithHTTPClient(config ClientConfig, httpClient *http.Client) *Client {
	return &Client{
		config: config,
		client: httpClient,
	}
}

// Request makes an AI request with automatic format detection and fallback
func (c *Client) Request(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	result, err := c.RequestWithThinking(ctx, systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// RequestWithThinking makes an AI request and returns both content and thinking
func (c *Client) RequestWithThinking(ctx context.Context, systemPrompt, userPrompt string) (ResponseResult, error) {
	config := RequestConfig{
		Model:        c.config.Model,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.3,
		MaxTokens:    2048,
	}

	return c.RequestWithConfig(ctx, config)
}

// RequestWithMessages makes an AI request using messages format
func (c *Client) RequestWithMessages(ctx context.Context, messages []map[string]string) (ResponseResult, error) {
	config := RequestConfig{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: 0.3,
		MaxTokens:   2048,
	}

	return c.RequestWithConfig(ctx, config)
}

// RequestWithConfig makes an AI request with full configuration
func (c *Client) RequestWithConfig(ctx context.Context, config RequestConfig) (ResponseResult, error) {
	// Try provider-specific format first based on endpoint detection
	if isDeepSeekEndpoint(c.config.Endpoint) {
		result, err := c.tryFormat(ctx, &DeepSeekHandler{}, config)
		if err == nil {
			return result, nil
		}
		// Fall through to the OpenAI-compatible format
	}

	// Try OpenAI format (most common, good fallback)
	result, err := c.tryFormat(ctx, NewOpenAIHandler(), config)
	if err == nil {
		return result, nil
	}

	// All formats failed
	return ResponseResult{}, fmt.Errorf("all API formats failed: %w", err)
}

// isDeepSeekEndpoint reports whether the endpoint URL should be tried with the
// DeepSeek request format. Every other endpoint is served by the
// OpenAI-compatible format, so DeepSeek is the only one worth detecting.
func isDeepSeekEndpoint(endpoint string) bool {
	return strings.Contains(strings.ToLower(endpoint), "deepseek")
}

// tryFormat attempts to make a request using a specific format handler
func (c *Client) tryFormat(ctx context.Context, handler FormatHandler, config RequestConfig) (ResponseResult, error) {
	// Build request body
	requestBody, err := handler.BuildRequest(config)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("failed to build request: %w", err)
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Format endpoint
	formattedEndpoint := handler.FormatEndpoint(c.config.Endpoint, c.config.Model)

	// Send request with formatted endpoint and handler
	resp, err := c.sendRequestToEndpointWithHandler(ctx, jsonBody, formattedEndpoint, handler)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Validate response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("failed to read response body: %w", err)
	}
	if err := handler.ValidateResponse(resp.StatusCode, bodyBytes); err != nil {
		return ResponseResult{}, err
	}

	// Parse response
	result, err := handler.ParseResponse(bodyBytes)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// sendRequestToEndpointWithHandler sends the HTTP request to a specific endpoint with handler-specific headers
func (c *Client) sendRequestToEndpointWithHandler(ctx context.Context, jsonBody []byte, apiURL string, handler FormatHandler) (*http.Response, error) {
	// Validate endpoint URL to prevent SSRF attacks. The URL stays out of the
	// error: an endpoint may carry a key in its query, and errors are logged.
	parsedURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, errors.New("invalid API endpoint URL")
	}

	// Both HTTP and HTTPS are allowed
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("API endpoint must use HTTP or HTTPS")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Check if handler provides custom headers
	type HeaderProvider interface {
		GetRequiredHeaders(apiKey string) map[string]string
	}

	if handler != nil {
		if hp, ok := handler.(HeaderProvider); ok {
			// Use handler-specific headers
			requiredHeaders := hp.GetRequiredHeaders(c.config.APIKey)
			for key, value := range requiredHeaders {
				req.Header.Set(key, value)
			}
		} else {
			// Use default headers
			req.Header.Set("Content-Type", "application/json")
			// Only add Authorization header if API key is provided
			if c.config.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
			}
		}
	} else {
		// No handler provided, use default headers
		req.Header.Set("Content-Type", "application/json")
		// Only add Authorization header if API key is provided
		if c.config.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}
	}

	// Parse and add custom headers if provided
	if c.config.CustomHeaders != "" {
		customHeaders, err := parseCustomHeaders(c.config.CustomHeaders)
		if err != nil {
			return nil, fmt.Errorf("failed to parse custom headers: %w", err)
		}
		// Apply custom headers
		for key, value := range customHeaders {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.client.Do(req)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		redacted := *parsedURL
		redacted.RawQuery, redacted.User = "", nil
		urlErr.URL = redacted.String()
	}
	return resp, err
}

// parseCustomHeaders parses the JSON string of custom headers into a map
func parseCustomHeaders(headersJSON string) (map[string]string, error) {
	// Return empty map if headers string is empty
	if headersJSON == "" {
		return make(map[string]string), nil
	}

	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return nil, fmt.Errorf("failed to parse custom headers JSON: %w", err)
	}
	return headers, nil
}

// ExtractThinking extracts thinking content from <thinking> tags (case-insensitive)
func ExtractThinking(content string) string {
	tagVariations := []struct {
		start string
		end   string
	}{
		{"<thinking>", "</thinking>"},
		{"<THINKING>", "</THINKING>"},
		{"<Thinking>", "</Thinking>"},
		{"<think>", "</think>"},
		{"<THINK>", "</THINK>"},
		{"<Think>", "</Think>"},
	}

	for _, tags := range tagVariations {
		startIndex := strings.Index(content, tags.start)
		if startIndex == -1 {
			continue
		}

		endIndex := strings.Index(content[startIndex:], tags.end)
		if endIndex == -1 {
			continue
		}

		// Extract the content between tags (excluding tags themselves)
		thinkingStart := startIndex + len(tags.start)
		thinkingEnd := startIndex + endIndex
		thinking := strings.TrimSpace(content[thinkingStart:thinkingEnd])

		return thinking
	}

	return ""
}

// RemoveThinkingTags removes <thinking> tags and their content from the response (case-insensitive)
func RemoveThinkingTags(content string) string {
	tagVariations := []struct {
		start string
		end   string
	}{
		{"<thinking>", "</thinking>"},
		{"<THINKING>", "</THINKING>"},
		{"<Thinking>", "</Thinking>"},
		{"<think>", "</think>"},
		{"<THINK>", "</THINK>"},
		{"<Think>", "</Think>"},
	}

	result := content
	for _, tags := range tagVariations {
		for {
			startIndex := strings.Index(result, tags.start)
			if startIndex == -1 {
				break
			}

			endIndex := strings.Index(result[startIndex:], tags.end)
			if endIndex == -1 {
				break
			}

			// Remove the entire thinking block including tags
			thinkingEnd := startIndex + endIndex + len(tags.end)
			result = result[:startIndex] + result[thinkingEnd:]
		}
	}

	return strings.TrimSpace(result)
}
