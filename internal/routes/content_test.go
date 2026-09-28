package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"LiteRSS/internal/enrich"
)

// noSettings is an empty configuration: no Baidu, no model.
type noSettings struct{}

func (noSettings) Load(context.Context) (map[string]string, error) { return map[string]string{}, nil }

func TestContentActions(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO articles (item_id, stream_id, url, title, published_at) VALUES
		(100, 'feed/1', '', 'Hello world', 1), (200, 'feed/1', 'https://x/2', '少数派周报', 2)`)

	rec := api.send(t, http.MethodPost, "/api/articles/100/fulltext", "")
	var ft enrich.FullText
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &ft) != nil || ft.Outcome != "no_link" || ft.Message == "" {
		t.Fatalf("fulltext: %d %s", rec.Code, rec.Body)
	}

	rec = api.send(t, http.MethodPost, "/api/articles/translate-titles", `{"ids":[100,200]}`)
	var titles enrich.Titles
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &titles) != nil ||
		len(titles.Titles) != 1 || titles.Titles[0].ID != 200 || titles.Message == "" {
		t.Fatalf("translate titles: %d %s", rec.Code, rec.Body)
	}

	rec = api.send(t, http.MethodPost, "/api/articles/100/summary", "")
	var sum enrich.Summary
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &sum) != nil || sum.HTML != "" || sum.Note == "" {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}

	for _, c := range []struct {
		target, body string
		code         int
	}{
		{"/api/articles/999/fulltext", "", http.StatusNotFound},
		{"/api/articles/999/summary", "", http.StatusNotFound},
		{"/api/articles/x/summary", "", http.StatusBadRequest},
		{"/api/articles/translate-titles", `{"ids":"1"}`, http.StatusBadRequest},
		{"/api/articles/translate-titles", `{}`, http.StatusBadRequest},
		{"/api/articles/translate-titles", `{"ids":[` + strings.Repeat("1,", 200) + `1]}`, http.StatusBadRequest},
	} {
		if rec := api.send(t, http.MethodPost, c.target, c.body); rec.Code != c.code {
			t.Errorf("POST %s %s: %d, want %d", c.target, c.body, rec.Code, c.code)
		}
	}
}
