package freshrss

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stream ids of the Google Reader API.
const (
	StreamReadingList = "user/-/state/com.google/reading-list"
	StreamRead        = "user/-/state/com.google/read"
	StreamStarred     = "user/-/state/com.google/starred"
	LabelPrefix       = "user/-/label/"
)

const longItemIDPrefix = "tag:google.com,2005:reader/item/"

// Client talks to one FreshRSS account. It logs in on first use, keeps the
// session and write token, and on a 401 logs in again and retries once, so
// callers never manage the session themselves. Safe for concurrent use.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client

	mu         sync.Mutex
	authToken  string
	writeToken string
	// FreshRSS's placeholder favicon, once fetched (icon.go).
	placeholderKnown bool
	placeholder      []byte
}

// NewClient creates a client; it does not contact the server.
func NewClient(serverURL, username, password string) *Client {
	// Ensure URL ends with /api/greader.php
	if !strings.HasSuffix(serverURL, "/api/greader.php") {
		serverURL = strings.TrimSuffix(serverURL, "/") + "/api/greader.php"
	}

	// Deliberately not httputil.CreateHTTPClient: FreshRSS is a LAN server, and
	// the shared client would hand it to the system proxy (go-ieproxy cannot read
	// the wildcard bypass entries Windows writes for private ranges). Sync would
	// then only work as long as the proxy tool routes private ranges direct.
	// See docs/pitfalls.md, pitfall 9.
	return &Client{
		baseURL:  serverURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
}

// Clients hands out one Client per configuration, so every caller shares a
// single logged-in session instead of logging in per operation. A changed
// configuration replaces the client.
type Clients struct {
	mu     sync.Mutex
	key    [3]string
	client *Client
}

// For returns the shared client for this configuration.
func (p *Clients) For(serverURL, username, password string) *Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := [3]string{serverURL, username, password}
	if p.client == nil || p.key != key {
		p.key = key
		p.client = NewClient(serverURL, username, password)
	}
	return p.client
}

// APIError reports that the FreshRSS server answered a request and refused it.
// A transport failure carries no such answer, and the difference is what decides
// whether a queued change was rejected or merely never delivered.
type APIError struct {
	Op         string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s failed with status %d: %s", e.Op, e.StatusCode, e.Body)
}

// RejectsItem reports whether the answer blames the items in the request rather
// than the session or the server itself. Only such an answer may spend a queued
// change's retries: an expired session or a server hiccup says nothing about the
// item, and would otherwise discard changes that were never at fault.
func (e *APIError) RejectsItem() bool {
	switch e.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusTooManyRequests:
		return false
	}
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// Login authenticates now, replacing any session. Other calls log in on their
// own; this is for checking a configuration.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	c.authToken, c.writeToken = "", ""

	data := url.Values{}
	data.Set("Email", c.username)
	data.Set("Passwd", c.password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/accounts/ClientLogin", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	body, err := c.send(req, "login")
	if err != nil {
		return err
	}
	// Response: SID=...\nLSID=...\nAuth=...
	for _, line := range strings.Split(string(body), "\n") {
		if token, ok := strings.CutPrefix(strings.TrimSpace(line), "Auth="); ok && token != "" {
			c.authToken = token
			return nil
		}
	}
	return errors.New("login: auth token not found in response")
}

// ensureSession logs in when there is no session and, for writes, fetches the
// write token. It returns the tokens to use.
func (c *Client) ensureSession(ctx context.Context, write bool) (auth, writeToken string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authToken == "" {
		if err := c.loginLocked(ctx); err != nil {
			return "", "", err
		}
	}
	if write && c.writeToken == "" {
		err := c.fetchWriteTokenLocked(ctx)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			// The session expired before the write token was fetched.
			if err = c.loginLocked(ctx); err == nil {
				err = c.fetchWriteTokenLocked(ctx)
			}
		}
		if err != nil {
			return "", "", err
		}
	}
	return c.authToken, c.writeToken, nil
}

func (c *Client) fetchWriteTokenLocked(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/reader/api/0/token", nil)
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Authorization", "GoogleLogin auth="+c.authToken)
	body, err := c.send(req, "token")
	if err != nil {
		return err
	}
	// FreshRSS ends the token with a newline.
	c.writeToken = strings.TrimSpace(string(body))
	return nil
}

// dropSession forgets the session if it is still the one that was refused, so
// a concurrent caller that already logged in again is not undone.
func (c *Client) dropSession(auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authToken == auth {
		c.authToken, c.writeToken = "", ""
	}
}

// call sends an authenticated request built by build, which receives the
// write token when write is set. A 401 costs one fresh login and one retry.
func (c *Client) call(ctx context.Context, op string, write bool, build func(writeToken string) (*http.Request, error)) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		auth, writeToken, err := c.ensureSession(ctx, write)
		if err != nil {
			return nil, err
		}
		req, err := build(writeToken)
		if err != nil {
			return nil, fmt.Errorf("create %s request: %w", op, err)
		}
		req.Header.Set("Authorization", "GoogleLogin auth="+auth)

		body, err := c.send(req, op)
		var apiErr *APIError
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			c.dropSession(auth)
			continue
		}
		return body, err
	}
}

// send performs one request and reads the whole answer; a non-200 answer is an
// *APIError.
func (c *Client) send(req *http.Request, op string) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", op, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{Op: op, StatusCode: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

func (c *Client) get(ctx context.Context, op, path string, query url.Values) ([]byte, error) {
	return c.call(ctx, op, false, func(string) (*http.Request, error) {
		target := c.baseURL + path
		if len(query) > 0 {
			target += "?" + query.Encode()
		}
		return http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	})
}

// post sends a form; with write set the form carries the write token T.
func (c *Client) post(ctx context.Context, op, path string, write bool, form url.Values) ([]byte, error) {
	return c.call(ctx, op, write, func(writeToken string) (*http.Request, error) {
		data := url.Values{}
		for k, v := range form {
			data[k] = v
		}
		if write {
			data.Set("T", writeToken)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(data.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
}

// Subscription represents a feed subscription. ID is its stream id, "feed/<n>".
type Subscription struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	HTMLURL    string     `json:"htmlUrl"`
	IconURL    string     `json:"iconUrl"`
	Categories []Category `json:"categories"`
}

// Category represents a feed category
type Category struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// GetCategories retrieves the label tags; state tags are left out.
func (c *Client) GetCategories(ctx context.Context) ([]Category, error) {
	body, err := c.get(ctx, "tag/list", "/reader/api/0/tag/list", url.Values{"output": {"json"}})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tags []struct {
			ID string `json:"id"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode tag/list response: %w", err)
	}
	categories := make([]Category, 0, len(result.Tags))
	for _, tag := range result.Tags {
		if label, ok := strings.CutPrefix(tag.ID, LabelPrefix); ok {
			categories = append(categories, Category{ID: tag.ID, Label: label})
		}
	}
	return categories, nil
}

// GetSubscriptions retrieves all feed subscriptions
func (c *Client) GetSubscriptions(ctx context.Context) ([]Subscription, error) {
	body, err := c.get(ctx, "subscription/list", "/reader/api/0/subscription/list", url.Values{"output": {"json"}})
	if err != nil {
		return nil, err
	}
	var result struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode subscription/list response: %w", err)
	}
	return result.Subscriptions, nil
}

// ItemIDQuery selects item ids from a stream.
type ItemIDQuery struct {
	// Stream is s: StreamReadingList, a "feed/<n>" id, a label, or a state.
	Stream string
	// Include (it) keeps only items also in this stream; Exclude (xt) drops
	// items in it. Unread is Exclude: StreamRead.
	Include string
	Exclude string
	// Count is n. The server has no upper bound, and asking for enough to get
	// everything at once avoids the continuation edge case in pitfall 28.
	Count int
	// Continuation is c from the previous page; empty for the first.
	Continuation string
}

// ItemIDPage is one page of ids, newest first. Continuation is empty on the
// last page.
type ItemIDPage struct {
	IDs          []int64
	Continuation string
}

// StreamItemIDs is stream/items/ids, which answers decimal ids.
func (c *Client) StreamItemIDs(ctx context.Context, q ItemIDQuery) (ItemIDPage, error) {
	params := url.Values{"output": {"json"}, "s": {q.Stream}}
	if q.Count > 0 {
		params.Set("n", strconv.Itoa(q.Count))
	}
	if q.Include != "" {
		params.Set("it", q.Include)
	}
	if q.Exclude != "" {
		params.Set("xt", q.Exclude)
	}
	if q.Continuation != "" {
		params.Set("c", q.Continuation)
	}
	body, err := c.get(ctx, "stream/items/ids", "/reader/api/0/stream/items/ids", params)
	if err != nil {
		return ItemIDPage{}, err
	}
	var result struct {
		ItemRefs []struct {
			ID string `json:"id"`
		} `json:"itemRefs"`
		Continuation json.RawMessage `json:"continuation"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ItemIDPage{}, fmt.Errorf("decode stream/items/ids response: %w", err)
	}
	page := ItemIDPage{IDs: make([]int64, 0, len(result.ItemRefs))}
	for _, ref := range result.ItemRefs {
		id, err := ParseItemID(ref.ID)
		if err != nil {
			return ItemIDPage{}, fmt.Errorf("stream/items/ids: %w", err)
		}
		page.IDs = append(page.IDs, id)
	}
	// Some servers send the continuation as a number, FreshRSS as a string.
	if len(result.Continuation) > 0 && string(result.Continuation) != "null" {
		page.Continuation = strings.Trim(string(result.Continuation), `"`)
	}
	return page, nil
}

// Item is one entry from stream/items/contents.
type Item struct {
	// ID is the entry id; it is also the crawl time in microseconds.
	ID       int64
	StreamID string
	Title    string
	URL      string
	Author   string
	// Content is the feed's body HTML, untrusted.
	Content string
	// Published is the feed's claimed publish time in Unix seconds, 0 when absent.
	Published int64
	Read      bool
}

// StreamItemContents is stream/items/contents for the given ids. Ids the
// server no longer has are simply missing from the answer.
func (c *Client) StreamItemContents(ctx context.Context, ids []int64) ([]Item, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	form := url.Values{"output": {"json"}}
	for _, id := range ids {
		form.Add("i", strconv.FormatInt(id, 10))
	}
	body, err := c.post(ctx, "stream/items/contents", "/reader/api/0/stream/items/contents", false, form)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Published int64  `json:"published"`
			Author    string `json:"author"`
			Canonical []struct {
				Href string `json:"href"`
			} `json:"canonical"`
			Alternate []struct {
				Href string `json:"href"`
			} `json:"alternate"`
			Categories []string `json:"categories"`
			Summary    struct {
				Content string `json:"content"`
			} `json:"summary"`
			Content struct {
				Content string `json:"content"`
			} `json:"content"`
			Origin struct {
				StreamID string `json:"streamId"`
			} `json:"origin"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode stream/items/contents response: %w", err)
	}
	items := make([]Item, 0, len(result.Items))
	for _, raw := range result.Items {
		id, err := ParseItemID(raw.ID)
		if err != nil {
			return nil, fmt.Errorf("stream/items/contents: %w", err)
		}
		item := Item{
			ID:        id,
			StreamID:  raw.Origin.StreamID,
			Title:     raw.Title,
			Author:    raw.Author,
			Content:   raw.Summary.Content,
			Published: raw.Published,
		}
		if item.Content == "" {
			item.Content = raw.Content.Content
		}
		if len(raw.Canonical) > 0 {
			item.URL = raw.Canonical[0].Href
		} else if len(raw.Alternate) > 0 {
			item.URL = raw.Alternate[0].Href
		}
		for _, cat := range raw.Categories {
			if cat == StreamRead {
				item.Read = true
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// ParseItemID reads an entry id in either format the API uses: the decimal
// form of stream/items/ids, or the long form
// "tag:google.com,2005:reader/item/<16 hex digits>" of stream/items/contents.
func ParseItemID(s string) (int64, error) {
	if hex, ok := strings.CutPrefix(s, longItemIDPrefix); ok {
		v, err := strconv.ParseUint(hex, 16, 63)
		if err != nil {
			return 0, fmt.Errorf("item id %q: %w", s, err)
		}
		return int64(v), nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("item id %q is neither decimal nor long form", s)
	}
	return v, nil
}

// editTag adds or removes a tag on items. The server answers OK for any i,
// including ids it does not have, so success confirms nothing (pitfall 29).
func (c *Client) editTag(ctx context.Context, itemIDs []int64, addTag, removeTag string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	form := url.Values{}
	for _, id := range itemIDs {
		form.Add("i", strconv.FormatInt(id, 10))
	}
	if addTag != "" {
		form.Set("a", addTag)
	}
	if removeTag != "" {
		form.Set("r", removeTag)
	}
	_, err := c.post(ctx, "edit-tag", "/reader/api/0/edit-tag", true, form)
	return err
}

// MarkAsRead marks items as read in one request.
func (c *Client) MarkAsRead(ctx context.Context, itemIDs []int64) error {
	return c.editTag(ctx, itemIDs, StreamRead, "")
}

// MarkAsUnread marks items as unread in one request.
func (c *Client) MarkAsUnread(ctx context.Context, itemIDs []int64) error {
	return c.editTag(ctx, itemIDs, "", StreamRead)
}

// MarkAllAsRead marks every item of a stream ("feed/<n>", a label, or
// StreamReadingList) up to and including olderThan as read. The server compares
// ts with item ids, so olderThan is an item id, i.e. a crawl time in
// microseconds, not a publish time (pitfall 30). It must be positive: ts 0 means "now" to
// the server and would also mark items the user never saw.
func (c *Client) MarkAllAsRead(ctx context.Context, streamID string, olderThan int64) error {
	if olderThan <= 0 {
		return fmt.Errorf("mark-all-as-read: olderThan must be a positive item id, got %d", olderThan)
	}
	form := url.Values{"s": {streamID}, "ts": {strconv.FormatInt(olderThan, 10)}}
	_, err := c.post(ctx, "mark-all-as-read", "/reader/api/0/mark-all-as-read", true, form)
	return err
}
