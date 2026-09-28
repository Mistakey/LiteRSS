package database

import (
	"context"
	"database/sql"
	"testing"
)

// A sample version 2 appended after the real version 1, the way a later step adds one.
func TestMigrateAppendsVersion2Once(t *testing.T) {
	ctx := context.Background()
	path := tempDBPath(t)

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('before_v2', 'kept')`); err != nil {
		t.Fatalf("seed v1 data: %v", err)
	}

	runs := 0
	v2 := func(ctx context.Context, tx *sql.Tx) error {
		runs++
		_, err := tx.ExecContext(ctx, `ALTER TABLE summaries ADD COLUMN model TEXT NOT NULL DEFAULT ''`)
		return err
	}
	steps := append(migrations[:len(migrations):len(migrations)], v2)

	for i := 0; i < 2; i++ {
		if err := migrate(ctx, db.DB, steps); err != nil {
			t.Fatalf("migrate #%d: %v", i+1, err)
		}
	}
	db.Close()

	// A restart runs the same list again.
	db2, err := openRaw(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	if err := migrate(ctx, db2, steps); err != nil {
		t.Fatalf("migrate after restart: %v", err)
	}

	if runs != 1 {
		t.Errorf("version 2 ran %d times, want 1", runs)
	}
	if got := userVersion(t, db2); got != len(migrations)+1 {
		t.Errorf("user_version = %d, want %d", got, len(migrations)+1)
	}
	var kept string
	if err := db2.QueryRow(`SELECT value FROM meta WHERE key = 'before_v2'`).Scan(&kept); err != nil || kept != "kept" {
		t.Errorf("v1 data after upgrade = %q, %v", kept, err)
	}
	if _, err := db2.Exec(`SELECT model FROM summaries LIMIT 0`); err != nil {
		t.Errorf("version 2 column missing: %v", err)
	}
}

func TestMigrateFailureLeavesVersionUnchanged(t *testing.T) {
	db := openTemp(t)

	broken := func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE half_done (x INTEGER)`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `THIS IS NOT SQL`)
		return err
	}
	steps := append(migrations[:len(migrations):len(migrations)], broken)
	if err := migrate(context.Background(), db.DB, steps); err == nil {
		t.Fatal("broken migration reported success")
	}
	if got := userVersion(t, db.DB); got != len(migrations) {
		t.Errorf("user_version = %d after failed migration, want %d", got, len(migrations))
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'half_done'`).Scan(&n); err != nil {
		t.Fatalf("look up half_done: %v", err)
	}
	if n != 0 {
		t.Error("failed migration left its partial changes behind")
	}
}

func TestMigrateRefusesNewerDatabase(t *testing.T) {
	db := openTemp(t)
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if err := migrate(context.Background(), db.DB, migrations); err == nil {
		t.Fatal("database from a newer version accepted")
	}
}
