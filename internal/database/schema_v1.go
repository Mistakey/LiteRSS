package database

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaV1 is the first LiteRSS schema. Column authority is carried by the
// table a column lives in (spec D6, D9):
//
//   - feeds, tags, feed_tags and every articles column are FreshRSS-owned and
//     written only by the sync pull (and once by the legacy import, spec D12);
//     server_read is a mirror of the server, which a confirmed push also sets
//     to the value it sent until the next pull.
//   - article_contents holds the RSS body the pull delivered; fulltext_cache,
//     title_translations and summaries are local and never written by sync.
//   - pending_read and pending_mark_all are the user's unpushed intents; the
//     displayed read state is the intent when one exists, else server_read.
//   - meta and settings are local key-value tables: meta for records such as
//     the last successful sync, settings for what the user configures.
//
// Rows keyed by item_id go away with their article (ON DELETE CASCADE).
// Articles carry stream_id without a foreign key: imported articles arrive
// before the first sync has rebuilt feeds.
func schemaV1(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE feeds (
			stream_id TEXT PRIMARY KEY,
			title     TEXT NOT NULL DEFAULT '',
			url       TEXT NOT NULL DEFAULT '',
			site_url  TEXT NOT NULL DEFAULT '',
			icon_url  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE tags (
			tag_id TEXT PRIMARY KEY,
			label  TEXT NOT NULL
		)`,
		`CREATE TABLE feed_tags (
			stream_id TEXT NOT NULL REFERENCES feeds(stream_id) ON DELETE CASCADE,
			tag_id    TEXT NOT NULL REFERENCES tags(tag_id) ON DELETE CASCADE,
			PRIMARY KEY (stream_id, tag_id)
		)`,
		`CREATE INDEX idx_feed_tags_tag ON feed_tags(tag_id)`,

		// item_id is the FreshRSS item ID in decimal; its value is the fetch
		// time in microseconds (FetchedAt). published_at is Unix seconds,
		// normalized by NormalizePublishedAt. Every other *_at column is Unix
		// seconds too.
		`CREATE TABLE articles (
			item_id      INTEGER PRIMARY KEY,
			stream_id    TEXT    NOT NULL,
			url          TEXT    NOT NULL DEFAULT '',
			title        TEXT    NOT NULL DEFAULT '',
			image_url    TEXT    NOT NULL DEFAULT '',
			published_at INTEGER NOT NULL,
			server_read  INTEGER NOT NULL DEFAULT 0 CHECK (server_read IN (0, 1))
		)`,
		`CREATE INDEX idx_articles_order ON articles(published_at DESC, item_id DESC)`,
		`CREATE INDEX idx_articles_stream ON articles(stream_id, published_at DESC, item_id DESC)`,
		`CREATE INDEX idx_articles_url ON articles(url)`,

		`CREATE TABLE article_contents (
			item_id INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
			content TEXT NOT NULL
		)`,
		`CREATE TABLE fulltext_cache (
			item_id    INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
			content    TEXT    NOT NULL,
			cached_at  INTEGER NOT NULL
		)`,
		// A row means "decided": a value equal to the title means the title
		// was judged already Chinese. No row is the only undecided state
		// (pitfall 12), so an empty value is refused.
		`CREATE TABLE title_translations (
			item_id          INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
			translated_title TEXT NOT NULL CHECK (translated_title <> '')
		)`,
		`CREATE TABLE summaries (
			item_id    INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
			summary    TEXT    NOT NULL CHECK (summary <> ''),
			created_at INTEGER NOT NULL
		)`,

		// seq comes from one counter across both intent tables (meta
		// intent_seq), so a push deletes only the intent it sent. ts of
		// pending_mark_all is an item id, the bound mark-all-as-read cuts at
		// (pitfall 30).
		`CREATE TABLE pending_read (
			item_id    INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
			value      INTEGER NOT NULL CHECK (value IN (0, 1)),
			seq        INTEGER NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0,
			last_error TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE pending_mark_all (
			id         INTEGER PRIMARY KEY,
			stream_id  TEXT    NOT NULL,
			ts         INTEGER NOT NULL,
			seq        INTEGER NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0,
			last_error TEXT    NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		// Settings keys come from internal/config's schema; the settings
		// package refuses any other key and stores credentials encrypted.
		`CREATE TABLE settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}
	for i, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("schema v1 statement %d: %w", i, err)
		}
	}
	return nil
}
