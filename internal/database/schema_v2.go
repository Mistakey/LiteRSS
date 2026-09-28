package database

import (
	"context"
	"database/sql"
)

// schemaV2 keeps a summary's note: what it is based on when that is not the
// full text (spec D11), so reopening the summary still says so. Imported and
// earlier summaries have none.
func schemaV2(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE summaries ADD COLUMN note TEXT NOT NULL DEFAULT ''`)
	return err
}
