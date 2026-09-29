package database

import (
	"context"
	"database/sql"
)

// schemaV4 caches a feed's icon from FreshRSS's favicon cache (spec D15).
// icon_url is the iconUrl the icon was fetched for, so a changed iconUrl
// fetches again. An empty data records that FreshRSS has no icon for the feed
// (only its placeholder, or nothing usable), and fetched_at says when that was
// last asked. The rows are local and leave with their feed.
func schemaV4(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE feed_icons (
		stream_id    TEXT    PRIMARY KEY REFERENCES feeds(stream_id) ON DELETE CASCADE,
		icon_url     TEXT    NOT NULL,
		data         BLOB    NOT NULL,
		content_type TEXT    NOT NULL,
		fetched_at   INTEGER NOT NULL
	)`)
	return err
}
