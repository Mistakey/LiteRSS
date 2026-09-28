package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"LiteRSS/internal/database"
	"LiteRSS/internal/fulltext"
)

type fakeSettings map[string]string

func (f fakeSettings) Load(context.Context) (map[string]string, error) { return f, nil }

type env struct {
	db  *database.DB
	svc *Service
	set fakeSettings
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	set := fakeSettings{}
	svc := New(db.DB, set, Clients{Web: &http.Client{}, API: &http.Client{}})
	svc.BaiduGap = 0
	return &env{db: db, svc: svc, set: set}
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (e *env) article(t *testing.T, id int64, url, title, body string) {
	t.Helper()
	e.exec(t, `INSERT INTO articles (item_id, stream_id, url, title, published_at) VALUES (?, 'feed/1', ?, ?, 1)`, id, url, title)
	if body != "" {
		e.exec(t, `INSERT INTO article_contents (item_id, content) VALUES (?, ?)`, id, body)
	}
}

func (e *env) stored(t *testing.T, q string, id int64) (string, bool) {
	t.Helper()
	var v string
	err := e.db.QueryRow(q, id).Scan(&v)
	if err != nil {
		return "", false
	}
	return v, true
}

// fakeBaidu translates each line of q through dict and records every q.
func fakeBaidu(t *testing.T, dict map[string]string, errorCode string) (*httptest.Server, *[]string) {
	t.Helper()
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("to") != "zh" || r.Form.Get("appid") != "app" || r.Form.Get("sign") == "" {
			t.Errorf("unexpected Baidu form: %v", r.Form)
		}
		q := r.Form.Get("q")
		queries = append(queries, q)
		if errorCode != "" {
			json.NewEncoder(w).Encode(map[string]string{"error_code": errorCode, "error_msg": "fail"})
			return
		}
		var result []map[string]string
		for _, line := range strings.Split(q, "\n") {
			dst, ok := dict[line]
			if !ok {
				dst = line
			}
			result = append(result, map[string]string{"src": line, "dst": dst})
		}
		json.NewEncoder(w).Encode(map[string]any{"trans_result": result})
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

func (e *env) useBaidu(url string) {
	e.set["baidu_app_id"] = "app"
	e.set["baidu_secret_key"] = "secret"
	e.svc.BaiduEndpoint = url
}

func titlesOf(r Titles) map[int64]string {
	m := map[int64]string{}
	for _, t := range r.Titles {
		m[t.ID] = t.TranslatedTitle
	}
	return m
}

func TestTranslateTitlesStoresTranslationsAndDecidedChinese(t *testing.T) {
	e := newEnv(t)
	srv, queries := fakeBaidu(t, map[string]string{
		"OpenAI releases a new model": "OpenAI 发布新模型",
		"Hello World":                 "你好世界",
	}, "")
	e.useBaidu(srv.URL)
	e.article(t, 1, "https://x/1", "OpenAI releases a new model", "")
	e.article(t, 2, "https://x/2", "少数派周报", "")
	e.article(t, 3, "https://x/3", "Old news", "")
	e.exec(t, `INSERT INTO title_translations (item_id, translated_title) VALUES (3, '旧闻')`)
	e.article(t, 4, "https://x/4", "Hello\nWorld", "")
	e.article(t, 5, "https://x/5", "Kubernetes", "") // Baidu keeps it as is

	got, err := e.svc.TranslateTitles(context.Background(), []int64{1, 2, 3, 4, 5, 99})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{1: "OpenAI 发布新模型", 2: "少数派周报", 3: "旧闻", 4: "你好世界", 5: "Kubernetes"}
	if m := titlesOf(got); len(m) != len(want) || got.Message != "" {
		t.Fatalf("got %+v, want %v", got, want)
	}
	for id, w := range want {
		if m := titlesOf(got); m[id] != w {
			t.Errorf("answer %d = %q, want %q", id, m[id], w)
		}
		if s, _ := e.stored(t, `SELECT translated_title FROM title_translations WHERE item_id = ?`, id); s != w {
			t.Errorf("stored %d = %q, want %q", id, s, w)
		}
	}
	if len(*queries) != 1 || (*queries)[0] != "OpenAI releases a new model\nHello World\nKubernetes" {
		t.Fatalf("Baidu queries = %q; the Chinese and decided titles must not be sent", *queries)
	}

	// Everything is decided now: no second request.
	if _, err := e.svc.TranslateTitles(context.Background(), []int64{1, 2, 4, 5}); err != nil {
		t.Fatal(err)
	}
	if len(*queries) != 1 {
		t.Fatalf("decided titles were sent again: %q", *queries)
	}
}

func TestTranslateTitlesWithoutBaiduDecidesOnlyChinese(t *testing.T) {
	e := newEnv(t)
	e.article(t, 1, "https://x/1", "OpenAI releases a new model", "")
	e.article(t, 2, "https://x/2", "少数派周报", "")

	got, err := e.svc.TranslateTitles(context.Background(), []int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if m := titlesOf(got); len(m) != 1 || m[2] != "少数派周报" || !strings.Contains(got.Message, "百度翻译") {
		t.Fatalf("got %+v", got)
	}
	if _, ok := e.stored(t, `SELECT translated_title FROM title_translations WHERE item_id = ?`, 1); ok {
		t.Fatal("an untranslated English title was stored as decided")
	}
}

func TestTranslateTitlesReportsBaiduErrorInChinese(t *testing.T) {
	e := newEnv(t)
	srv, _ := fakeBaidu(t, nil, "54003")
	e.useBaidu(srv.URL)
	e.article(t, 1, "https://x/1", "OpenAI releases a new model", "")

	got, err := e.svc.TranslateTitles(context.Background(), []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Titles) != 0 || got.Message != "百度翻译访问太频繁，稍后再试。" {
		t.Fatalf("got %+v", got)
	}
}

// articlePage is a page readability extracts; paragraph is its text.
const paragraph = "The quick brown fox jumps over the lazy dog while the committee reviews the budget for the next fiscal year. "

func articlePage() string {
	return "<html><head><title>T</title></head><body><nav>menu</nav><article><h1>T</h1><p>" +
		strings.Repeat(paragraph, 6) + "</p><p>" + strings.Repeat(paragraph, 6) + "</p></article></body></html>"
}

// fakeWeb serves the article page at /ok and 403 elsewhere, counting hits.
func fakeWeb(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/ok" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(articlePage()))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestFullTextCachesSuccess(t *testing.T) {
	e := newEnv(t)
	web, hits := fakeWeb(t)
	e.article(t, 1, web.URL+"/ok", "T", "")

	for range 2 {
		got, err := e.svc.FullText(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome != fulltext.OutcomeSuccess || !strings.Contains(got.Content, "quick brown fox") || got.Message != "" {
			t.Fatalf("got %+v", got)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("page fetched %d times, want once then cached", hits.Load())
	}
}

func TestFullTextFailureGivesChineseReason(t *testing.T) {
	e := newEnv(t)
	web, hits := fakeWeb(t)
	e.article(t, 1, web.URL+"/paywall", "T", "")
	e.article(t, 2, "", "T", "")

	got, err := e.svc.FullText(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != fulltext.OutcomeBlocked || got.Content != "" || got.Message != "站点拒绝了这次抓取，可以在浏览器里打开原文。" {
		t.Fatalf("got %+v", got)
	}
	if _, ok := e.stored(t, `SELECT content FROM fulltext_cache WHERE item_id = ?`, 1); ok {
		t.Fatal("a failure was cached")
	}

	got, err = e.svc.FullText(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != fulltext.OutcomeNoLink || got.Message != "这篇文章没有可抓取的原文链接。" {
		t.Fatalf("no link: got %+v", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

// fakeModel answers every chat request with reply and records the user
// prompts.
func fakeModel(t *testing.T, reply string) (*httptest.Server, *[]string) {
	t.Helper()
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		for _, m := range req.Messages {
			if m.Role == "user" {
				prompts = append(prompts, m.Content)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &prompts
}

func (e *env) useModel(url string) {
	e.set["llm_endpoint"] = url + "/v1/chat/completions"
	e.set["llm_model"] = "test-model"
	e.set["llm_api_key"] = "key"
}

// rssBody has 350 visible characters: long enough to summarize.
var rssBody = "<p>" + strings.Repeat("长", 350) + "</p>"

func TestSummarizeFromFullTextThenStored(t *testing.T) {
	e := newEnv(t)
	web, _ := fakeWeb(t)
	model, prompts := fakeModel(t, "文章讲了一只狐狸。\n\n- 要点一\n- 要点二")
	e.useModel(model.URL)
	e.article(t, 1, web.URL+"/ok", "Fox", rssBody)

	got, err := e.svc.Summarize(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "" || !strings.Contains(got.HTML, "<li>要点一</li>") {
		t.Fatalf("got %+v", got)
	}
	if len(*prompts) != 1 || !strings.Contains((*prompts)[0], "quick brown fox") || strings.Contains((*prompts)[0], "长长") {
		t.Fatalf("the model was not given the full text: %q", *prompts)
	}
	if again, err := e.svc.Summarize(context.Background(), 1); err != nil || again.HTML != got.HTML || len(*prompts) != 1 {
		t.Fatalf("stored summary not reused: %+v %v, %d model calls", again, err, len(*prompts))
	}
}

func TestSummarizeFallsBackToLongRSSBody(t *testing.T) {
	e := newEnv(t)
	web, _ := fakeWeb(t)
	model, prompts := fakeModel(t, "摘要 <script>alert(1)</script><img src=x onerror=alert(1)> [点我](javascript:alert(1))\n\n<div>原始块</div>")
	e.useModel(model.URL)
	e.article(t, 1, web.URL+"/paywall", "Paywalled", rssBody)

	got, err := e.svc.Summarize(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "未能获取全文：站点拒绝了这次抓取，可以在浏览器里打开原文。摘要基于 RSS 正文。" {
		t.Fatalf("note = %q", got.Note)
	}
	if len(*prompts) != 1 || !strings.Contains((*prompts)[0], strings.Repeat("长", 350)) {
		t.Fatalf("the model was not given the RSS body: %q", *prompts)
	}
	if !strings.Contains(got.HTML, "摘要") {
		t.Fatalf("html = %q", got.HTML)
	}
	for _, raw := range []string{"<script", "<img", "onerror", "javascript:", "<div"} {
		if strings.Contains(got.HTML, raw) {
			t.Errorf("summary HTML carries raw HTML %q: %s", raw, got.HTML)
		}
	}
	// Reopened, the stored summary still says what it is based on.
	if again, err := e.svc.Summarize(context.Background(), 1); err != nil || again != got || len(*prompts) != 1 {
		t.Fatalf("reopened: %+v %v, %d model calls; want %+v from the store", again, err, len(*prompts), got)
	}
}

func TestSummarizeRefusesShortRSSBody(t *testing.T) {
	e := newEnv(t)
	web, _ := fakeWeb(t)
	model, prompts := fakeModel(t, "不该调用")
	e.useModel(model.URL)
	e.article(t, 1, web.URL+"/paywall", "Paywalled", "<p>Only a teaser.</p>")

	got, err := e.svc.Summarize(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != "" || got.Note != "未能获取全文：站点拒绝了这次抓取，可以在浏览器里打开原文。RSS 正文太短，无法生成摘要。" {
		t.Fatalf("got %+v", got)
	}
	if len(*prompts) != 0 {
		t.Fatal("the model was called for a body too short")
	}
	if _, ok := e.stored(t, `SELECT summary FROM summaries WHERE item_id = ?`, 1); ok {
		t.Fatal("a refusal was stored")
	}
}

func TestSummarizeWithoutModelSaysSo(t *testing.T) {
	e := newEnv(t)
	e.article(t, 1, "https://x/1", "T", rssBody)

	got, err := e.svc.Summarize(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.HTML != "" || got.Note != "还没有配置摘要模型，请在设置里填写。" {
		t.Fatalf("got %+v", got)
	}
}
