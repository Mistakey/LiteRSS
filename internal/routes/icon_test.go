package routes

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"LiteRSS/internal/freshrss"
)

// fakeIconSource answers every iconUrl with icon, or err when set.
type fakeIconSource struct {
	icon freshrss.Icon
	err  error
}

func (f *fakeIconSource) Icon(context.Context, string) (freshrss.Icon, error) {
	return f.icon, f.err
}

func TestFeedIcon(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO feeds (stream_id, title, icon_url) VALUES
		('feed/1', 'One', 'http://rss/f.php?h=1'), ('feed/2', 'Two', 'http://rss/f.php?h=2'), ('feed/3', 'Three', '')`)
	api.iconSource.icon = freshrss.Icon{Data: []byte("\x89PNG\r\n\x1a\nicon"), ContentType: "image/png"}

	rec := api.do(t, http.MethodGet, "/api/feeds/icon?id=feed/1&v=x")
	if rec.Code != http.StatusOK || rec.Body.String() != "\x89PNG\r\n\x1a\nicon" {
		t.Fatalf("icon = %d %q", rec.Code, rec.Body.String())
	}
	if h := rec.Header(); h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Cache-Control") != "private, max-age=86400" {
		t.Errorf("icon headers = %v", h)
	}

	// No icon for any reason is an empty 204 the frontend turns into the letter badge.
	for _, id := range []string{"feed/3", "feed/9"} {
		if rec := api.do(t, http.MethodGet, "/api/feeds/icon?id="+id); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Errorf("icon of %s = %d %q, want an empty 204", id, rec.Code, rec.Body.String())
		}
	}
	api.iconSource.err = errors.New("connection refused")
	rec = api.do(t, http.MethodGet, "/api/feeds/icon?id=feed/2")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("icon with FreshRSS unreachable = %d %q, want an uncached 204", rec.Code, rec.Header().Get("Cache-Control"))
	}

	if rec := api.do(t, http.MethodGet, "/api/feeds/icon"); rec.Code != http.StatusBadRequest {
		t.Errorf("icon without id = %d, want 400", rec.Code)
	}
}
