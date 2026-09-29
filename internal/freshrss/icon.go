package freshrss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNoIcon reports that a feed has no icon worth showing: its iconUrl is not
// FreshRSS's favicon cache, or the cache answered with FreshRSS's placeholder
// or with something that is not a raster image.
var ErrNoIcon = errors.New("freshrss: no feed icon")

// maxIconBytes bounds an icon; a favicon is a few kilobytes.
const maxIconBytes = 512 << 10

// defaultFaviconPath is where FreshRSS keeps the placeholder f.php answers
// for a feed whose favicon it could not get.
const defaultFaviconPath = "/themes/icons/default_favicon.ico"

// Icon is a feed icon: raster image bytes and their sniffed media type.
type Icon struct {
	Data        []byte
	ContentType string
}

// Icon fetches a feed's icon from FreshRSS's favicon cache, f.php, given the
// subscription's iconUrl. Only the iconUrl's query is used: the request always
// goes to f.php beside the configured API, so neither an iconUrl naming
// another host nor a FreshRSS base_url that differs from the address this
// client uses sends a request anywhere else (spec D15). Like f.php itself it
// needs no session. FreshRSS's placeholder and anything that is not a raster
// image are ErrNoIcon; a refusing server is an *APIError.
func (c *Client) Icon(ctx context.Context, iconURL string) (Icon, error) {
	u, err := url.Parse(iconURL)
	if err != nil || !strings.HasSuffix(u.Path, "/f.php") || u.RawQuery == "" {
		return Icon{}, ErrNoIcon
	}
	data, err := c.fetchPublic(ctx, "favicon", "/f.php?"+u.RawQuery)
	if err != nil {
		return Icon{}, err
	}
	placeholder, err := c.placeholderIcon(ctx)
	if err != nil {
		return Icon{}, err
	}
	if placeholder != nil && bytes.Equal(data, placeholder) {
		return Icon{}, ErrNoIcon
	}
	// Raster formats only: an SVG would be a document on this origin.
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") || strings.Contains(contentType, "svg") {
		return Icon{}, ErrNoIcon
	}
	return Icon{Data: data, ContentType: contentType}, nil
}

// placeholderIcon returns FreshRSS's placeholder favicon, fetched once per
// client. A server without one (a 404) has none to compare with, which is
// nil; a failed fetch is tried again next time.
func (c *Client) placeholderIcon(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	known, placeholder := c.placeholderKnown, c.placeholder
	c.mu.Unlock()
	if known {
		return placeholder, nil
	}
	data, err := c.fetchPublic(ctx, "default favicon", defaultFaviconPath)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		data, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.placeholderKnown, c.placeholder = true, data
	c.mu.Unlock()
	return data, nil
}

// fetchPublic GETs a path under the FreshRSS root, the directory holding the
// API, without a session.
func (c *Client) fetchPublic(ctx context.Context, op, path string) ([]byte, error) {
	root := strings.TrimSuffix(c.baseURL, "/api/greader.php")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", op, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", op, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{Op: op, StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", op, err)
	}
	if len(data) > maxIconBytes {
		return nil, fmt.Errorf("%s: larger than %d bytes: %w", op, maxIconBytes, ErrNoIcon)
	}
	return data, nil
}
