package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"LiteRSS/internal/ai"
)

// translatePrompt is fixed like the summary's: the translation is always
// Simplified Chinese plain text, one entry per source block (spec D21).
const translatePrompt = "你是翻译助手。用户会给出一个 JSON 字符串数组，每一项是文章里的一段文字。" +
	"把每一项翻译成通顺的简体中文，专有名词、代码与数字保持原样。" +
	"只输出一个 JSON 字符串数组：项数与输入相同，顺序一一对应，每项只是对应那一段的译文纯文本，" +
	"不带 HTML 或 Markdown，不合并也不拆分段落，不加任何解释。"

// translateMaxTokens leaves room for the Chinese of one batch; the summary's
// default of 2048 would cut a long batch short.
const translateMaxTokens = 8192

// ErrBlockCount is returned when the model answers a different number of
// blocks than it was given: the translation cannot be matched to the source.
var ErrBlockCount = errors.New("the model answered a different number of blocks")

// Translate returns the model's Chinese for each block, in order.
func (m Model) Translate(ctx context.Context, blocks []string) ([]string, error) {
	user, err := json.Marshal(blocks)
	if err != nil {
		return nil, err
	}
	result, err := m.client().RequestWithConfig(ctx, ai.RequestConfig{
		Model:        m.Name,
		SystemPrompt: translatePrompt,
		UserPrompt:   string(user),
		Temperature:  0.3,
		MaxTokens:    translateMaxTokens,
	})
	if err != nil {
		return nil, err
	}
	out, err := parseBlocks(ai.RemoveThinkingTags(result.Content))
	if err != nil {
		return nil, err
	}
	if len(out) != len(blocks) {
		return nil, fmt.Errorf("%w: sent %d, got %d", ErrBlockCount, len(blocks), len(out))
	}
	return out, nil
}

// parseBlocks reads the JSON string array in a model's answer, tolerating a
// code fence or words around it.
func parseBlocks(answer string) ([]string, error) {
	start, end := strings.Index(answer, "["), strings.LastIndex(answer, "]")
	if start < 0 || end < start {
		return nil, errors.New("the model answered no JSON array")
	}
	var out []string
	if err := json.Unmarshal([]byte(answer[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("the model's JSON array: %w", err)
	}
	return out, nil
}
