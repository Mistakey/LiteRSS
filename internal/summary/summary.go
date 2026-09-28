// Package summary asks the configured model for a Chinese summary of an
// article and renders the Markdown it answers (spec D11).
package summary

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	"LiteRSS/internal/ai"
)

// systemPrompt is fixed: the prompt is not a setting, and the summary is
// always Chinese whatever the article's language (spec D11).
const systemPrompt = "你是文章摘要助手。无论原文是什么语言，都只用简体中文写摘要。" +
	"先用一两句话概括文章讲了什么，再用 3 到 6 条要点列出关键事实、数据与结论。" +
	"全文不超过 300 字，使用 Markdown，不写标题，不加评论，只输出摘要本身。"

// Model is the one model configuration (spec D10).
type Model struct {
	Endpoint string
	Name     string
	APIKey   string
	// HTTP carries the request; the caller passes the shared outbound client
	// (pitfall 8).
	HTTP *http.Client
}

// Summarize returns the model's Markdown summary of an article.
func (m Model) Summarize(ctx context.Context, title, text string) (string, error) {
	user := fmt.Sprintf("请为下面这篇文章写中文摘要。\n\n标题：%s\n\n正文：\n%s", title, text)
	result, err := m.client().RequestWithThinking(ctx, systemPrompt, user)
	if err != nil {
		return "", err
	}
	summary := ai.RemoveThinkingTags(result.Content)
	if summary == "" {
		return "", errors.New("the model answered an empty summary")
	}
	return summary, nil
}

// Check sends the model a one-word request, telling whether the
// configuration answers at all; the settings panel's connection test.
func (m Model) Check(ctx context.Context) error {
	_, err := m.client().Request(ctx, "只回复 OK。", "OK")
	return err
}

func (m Model) client() *ai.Client {
	return ai.NewClientWithHTTPClient(ai.ClientConfig{
		APIKey:   m.APIKey,
		Endpoint: strings.TrimSuffix(m.Endpoint, "/"),
		Model:    m.Name,
	}, m.HTTP)
}

// RenderHTML renders a summary's Markdown. Raw HTML in it is dropped, not
// passed through, and links keep only safe schemes; the frontend still
// sanitizes the result (spec D16).
func RenderHTML(md string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions)
	renderer := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags | html.SkipHTML | html.Safelink})
	return string(markdown.ToHTML([]byte(md), p, renderer))
}
