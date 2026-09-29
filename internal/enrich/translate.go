package enrich

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"LiteRSS/internal/library"
	"LiteRSS/internal/summary"
)

const (
	// maxTranslateBlocks bounds one request; a very long article is still
	// far below it.
	maxTranslateBlocks = 3000
	// maxBatchRunes and maxBatchBlocks bound one model request, so its
	// answer fits the model's output limit and its time the client timeout.
	// A single block longer than that goes alone.
	maxBatchRunes  = 3000
	maxBatchBlocks = 40
	// translateWorkers is how many batches are at the model at once.
	translateWorkers = 3
)

// Translation answers a full-text translation. Blocks are the Chinese of the
// given blocks, one each in order, plain text; empty when there is none, and
// Message then says why in Chinese.
type Translation struct {
	Blocks  []string `json:"blocks"`
	Message string   `json:"message"`
}

// Translate returns the Chinese of an article's text blocks (spec D21). The
// frontend extracts the blocks from the body the reader shows, so a stored
// translation is reused only for the same blocks. Long bodies go to the
// model in batches; any failed batch fails the whole translation, and
// nothing is stored.
func (s *Service) Translate(ctx context.Context, id int64, blocks []string) (Translation, error) {
	if len(blocks) == 0 || len(blocks) > maxTranslateBlocks {
		return Translation{}, fmt.Errorf("between 1 and %d blocks: %w", maxTranslateBlocks, library.ErrBadRequest)
	}
	for _, b := range blocks {
		if strings.TrimSpace(b) == "" {
			return Translation{}, fmt.Errorf("an empty block: %w", library.ErrBadRequest)
		}
	}
	source, err := json.Marshal(blocks)
	if err != nil {
		return Translation{}, err
	}
	sum := sha256.Sum256(source)
	hash := hex.EncodeToString(sum[:])

	// hadFullText tells whether these blocks can be from the full text; if
	// not, they are the RSS body's, and a full text arriving meanwhile
	// invalidates them (spec D21).
	var hadFullText bool
	var stored sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT f.item_id IS NOT NULL, t.blocks FROM articles a
		 LEFT JOIN fulltext_cache f ON f.item_id = a.item_id
		 LEFT JOIN article_translations t ON t.item_id = a.item_id AND t.source_hash = ?
		 WHERE a.item_id = ?`, hash, id).Scan(&hadFullText, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return Translation{}, fmt.Errorf("article %d: %w", id, library.ErrNotFound)
	}
	if err != nil {
		return Translation{}, fmt.Errorf("read translation: %w", err)
	}
	if stored.Valid {
		var out []string
		if err := json.Unmarshal([]byte(stored.String), &out); err == nil && len(out) == len(blocks) {
			return Translation{Blocks: out}, nil
		}
		log.Printf("Translation for %d: the stored one is unreadable; translating again", id)
	}

	model, ok, err := s.model(ctx)
	if err != nil {
		return Translation{}, err
	}
	if !ok {
		return Translation{Message: noModel}, nil
	}

	out, err := translateBatches(ctx, model, blocks)
	if err != nil {
		if ctx.Err() != nil {
			return Translation{}, ctx.Err()
		}
		log.Printf("Translation for %d: %v", id, err)
		if errors.Is(err, summary.ErrBlockCount) {
			return Translation{Message: "模型返回的译文和原文段落对不上，请稍后再试。"}, nil
		}
		return Translation{Message: "全文翻译失败，请检查设置里的大模型，或稍后再试。"}, nil
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return Translation{}, err
	}
	// Kept even when the reader has left meanwhile, but not one of the RSS
	// body once a full text has arrived: that one replaced it. The article may
	// be gone (retention cleanup), and then nothing is stored.
	if _, err := s.db.ExecContext(context.WithoutCancel(ctx),
		`INSERT INTO article_translations (item_id, source_hash, blocks, created_at)
		 SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM articles WHERE item_id = ?)
		   AND NOT (? AND EXISTS (SELECT 1 FROM fulltext_cache WHERE item_id = ?))
		 ON CONFLICT (item_id) DO UPDATE SET source_hash = excluded.source_hash, blocks = excluded.blocks, created_at = excluded.created_at`,
		id, hash, string(encoded), time.Now().Unix(), id, !hadFullText, id); err != nil {
		return Translation{}, fmt.Errorf("store translation: %w", err)
	}
	return Translation{Blocks: out}, nil
}

// translateBatches sends blocks to the model in batches, a few at a time,
// and stops at the first failure.
func translateBatches(ctx context.Context, model summary.Model, blocks []string) ([]string, error) {
	type batch struct{ start, end int }
	var batches []batch
	for start := 0; start < len(blocks); {
		end, runes := start, 0
		for end < len(blocks) && end-start < maxBatchBlocks {
			n := len([]rune(blocks[end]))
			if end > start && runes+n > maxBatchRunes {
				break
			}
			runes += n
			end++
		}
		batches = append(batches, batch{start, end})
		start = end
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make([]string, len(blocks))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	sem := make(chan struct{}, translateWorkers)
	for _, b := range batches {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			got, err := model.Translate(ctx, blocks[b.start:b.end])
			if err != nil {
				once.Do(func() { firstErr = err; cancel() })
				return
			}
			copy(out[b.start:b.end], got)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
