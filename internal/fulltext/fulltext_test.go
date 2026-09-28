package fulltext_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"LiteRSS/internal/fulltext"
)

// 一次抓取的结论有五种，而不是「成功」与「500」两种。下面每个测试钉住其中一种，
// 并且全部离线：分类层只看状态码、Content-Type 与 body。

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

const articleHTML = `<html><head><title>T</title></head><body><article>` +
	`<p>Readability needs a paragraph long enough to be taken for the main body of the page, ` +
	`so this sentence goes on for a while to clear its scoring threshold.</p>` +
	`</article></body></html>`

func articleWithBody(body string) string {
	return `<html><head><title>Article</title></head><body><article><p>` + body +
		`</p></article></body></html>`
}

func TestFetchReturnsContentOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML)
	}))
	defer srv.Close()

	res, err := fulltext.NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.Outcome != fulltext.OutcomeSuccess {
		t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, fulltext.OutcomeSuccess, res.Detail)
	}
	if !strings.Contains(res.Content, "Readability needs a paragraph") {
		t.Fatalf("content did not carry the article body: %q", res.Content)
	}
}

// reddit 与硬反爬六站：请求到达了，站点拒绝作答。
func TestFetchClassifiesForbiddenAsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	res, err := fulltext.NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.Outcome != fulltext.OutcomeBlocked {
		t.Fatalf("outcome = %q, want %q", res.Outcome, fulltext.OutcomeBlocked)
	}
}

// PDF 等非 HTML 在进入 readability 之前就该被拦下。
func TestFetchClassifiesNonHTMLAsNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7\n")
	}))
	defer srv.Close()

	res, err := fulltext.NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL+"/a.pdf")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.Outcome != fulltext.OutcomeNoContent {
		t.Fatalf("outcome = %q, want %q", res.Outcome, fulltext.OutcomeNoContent)
	}
}

// youtube 视频页与 SPA 落地页：HTML 拿到了，readability 判定这页没有文章
// （Article.Node == nil），这不是错误。见 AGENTS.md 第 21 条。
func TestFetchClassifiesEmptyDocumentAsNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><head><title>SPA</title></head><body><div id=\"root\"></div></body></html>")
	}))
	defer srv.Close()

	res, err := fulltext.NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.Outcome != fulltext.OutcomeNoContent {
		t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, fulltext.OutcomeNoContent, res.Detail)
	}
	if res.Content != "" {
		t.Fatalf("content = %q, want empty", res.Content)
	}
}

// bbc.com 的传输层失败：连不上。
func TestFetchClassifiesTransportFailureAsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	addr := srv.URL
	srv.Close() // 端口关掉，请求必然失败

	res, err := fulltext.NewFetcher(client).Fetch(context.Background(), addr+"/a")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if res.Outcome != fulltext.OutcomeUnreachable {
		t.Fatalf("outcome = %q, want %q", res.Outcome, fulltext.OutcomeUnreachable)
	}
}

// 内容读到一半断了：拿到了 HTML 却提不出来，属于解析失败而非「这页没有正文」。
func TestExtractClassifiesUnreadableBodyAsParseFailed(t *testing.T) {
	body := io.MultiReader(strings.NewReader("<html><body><p>start"), errReader{})

	res := fulltext.Extract(http.StatusOK, "text/html", body, mustParse(t, "https://example.com/a"))
	if res.Outcome != fulltext.OutcomeParseFailed {
		t.Fatalf("outcome = %q, want %q", res.Outcome, fulltext.OutcomeParseFailed)
	}
}

// Content-Type 缺失的站点不少，不能因此判成「没有正文」。
func TestExtractTreatsMissingContentTypeAsHTML(t *testing.T) {
	res := fulltext.Extract(http.StatusOK, "", strings.NewReader(articleHTML), mustParse(t, "https://example.com/a"))
	if res.Outcome != fulltext.OutcomeSuccess {
		t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, fulltext.OutcomeSuccess, res.Detail)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// 抓取能力包：2023 年的 Chrome/120 换成 Chrome 141，并带上 Google 的 Referer
// （实测：前者救回 engadget 与 propastop，后者救回 nytimes）。
func TestFetchSendsBrowserCapabilities(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, articleHTML)
	}))
	defer srv.Close()

	if _, err := fulltext.NewFetcher(srv.Client()).Fetch(context.Background(), srv.URL+"/a"); err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if ua := got.Header.Get("User-Agent"); !strings.Contains(ua, "Chrome/141") {
		t.Fatalf("User-Agent = %q, want a Chrome 141 build", ua)
	}
	if ref := got.Header.Get("Referer"); ref != "https://www.google.com/" {
		t.Fatalf("Referer = %q, want https://www.google.com/", ref)
	}
}

func TestExtractClassifiesTwentyCharacterResidueAsNoContent(t *testing.T) {
	res := fulltext.Extract(
		http.StatusOK,
		"text/html",
		strings.NewReader(articleWithBody(strings.Repeat("x", 20))),
		mustParse(t, "https://example.com/a"),
	)

	if res.Outcome != fulltext.OutcomeNoContent {
		t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, fulltext.OutcomeNoContent, res.Detail)
	}
	if res.Content != "" {
		t.Fatalf("content = %q, want empty", res.Content)
	}
}

func TestExtractUsesUnicodeVisibleCharacterBoundary(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		outcome fulltext.Outcome
	}{
		{name: "twenty Chinese characters", body: strings.Repeat("字", 20), outcome: fulltext.OutcomeNoContent},
		{name: "forty-nine characters", body: strings.Repeat("x", 49), outcome: fulltext.OutcomeNoContent},
		{name: "fifty characters", body: strings.Repeat("x", 50), outcome: fulltext.OutcomeSuccess},
		{name: "one hundred fifty-three characters", body: strings.Repeat("x", 153), outcome: fulltext.OutcomeSuccess},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := fulltext.Extract(
				http.StatusOK,
				"text/html",
				strings.NewReader(articleWithBody(tt.body)),
				mustParse(t, "https://example.com/a"),
			)

			if res.Outcome != tt.outcome {
				t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, tt.outcome, res.Detail)
			}
			if tt.outcome == fulltext.OutcomeNoContent && res.Content != "" {
				t.Fatalf("content = %q, want empty", res.Content)
			}
		})
	}
}

func TestExtractPreservesShortArticleWithMedia(t *testing.T) {
	tests := []struct {
		name  string
		media string
	}{
		{name: "image", media: `<img src="/photo.jpg" alt="photo">`},
		{name: "audio", media: `<audio src="/episode.mp3"></audio>`},
		{name: "video", media: `<video src="/clip.mp4"></video>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `<html><head><title>Media</title></head><body><article>` +
				`<p>Short caption.</p>` + tt.media + `</article></body></html>`
			res := fulltext.Extract(
				http.StatusOK,
				"text/html",
				strings.NewReader(body),
				mustParse(t, "https://example.com/a"),
			)

			if res.Outcome != fulltext.OutcomeSuccess {
				t.Fatalf("outcome = %q, want %q (detail: %s)", res.Outcome, fulltext.OutcomeSuccess, res.Detail)
			}
			if res.Content == "" {
				t.Fatal("content is empty")
			}
		})
	}
}
