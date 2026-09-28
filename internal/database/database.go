// Package database owns the local SQLite library: opening it, its schema and
// the versioned migrations that grow the schema (spec D9).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// DB is the opened, migrated local library.
type DB struct {
	*sql.DB
}

// Open opens the library at path, creating it when absent, and brings its
// schema up to the latest version.
func Open(ctx context.Context, path string) (*DB, error) {
	db, err := openRaw(path)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{DB: db}, nil
}

// connectionPragmas run on every new connection, in order. auto_vacuum comes
// first: it only takes effect on a file that has no tables yet, which lets the
// retention cleanup hand space back with incremental_vacuum.
var connectionPragmas = []string{
	"auto_vacuum(incremental)",
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"foreign_keys(1)",
}

func openRaw(path string) (*sql.DB, error) {
	q := url.Values{}
	for _, p := range connectionPragmas {
		q.Add("_pragma", p)
	}
	db, err := sql.Open("sqlite", path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return db, nil
}
