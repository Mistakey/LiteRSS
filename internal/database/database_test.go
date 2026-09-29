package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func tempDBPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "literss.db")
}

// openTemp opens a fresh library in a temp dir, closed when the test ends.
func openTemp(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), tempDBPath(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func TestOpenCreatesLatestSchemaVersion(t *testing.T) {
	db := openTemp(t)

	if got := userVersion(t, db.DB); got != len(migrations) {
		t.Fatalf("user_version = %d, want %d", got, len(migrations))
	}

	want := []string{
		"feeds", "tags", "feed_tags",
		"articles", "article_contents", "fulltext_cache", "title_translations", "summaries",
		"article_translations", "pending_read", "pending_mark_all", "meta", "settings",
	}
	for _, name := range want {
		var n int
		err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n)
		if err != nil {
			t.Fatalf("look up table %s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("table %s missing", name)
		}
	}

	var autoVacuum int
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&autoVacuum); err != nil {
		t.Fatalf("read auto_vacuum: %v", err)
	}
	if autoVacuum != 2 {
		t.Errorf("auto_vacuum = %d, want 2 (incremental)", autoVacuum)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := tempDBPath(t)
	for i := 0; i < 2; i++ {
		db, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i+1, err)
		}
		if got := userVersion(t, db.DB); got != len(migrations) {
			t.Fatalf("Open #%d: user_version = %d, want %d", i+1, got, len(migrations))
		}
		db.Close()
	}
}

func TestDeletingArticleCascadesToLocalTables(t *testing.T) {
	db := openTemp(t)

	const id = 1_700_000_000_000_000
	stmts := []string{
		`INSERT INTO articles (item_id, stream_id, url, title, published_at) VALUES (?, 'feed/1', 'https://example.com/a', 'A', 1700000000)`,
		`INSERT INTO article_contents (item_id, content) VALUES (?, '<p>rss</p>')`,
		`INSERT INTO fulltext_cache (item_id, content, cached_at) VALUES (?, '<p>full</p>', 1700000100)`,
		`INSERT INTO title_translations (item_id, translated_title) VALUES (?, '甲')`,
		`INSERT INTO summaries (item_id, summary, created_at) VALUES (?, '摘要', 1700000200)`,
		`INSERT INTO article_translations (item_id, source_hash, blocks, created_at) VALUES (?, 'h', '["甲"]', 1700000300)`,
		`INSERT INTO pending_read (item_id, value, seq) VALUES (?, 1, 1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s, id); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM articles WHERE item_id = ?`, id); err != nil {
		t.Fatalf("delete article: %v", err)
	}
	for _, table := range []string{"article_contents", "fulltext_cache", "title_translations", "summaries", "article_translations", "pending_read"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s still has %d rows after the article was deleted", table, n)
		}
	}
}

func TestTitleTranslationRejectsEmpty(t *testing.T) {
	db := openTemp(t)

	if _, err := db.Exec(`INSERT INTO articles (item_id, stream_id, published_at) VALUES (1, 'feed/1', 1)`); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO title_translations (item_id, translated_title) VALUES (1, '')`); err == nil {
		t.Fatal("empty translated_title accepted; a missing row is the only undecided state")
	}
}
