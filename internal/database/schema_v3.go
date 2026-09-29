package database

import (
	"context"
	"database/sql"
)

// schemaV3 keeps an article's full-text translation (spec D21): the model's
// Chinese for each text block of the body the reader showed, as a JSON array.
// source_hash identifies those source blocks, so a translation is only
// reused for the same body. The rows are local like summaries, and leave with
// their article (spec D9).
func schemaV3(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE article_translations (
		item_id     INTEGER PRIMARY KEY REFERENCES articles(item_id) ON DELETE CASCADE,
		source_hash TEXT    NOT NULL,
		blocks      TEXT    NOT NULL,
		created_at  INTEGER NOT NULL
	)`)
	return err
}
