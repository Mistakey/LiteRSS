package freshrss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"LiteRSS/internal/freshrss/freshrsstest"
)

var pngIcon = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR fake png body")

// iconURLs returns each subscription's iconUrl by stream id.
func iconURLs(t *testing.T, c *Client) map[string]string {
	t.Helper()
	subs, err := c.GetSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	urls := map[string]string{}
	for _, s := range subs {
		urls[s.ID] = s.IconURL
	}
	return urls
}

func TestIconComesFromTheFaviconCache(t *testing.T) {
	fake, c := newFake(t)
	fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One", Icon: pngIcon})

	icon, err := c.Icon(context.Background(), iconURLs(t, c)["feed/1"])
	if err != nil {
		t.Fatalf("Icon: %v", err)
	}
	if string(icon.Data) != string(pngIcon) || icon.ContentType != "image/png" {
		t.Errorf("icon = %q %q, want the feed's PNG", icon.Data, icon.ContentType)
	}
}

// FreshRSS answers f.php with its placeholder for a feed it has no icon for;
// that is no icon, so the letter badge stays.
func TestPlaceholderIsNoIcon(t *testing.T) {
	_, c := newFake(t)
	if _, err := c.Icon(context.Background(), iconURLs(t, c)["feed/2"]); !errors.Is(err, ErrNoIcon) {
		t.Errorf("Icon of a feed without one = %v, want ErrNoIcon", err)
	}
}

// Whatever host the iconUrl names, the request goes to the configured
// server's f.php: FreshRSS builds iconUrl from its base_url, which may differ
// from the address the client uses, and no other host is ever contacted.
func TestIconOnlyAsksTheConfiguredServer(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		_, _ = w.Write(pngIcon)
	}))
	defer other.Close()

	fake, c := newFake(t)
	fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One", Icon: pngIcon})

	icon, err := c.Icon(context.Background(), other.URL+"/p/f.php?h=1")
	if err != nil || string(icon.Data) != string(pngIcon) {
		t.Errorf("Icon via another host's iconUrl = %q, %v; want feed 1's icon from the configured server", icon.Data, err)
	}
	for _, bad := range []string{"", other.URL + "/favicon.ico", other.URL + "/f.php", "::"} {
		if _, err := c.Icon(context.Background(), bad); !errors.Is(err, ErrNoIcon) {
			t.Errorf("Icon(%q) = %v, want ErrNoIcon", bad, err)
		}
	}
	if n := elsewhere.Load(); n != 0 {
		t.Errorf("other host got %d requests, want 0", n)
	}
}

func TestIconRefusesWhatIsNotARasterImage(t *testing.T) {
	fake, c := newFake(t)
	for id, body := range map[int]string{
		1: `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		2: "<html>login</html>",
	} {
		fake.AddFeeds(freshrsstest.Feed{ID: id, Title: "x", Icon: []byte(body)})
	}
	urls := iconURLs(t, c)
	for _, feed := range []string{"feed/1", "feed/2"} {
		if _, err := c.Icon(context.Background(), urls[feed]); !errors.Is(err, ErrNoIcon) {
			t.Errorf("Icon of %s = %v, want ErrNoIcon", feed, err)
		}
	}
}

// A server that answers f.php with an error refused the icon.
func TestIconErrorStatusIsAnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/f.php") {
			http.Error(w, "gone", http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "user", "secret")
	var apiErr *APIError
	if _, err := c.Icon(context.Background(), srv.URL+"/f.php?h=1"); !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("Icon = %v, want an APIError with 403", err)
	}
}
