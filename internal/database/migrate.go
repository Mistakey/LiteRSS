package database

import (
	"context"
	"database/sql"
	"fmt"
)

// migration moves the schema from one version to the next inside tx.
type migration func(ctx context.Context, tx *sql.Tx) error

// migrations[i] takes the library from PRAGMA user_version i to i+1. The list
// only grows: a released step is never edited, reordered or removed, since a
// library that already ran it will not run it again.
var migrations = []migration{
	schemaV1,
	schemaV2,
	schemaV3,
}

// migrate runs every step the library has not run yet, one transaction per
// step. The version bump commits with the step, so a failed step leaves both
// the schema and user_version where they were.
func migrate(ctx context.Context, db *sql.DB, steps []migration) error {
	for {
		done, err := migrateNext(ctx, db, steps)
		if err != nil || done {
			return err
		}
	}
}

func migrateNext(ctx context.Context, db *sql.DB, steps []migration) (done bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin migration: %w", err)
	}
	defer func() {
		if err != nil || done {
			tx.Rollback()
		}
	}()

	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return false, fmt.Errorf("read schema version: %w", err)
	}
	if version > len(steps) {
		return false, fmt.Errorf("database schema version %d is newer than this build supports (%d)", version, len(steps))
	}
	if version == len(steps) {
		return true, nil
	}

	if err := steps[version](ctx, tx); err != nil {
		return false, fmt.Errorf("migrate schema to version %d: %w", version+1, err)
	}
	// PRAGMA takes no bound parameters; version is an int from this code.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version+1)); err != nil {
		return false, fmt.Errorf("set schema version %d: %w", version+1, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit schema version %d: %w", version+1, err)
	}
	return false, nil
}
