package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// modelSettings configures the model at url.
type modelSettings string

func (m modelSettings) Load(context.Context) (map[string]string, error) {
	return map[string]string{"llm_endpoint": string(m) + "/v1/chat/completions", "llm_model": "m"}, nil
}

func TestTranslateArticle(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO articles (item_id, stream_id, url, title, published_at) VALUES (100, 'feed/1', '', 'Hello world', 1)`)

	// Without a model: 200, no blocks, the Chinese hint.
	rec := api.send(t, http.MethodPost, "/api/articles/100/translation", `{"blocks":["Hello."]}`)
	var tr enrich.Translation
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tr) != nil ||
		len(tr.Blocks) != 0 || tr.Message != "还没有配置大模型，请在设置里填写。" {
		t.Fatalf("no model: %d %s", rec.Code, rec.Body)
	}

	fail := false
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `["你好。","世界。"]`}}}})
	}))
	t.Cleanup(model.Close)
	api.enrich = enrich.New(api.db.DB, modelSettings(model.URL), enrich.Clients{Web: &http.Client{}, API: &http.Client{}})
	api.handler = Handler(api.deps())

	rec = api.send(t, http.MethodPost, "/api/articles/100/translation", `{"blocks":["Hello.","World."]}`)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tr) != nil ||
		strings.Join(tr.Blocks, "|") != "你好。|世界。" || tr.Message != "" {
		t.Fatalf("success: %d %s", rec.Code, rec.Body)
	}

	fail = true
	rec = api.send(t, http.MethodPost, "/api/articles/100/translation", `{"blocks":["Other."]}`)
	tr = enrich.Translation{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tr) != nil ||
		len(tr.Blocks) != 0 || tr.Message != "全文翻译失败，请检查设置里的大模型，或稍后再试。" {
		t.Fatalf("failure: %d %s", rec.Code, rec.Body)
	}

	for _, c := range []struct {
		target, body string
		code         int
	}{
		{"/api/articles/999/translation", `{"blocks":["Hello."]}`, http.StatusNotFound},
		{"/api/articles/x/translation", `{"blocks":["Hello."]}`, http.StatusBadRequest},
		{"/api/articles/100/translation", `{}`, http.StatusBadRequest},
		{"/api/articles/100/translation", `{"blocks":[""]}`, http.StatusBadRequest},
		{"/api/articles/100/translation", `{"blocks":"Hello."}`, http.StatusBadRequest},
	} {
		if rec := api.send(t, http.MethodPost, c.target, c.body); rec.Code != c.code {
			t.Errorf("POST %s %s: %d, want %d", c.target, c.body, rec.Code, c.code)
		}
	}
}
