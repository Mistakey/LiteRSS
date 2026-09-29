// Package settings stores the user's settings in the library's settings table
// (spec D10). The key list is internal/config's schema and nothing else: a
// write naming any other key is refused whole, a stored row with any other key
// is ignored on read. Credentials are encrypted with internal/crypto.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"LiteRSS/internal/config"
	"LiteRSS/internal/crypto"
)

// UnknownKeysError names the keys outside the schema; the HTTP layer answers it
// with 400.
type UnknownKeysError struct {
	Keys []string
}

func (e *UnknownKeysError) Error() string {
	return "unknown setting keys: " + strings.Join(e.Keys, ", ")
}

func (e *UnknownKeysError) Is(target error) bool { return target == ErrInvalid }

// InvalidValueError is a value that does not parse as its setting's type. It
// carries no value: the value may be a credential.
type InvalidValueError struct {
	Key  string
	Type config.Type
}

func (e *InvalidValueError) Error() string {
	return fmt.Sprintf("setting %s must be a %s", e.Key, e.Type)
}

func (e *InvalidValueError) Is(target error) bool { return target == ErrInvalid }

// Store reads and writes the settings table.
type Store struct {
	db   *sql.DB
	logf func(format string, args ...any)
}

func New(db *sql.DB) *Store {
	return &Store{db: db, logf: log.Printf}
}

// Load returns every schema key: the stored value, decrypted, or the default.
// A credential that no longer decrypts (another user or machine) reads as
// empty, so the user is asked to fill it in again.
func (s *Store) Load(ctx context.Context) (map[string]string, error) {
	values := make(map[string]string)
	for _, key := range config.SettingsKeys() {
		values[key] = config.GetString(key)
	}

	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, stored string
		if err := rows.Scan(&key, &stored); err != nil {
			return nil, fmt.Errorf("load settings: %w", err)
		}
		def, ok := config.Lookup(key)
		if !ok {
			s.logf("settings: ignoring stored key %q outside the schema", key)
			continue
		}
		values[key] = s.open(key, def, stored)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	return values, nil
}

// Get returns one setting, or its default when it was never written.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	def, ok := config.Lookup(key)
	if !ok {
		return "", &UnknownKeysError{Keys: []string{key}}
	}
	var stored string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return config.GetString(key), nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return s.open(key, def, stored), nil
}

// Update writes values in one transaction. A key outside the schema or a
// value that does not parse as its type refuses the whole update.
func (s *Store) Update(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	defer tx.Rollback()
	if err := s.updateTx(ctx, tx, values); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// updateTx is Update inside tx.
func (s *Store) updateTx(ctx context.Context, tx *sql.Tx, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var unknown []string
	for _, key := range keys {
		if _, ok := config.Lookup(key); !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		return &UnknownKeysError{Keys: unknown}
	}

	stored := make(map[string]string, len(values))
	for _, key := range keys {
		if err := Check(key, values[key]); err != nil {
			return err
		}
		v := values[key]
		if def, _ := config.Lookup(key); def.Encrypted {
			enc, err := crypto.Encrypt(v)
			if err != nil {
				return fmt.Errorf("encrypt setting %s: %w", key, err)
			}
			v = enc
		}
		stored[key] = v
	}

	for _, key := range keys {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES (?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, stored[key]); err != nil {
			return fmt.Errorf("update setting %s: %w", key, err)
		}
	}
	return nil
}

// Check tells whether Update would accept value for key: an
// *UnknownKeysError or *InvalidValueError when it would not.
func Check(key, value string) error {
	def, ok := config.Lookup(key)
	if !ok {
		return &UnknownKeysError{Keys: []string{key}}
	}
	if !validValue(def.Type, value) {
		return &InvalidValueError{Key: key, Type: def.Type}
	}
	return nil
}

// open turns a stored value into the plain one.
func (s *Store) open(key string, def config.Setting, stored string) string {
	if !def.Encrypted {
		return stored
	}
	plain, err := crypto.Decrypt(stored)
	if err != nil {
		s.logf("settings: stored %s does not decrypt, treating it as empty: %v", key, err)
		return ""
	}
	return plain
}

func validValue(t config.Type, v string) bool {
	switch t {
	case config.TypeBool:
		return v == "true" || v == "false"
	case config.TypeInt:
		_, err := strconv.Atoi(v)
		return err == nil
	default:
		return true
	}
}
