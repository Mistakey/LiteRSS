package fulltext

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// 抓取能力包。三项都由 2026-08-29 的实测选出（27 个失败 URL × 6 策略）：换掉
// 2023 年的 Chrome/120 救回 engadget 与 propastop，Referer 救回 nytimes，HTTP/2
// 由调用方给的 client 提供。Sec-Fetch-* / Sec-CH-UA 全套浏览器头实测零增益，
// 因此不发；爬虫身份的兜底重试见 spec D3 的撤销记录。
const (
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	searchReferer    = "https://www.google.com/"
)

// Fetcher 是抓取的请求层：取回原文页，交给 Extract 下结论。代理与超时由调用方
// 通过 client 决定。
type Fetcher struct {
	client *http.Client
}

// NewFetcher 用给定的 HTTP client 构造抓取器。
func NewFetcher(client *http.Client) *Fetcher {
	return &Fetcher{client: client}
}

// Fetch 抓取一篇文章的原文页。返回的 error 只表示程序层面的错误（URL 不合法、
// 请求构造失败）；抓取本身的成败一律由 Result.Outcome 表达。
func (f *Fetcher) Fetch(ctx context.Context, articleURL string) (Result, error) {
	pageURL, err := url.ParseRequestURI(articleURL)
	if err != nil {
		return Result{}, fmt.Errorf("parse article URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Referer", searchReferer)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")

	resp, err := f.client.Do(req)
	if err != nil {
		return Result{Outcome: OutcomeUnreachable, Detail: err.Error()}, nil
	}
	defer resp.Body.Close()

	return Extract(resp.StatusCode, resp.Header.Get("Content-Type"), resp.Body, pageURL), nil
}
