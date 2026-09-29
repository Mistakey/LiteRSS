package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"LiteRSS/internal/library"
)

// translator is a fake model endpoint for full-text translation: it reads
// the JSON array of blocks in the user prompt and answers "译:" + each block.
// answer, when set, rewrites one answer's blocks.
type translator struct {
	mu      sync.Mutex
	batches [][]string
	answer  func(n int, blocks []string) ([]string, int)
}

func newTranslator(t *testing.T, e *env) *translator {
	t.Helper()
	tr := &translator{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		var blocks []string
		for _, m := range req.Messages {
			if m.Role == "user" {
				if err := json.Unmarshal([]byte(m.Content), &blocks); err != nil {
					t.Errorf("user prompt is not a JSON array of blocks: %q", m.Content)
				}
			}
		}
		tr.mu.Lock()
		tr.batches = append(tr.batches, blocks)
		n := len(tr.batches)
		tr.mu.Unlock()

		out := make([]string, len(blocks))
		for i, b := range blocks {
			out[i] = "译:" + b
		}
		status := http.StatusOK
		if tr.answer != nil {
			out, status = tr.answer(n, out)
		}
		if status != http.StatusOK {
			http.Error(w, `{"error":{"message":"boom"}}`, status)
			return
		}
		content, _ := json.Marshal(out)
		// Models like to fence their JSON; the parser has to cope.
		reply := "```json\n" + string(content) + "\n```"
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	e.useModel(srv.URL)
	return tr
}

func (tr *translator) calls() int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return len(tr.batches)
}

// longBlocks makes n paragraphs of about 400 characters each.
func longBlocks(n int) []string {
	blocks := make([]string, n)
	for i := range blocks {
		blocks[i] = fmt.Sprintf("Paragraph %d. ", i) + strings.Repeat("The committee reviews the budget. ", 12)
	}
	return blocks
}

func TestTranslateSendsLongArticlesInBatchesAndKeepsBlockOrder(t *testing.T) {
	e := newEnv(t)
	tr := newTranslator(t, e)
	e.article(t, 1, "https://x/1", "Budget", "")
	blocks := longBlocks(30) // about 12000 characters: more than one batch

	got, err := e.svc.Translate(context.Background(), 1, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message != "" || len(got.Blocks) != len(blocks) {
		t.Fatalf("got %d blocks, message %q; want %d blocks", len(got.Blocks), got.Message, len(blocks))
	}
	for i, b := range blocks {
		if got.Blocks[i] != "译:"+b {
			t.Fatalf("block %d = %q, want the translation of %q", i, got.Blocks[i], b)
		}
	}
	if tr.calls() < 2 {
		t.Fatalf("the model got %d request(s); a long article goes in batches", tr.calls())
	}
	sent := 0
	for _, batch := range tr.batches {
		runes := 0
		for _, b := range batch {
			runes += len([]rune(b))
		}
		if len(batch) > 1 && runes > maxBatchRunes {
			t.Errorf("a batch of %d blocks carries %d characters, over %d", len(batch), runes, maxBatchRunes)
		}
		sent += len(batch)
	}
	if sent != len(blocks) {
		t.Fatalf("the batches carried %d blocks, want each of the %d once", sent, len(blocks))
	}
}

func TestTranslateCachesAndAnswersFromTheCache(t *testing.T) {
	e := newEnv(t)
	tr := newTranslator(t, e)
	e.article(t, 1, "https://x/1", "Fox", "")
	blocks := []string{"The quick brown fox.", "It jumps."}

	first, err := e.svc.Translate(context.Background(), 1, blocks)
	if err != nil || first.Message != "" {
		t.Fatalf("first: %+v, %v", first, err)
	}
	again, err := e.svc.Translate(context.Background(), 1, blocks)
	if err != nil {
		t.Fatal(err)
	}
	if tr.calls() != 1 || strings.Join(again.Blocks, "|") != "译:The quick brown fox.|译:It jumps." {
		t.Fatalf("second call: %+v after %d model requests; want the cached translation", again, tr.calls())
	}

	// Other blocks (the body changed) are not answered from the old translation.
	if _, err := e.svc.Translate(context.Background(), 1, []string{"Another body."}); err != nil {
		t.Fatal(err)
	}
	if tr.calls() != 2 {
		t.Fatalf("model requests = %d; a different body must be translated anew", tr.calls())
	}
}

func TestTranslateFailsWhenTheBlockCountDiffers(t *testing.T) {
	e := newEnv(t)
	tr := newTranslator(t, e)
	tr.answer = func(_ int, out []string) ([]string, int) { return out[:len(out)-1], http.StatusOK }
	e.article(t, 1, "https://x/1", "Fox", "")

	got, err := e.svc.Translate(context.Background(), 1, []string{"One.", "Two."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 0 || got.Message != "模型返回的译文和原文段落对不上，请稍后再试。" {
		t.Fatalf("got %+v", got)
	}
	if _, ok := e.stored(t, `SELECT blocks FROM article_translations WHERE item_id = ?`, 1); ok {
		t.Fatal("a mismatched translation was cached")
	}
}

func TestTranslateFailsWholeWhenOneBatchFails(t *testing.T) {
	e := newEnv(t)
	tr := newTranslator(t, e)
	tr.answer = func(n int, out []string) ([]string, int) {
		if n == 2 {
			return nil, http.StatusInternalServerError
		}
		return out, http.StatusOK
	}
	e.article(t, 1, "https://x/1", "Budget", "")

	got, err := e.svc.Translate(context.Background(), 1, longBlocks(30))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 0 || got.Message != "全文翻译失败，请检查设置里的大模型，或稍后再试。" {
		t.Fatalf("got %+v", got)
	}
	if _, ok := e.stored(t, `SELECT blocks FROM article_translations WHERE item_id = ?`, 1); ok {
		t.Fatal("half a translation was cached")
	}
}

func TestTranslateWithoutModelSaysSo(t *testing.T) {
	e := newEnv(t)
	e.article(t, 1, "https://x/1", "Fox", "")

	got, err := e.svc.Translate(context.Background(), 1, []string{"One."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 0 || got.Message != "还没有配置大模型，请在设置里填写。" {
		t.Fatalf("got %+v", got)
	}
}

func TestTranslateRejectsBadRequests(t *testing.T) {
	e := newEnv(t)
	e.article(t, 1, "https://x/1", "Fox", "")

	for name, c := range map[string]struct {
		id     int64
		blocks []string
		want   error
	}{
		"no blocks":     {1, nil, library.ErrBadRequest},
		"empty block":   {1, []string{"One.", "  "}, library.ErrBadRequest},
		"too many":      {1, make([]string, maxTranslateBlocks+1), library.ErrBadRequest},
		"no such items": {99, []string{"One."}, library.ErrNotFound},
	} {
		if _, err := e.svc.Translate(context.Background(), c.id, c.blocks); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

func TestFullTextDropsTheTranslationItReplaces(t *testing.T) {
	e := newEnv(t)
	web, _ := fakeWeb(t)
	tr := newTranslator(t, e)
	e.article(t, 1, web.URL+"/ok", "Fox", "<p>Teaser.</p>")

	if _, err := e.svc.Translate(context.Background(), 1, []string{"Teaser."}); err != nil {
		t.Fatal(err)
	}
	e.cacheFullText(t, 1)
	if _, ok := e.stored(t, `SELECT blocks FROM article_translations WHERE item_id = ?`, 1); ok {
		t.Fatal("the RSS body's translation outlived the full text")
	}
	if tr.calls() != 1 {
		t.Fatalf("model requests = %d", tr.calls())
	}
}

func TestRSSTranslationIsNotStoredOverAFullTextCachedMeanwhile(t *testing.T) {
	e := newEnv(t)
	tr := newTranslator(t, e)
	e.article(t, 1, "https://x/1", "Fox", "<p>Teaser.</p>")
	tr.answer = func(_ int, out []string) ([]string, int) {
		// The reader fetches the full text while the model is translating.
		e.exec(t, `INSERT INTO fulltext_cache (item_id, content, cached_at) VALUES (1, '<p>full</p>', 1)`)
		return out, http.StatusOK
	}

	got, err := e.svc.Translate(context.Background(), 1, []string{"Teaser."})
	if err != nil || len(got.Blocks) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, ok := e.stored(t, `SELECT blocks FROM article_translations WHERE item_id = ?`, 1); ok {
		t.Fatal("an RSS translation was stored after the full text arrived")
	}

	// Blocks sent once the full text is shown are stored.
	tr.answer = nil
	if _, err := e.svc.Translate(context.Background(), 1, []string{"Full."}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.stored(t, `SELECT blocks FROM article_translations WHERE item_id = ?`, 1); !ok {
		t.Fatal("the full text's translation was not stored")
	}
}
