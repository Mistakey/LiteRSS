package settings

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"LiteRSS/internal/config"
	"LiteRSS/internal/crypto"
	"LiteRSS/internal/database"
)

func openStore(t *testing.T) (*Store, *database.DB, *[]string) {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "literss.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var logs []string
	s := New(db.DB)
	s.logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	return s, db, &logs
}

func storedValue(t *testing.T, db *database.DB, key string) string {
	t.Helper()
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		t.Fatalf("read stored %s: %v", key, err)
	}
	return v
}

func TestLoadReturnsDefaultsForEmptyLibrary(t *testing.T) {
	s, _, _ := openStore(t)
	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["freshrss_auto_sync_interval"] != "30" || got["proxy_mode"] != "system" || got["llm_api_key"] != "" {
		t.Errorf("Load defaults = %v", got)
	}
	if len(got) != len(config.SettingsKeys()) {
		t.Errorf("Load returned %d keys, want every schema key", len(got))
	}
}

func TestUpdateRejectsUnknownKeysAndWritesNothing(t *testing.T) {
	s, db, _ := openStore(t)
	err := s.Update(context.Background(), map[string]string{
		"freshrss_username": "alice",
		"ai_endpoint":       "https://example.com",
		"language":          "zh-CN",
	})

	var unknown *UnknownKeysError
	if !errors.As(err, &unknown) {
		t.Fatalf("Update error = %v, want *UnknownKeysError", err)
	}
	if strings.Join(unknown.Keys, ",") != "ai_endpoint,language" {
		t.Errorf("unknown keys = %v", unknown.Keys)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a rejected update wrote %d rows", n)
	}
}

func TestUpdateRejectsValuesOfTheWrongType(t *testing.T) {
	s, _, _ := openStore(t)
	for key, value := range map[string]string{
		"freshrss_auto_sync_interval": "often",
		"close_to_tray":               "yes",
		"window_maximized":            "1",
	} {
		err := s.Update(context.Background(), map[string]string{key: value})
		var invalid *InvalidValueError
		if !errors.As(err, &invalid) || invalid.Key != key {
			t.Errorf("Update(%s=%q) error = %v, want *InvalidValueError", key, value, err)
		}
	}
}

func TestUpdateThenLoad(t *testing.T) {
	s, _, _ := openStore(t)
	ctx := context.Background()
	if err := s.Update(ctx, map[string]string{"freshrss_username": "alice", "close_to_tray": "false"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := s.Update(ctx, map[string]string{"freshrss_username": "bob"}); err != nil {
		t.Fatalf("second Update: %v", err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["freshrss_username"] != "bob" || got["close_to_tray"] != "false" {
		t.Errorf("Load = %v", got)
	}
	if v, err := s.Get(ctx, "freshrss_username"); err != nil || v != "bob" {
		t.Errorf("Get = %q, %v", v, err)
	}
}

func TestLoadIgnoresAndLogsUnknownStoredKeys(t *testing.T) {
	s, db, logs := openStore(t)
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('ai_api_key', 'sk-secret'), ('freshrss_username', 'alice')`); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := got["ai_api_key"]; ok {
		t.Errorf("Load returned unknown key ai_api_key")
	}
	if got["freshrss_username"] != "alice" {
		t.Errorf("known key lost: %v", got)
	}
	if len(*logs) != 1 || !strings.Contains((*logs)[0], "ai_api_key") {
		t.Errorf("logs = %q, want one line naming ai_api_key", *logs)
	}
	for _, l := range *logs {
		if strings.Contains(l, "sk-secret") {
			t.Errorf("log leaks the stored value: %q", l)
		}
	}
}

func TestEncryptedSettingsRoundTrip(t *testing.T) {
	s, db, _ := openStore(t)
	ctx := context.Background()
	if err := s.Update(ctx, map[string]string{"llm_api_key": "sk-live-123", "proxy_password": ""}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	stored := storedValue(t, db, "llm_api_key")
	if stored == "sk-live-123" || strings.Contains(stored, "sk-live-123") {
		t.Fatalf("credential stored in clear: %q", stored)
	}
	if plain, err := crypto.Decrypt(stored); err != nil || plain != "sk-live-123" {
		t.Errorf("stored value does not decrypt with crypto.Decrypt: %q, %v", plain, err)
	}
	if storedValue(t, db, "proxy_password") != "" {
		t.Errorf("empty credential is not stored empty")
	}

	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["llm_api_key"] != "sk-live-123" {
		t.Errorf("Load llm_api_key = %q", got["llm_api_key"])
	}
}

func TestLoadDropsCredentialThatFailsToDecrypt(t *testing.T) {
	s, db, logs := openStore(t)
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('baidu_secret_key', 'plain-not-encrypted')`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got["baidu_secret_key"] != "" {
		t.Errorf("undecryptable credential = %q, want empty", got["baidu_secret_key"])
	}
	if len(*logs) != 1 || strings.Contains((*logs)[0], "plain-not-encrypted") {
		t.Errorf("logs = %q, want one line without the value", *logs)
	}
}

func TestGetRejectsUnknownKey(t *testing.T) {
	s, _, _ := openStore(t)
	_, err := s.Get(context.Background(), "theme")
	var unknown *UnknownKeysError
	if !errors.As(err, &unknown) {
		t.Errorf("Get(theme) error = %v, want *UnknownKeysError", err)
	}
}

func TestUpdateTxCommitsWithTheCallersTransaction(t *testing.T) {
	store, db, _ := openStore(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateTx(ctx, tx, map[string]string{"freshrss_username": "reader"}); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	if got, _ := store.Get(ctx, "freshrss_username"); got != "" {
		t.Errorf("a rolled-back UpdateTx left %q", got)
	}
}

func TestCheck(t *testing.T) {
	var unknown *UnknownKeysError
	var invalid *InvalidValueError
	if err := Check("ai_endpoint", "x"); !errors.As(err, &unknown) {
		t.Errorf("Check(unknown) = %v", err)
	}
	if err := Check("window_x", "left"); !errors.As(err, &invalid) {
		t.Errorf("Check(bad int) = %v", err)
	}
	if err := Check("window_x", "-1920"); err != nil {
		t.Errorf("Check(valid) = %v", err)
	}
}

func TestMinimizedWindowPos(t *testing.T) {
	tests := []struct {
		x, y int
		want bool
	}{
		{-32000, -32000, true},
		{-21333, -21333, true},
		{-16000, 100, true}, // -32000 at 200%
		{-3840, 0, false},   // a monitor left of two 4K screens
		{0, -1080, false},
		{100, 100, false},
	}
	for _, tt := range tests {
		if got := MinimizedWindowPos(tt.x, tt.y); got != tt.want {
			t.Errorf("MinimizedWindowPos(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}
