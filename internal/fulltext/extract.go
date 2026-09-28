// Package fulltext 抓取文章原文页并提取正文。
//
// 抓取分两层：请求层（fetch.go）负责网络、代理与请求头；提取与分类层（Extract）
// 不接触网络，只看一个 HTTP 响应的状态码、Content-Type 与 body。分层是为了让
// 「结论属于哪一类」可以被离线测试穷尽，而不必依赖某个站点当天的脾气。
package fulltext

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"
	"unicode"

	"codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html"
)

// Outcome 是一次抓取的结论。抓不到并不是「出错了」——它是一个有内容的结论，
// 四类失败对应完全不同的处置方式（换个入口读 / 去浏览器读 / 值得开适配 Issue /
// 检查网络），混进同一个 500 里就等于什么也没说。
type Outcome string

const (
	minimumVisibleTextCharacters = 50

	// OutcomeSuccess 提到了正文。
	OutcomeSuccess Outcome = "success"
	// OutcomeNoContent 这一页本来就没有文章：视频页、PDF、SPA 落地页。
	OutcomeNoContent Outcome = "no_content"
	// OutcomeBlocked 请求到达了，站点拒绝作答。
	OutcomeBlocked Outcome = "blocked"
	// OutcomeParseFailed 内容拿到了，提不出来。
	OutcomeParseFailed Outcome = "parse_failed"
	// OutcomeUnreachable 传输层就没成功。
	OutcomeUnreachable Outcome = "unreachable"
)

// Result 是一次抓取的完整结论。Detail 是给日志看的技术细节，不面向用户——
// 用户看到的是 Outcome 对应的那句说明。
type Result struct {
	Outcome Outcome
	Content string
	Detail  string
}

// Extract 把一个 HTTP 响应判成正文或四类失败之一。它不接触网络，因此
// 「什么样的响应算哪一类」可以被离线测试完整覆盖。
func Extract(statusCode int, contentType string, body io.Reader, pageURL *url.URL) Result {
	if statusCode < 200 || statusCode >= 300 {
		return Result{Outcome: OutcomeBlocked, Detail: fmt.Sprintf("HTTP %d", statusCode)}
	}

	if !isHTML(contentType) {
		return Result{Outcome: OutcomeNoContent, Detail: "non-HTML content type: " + contentType}
	}

	article, err := readability.FromReader(body, pageURL)
	if err != nil {
		return Result{Outcome: OutcomeParseFailed, Detail: err.Error()}
	}

	// Node == nil 读起来像库出了问题，实际含义是 readability 判定这一页提不出
	// 文章正文（库源码注释：may be nil if there were errors or if article content
	// was blank）。见 AGENTS.md 第 21 条。
	if article.Node == nil {
		return Result{Outcome: OutcomeNoContent, Detail: "readability found no article node"}
	}

	var text bytes.Buffer
	if err := article.RenderText(&text); err != nil {
		return Result{Outcome: OutcomeParseFailed, Detail: err.Error()}
	}
	visibleCharacters := countVisibleCharacters(text.String())
	if visibleCharacters < minimumVisibleTextCharacters && !hasArticleMedia(article.Node) {
		return Result{
			Outcome: OutcomeNoContent,
			Detail:  fmt.Sprintf("readability content too short: %d visible characters", visibleCharacters),
		}
	}

	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return Result{Outcome: OutcomeParseFailed, Detail: err.Error()}
	}

	return Result{Outcome: OutcomeSuccess, Content: buf.String()}
}

func hasArticleMedia(node *html.Node) bool {
	if node.Type == html.ElementNode {
		switch node.Data {
		case "img", "picture", "audio", "video":
			return true
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if hasArticleMedia(child) {
			return true
		}
	}
	return false
}

func countVisibleCharacters(text string) int {
	count := 0
	for _, character := range text {
		if !unicode.IsSpace(character) {
			count++
		}
	}
	return count
}

// isHTML 判断这个响应值不值得交给 readability。判断偏宽松：缺失或畸形的
// Content-Type 一律当 HTML 放行，让 readability 去下结论；只有明确声明为
// 其它类型（PDF、图片、JSON）的才在解析前就挡掉。
func isHTML(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return true
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
