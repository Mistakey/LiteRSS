package legacyimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"LiteRSS/internal/crypto"
	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/syncer"
)

// maxPushRetries is MrRSS's MaxSyncPushRetries: a queued change refused this
// often was given up by MrRSS and is not revived.
const maxPushRetries = 5

// legacyMarker prefixes a credential MrRSS encrypted. MrRSS encrypted a
// plain value only when it first read it, so a value without it is plain.
const legacyMarker = "MrRSS-v1:"

// plainSettings are the legacy settings kept as they are (spec D10, D12).
var plainSettings = []string{
	"freshrss_server_url", "freshrss_username", "freshrss_auto_sync_interval",
	"baidu_app_id",
	"proxy_mode", "proxy_type", "proxy_host", "proxy_port",
	"update_check_enabled", "close_to_tray", "startup_on_boot",
	"window_width", "window_height", "window_maximized",
}

// credentialSettings are the legacy settings MrRSS encrypted; they are
// decrypted here and encrypted again by the settings store.
var credentialSettings = []string{
	"freshrss_api_password", "baidu_secret_key", "proxy_username", "proxy_password",
}

// importFrom snapshots the legacy library at source and imports the snapshot
// in one transaction, which also writes the done record.
func (im *Importer) importFrom(ctx context.Context, source string, now func() time.Time, logf func(string, ...any)) (Report, error) {
	legacy, err := openReadOnly(source)
	if err != nil {
		return Report{}, err
	}
	defer legacy.Close()
	if err := recognize(ctx, legacy); err != nil {
		return Report{}, err
	}

	snapPath := filepath.Join(im.DataDir, fmt.Sprintf("legacy-snapshot-%d.db", now().UnixNano()))
	defer removeSnapshot(snapPath)
	if _, err := legacy.ExecContext(ctx, `VACUUM INTO ?`, snapPath); err != nil {
		return Report{}, fmt.Errorf("snapshot the legacy library: %w", err)
	}
	legacy.Close()

	snap, err := openReadOnly(snapPath)
	if err != nil {
		return Report{}, err
	}
	defer snap.Close()

	tx, err := im.DB.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, fmt.Errorf("begin import: %w", err)
	}
	defer tx.Rollback()

	var rep Report
	itemIDs, err := copyArticles(ctx, snap, tx, now().Unix(), &rep)
	if err != nil {
		return Report{}, err
	}
	if rep.Intents, err = copyIntents(ctx, snap, tx, itemIDs, logf); err != nil {
		return Report{}, err
	}
	values, refill, err := legacySettings(ctx, snap, logf)
	if err != nil {
		return Report{}, err
	}
	if err := im.Store.UpdateTx(ctx, tx, values); err != nil {
		return Report{}, fmt.Errorf("import settings: %w", err)
	}
	rep.Settings, rep.Refill = len(values), refill

	rep.Status, rep.Source, rep.At = StatusDone, source, now().Unix()
	if err := writeRecord(ctx, tx, rep); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, fmt.Errorf("commit import: %w", err)
	}
	return rep, nil
}

// openReadOnly opens a SQLite file through a read-only URI, so nothing this
// package does can write it.
func openReadOnly(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}
	p := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func removeSnapshot(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		_ = os.Remove(p)
	}
}

// recognize checks the tables and columns the import reads. A legacy library
// has user_version 0, so its shape is the only thing to go by.
func recognize(ctx context.Context, db *sql.DB) error {
	for _, table := range []string{"articles", "settings", "article_contents", "feeds"} {
		ok, err := hasTable(ctx, db, table)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("not a MrRSS library: no %s table", table)
		}
	}
	for _, c := range [][2]string{{"articles", "freshrss_item_id"}, {"feeds", "freshrss_stream_id"}} {
		ok, err := hasColumn(ctx, db, c[0], c[1])
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("not a MrRSS library with FreshRSS sync: no %s.%s column", c[0], c[1])
		}
	}
	return nil
}

func hasTable(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
		return false, fmt.Errorf("read the legacy library: %w", err)
	}
	return n > 0, nil
}

func hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("read the legacy library: %w", err)
	}
	return n > 0, nil
}

// copyArticles copies every article with a FreshRSS item id and a
// subscription stream, with its RSS body, full text, title translation and
// summary. It returns the item id of each copied legacy article id.
func copyArticles(ctx context.Context, snap *sql.DB, tx *sql.Tx, importedAt int64, rep *Report) (map[int64]int64, error) {
	rows, err := snap.QueryContext(ctx, `
		SELECT a.id, COALESCE(a.freshrss_item_id, ''), COALESCE(f.freshrss_stream_id, ''),
		       COALESCE(a.title, ''), COALESCE(a.url, ''), COALESCE(a.image_url, ''),
		       CAST(a.published_at AS TEXT), CASE WHEN a.is_read THEN 1 ELSE 0 END,
		       COALESCE(a.translated_title, ''), COALESCE(a.summary, ''),
		       COALESCE(c.content, ''), COALESCE(c.full_content, '')
		FROM articles a
		LEFT JOIN feeds f ON f.id = a.feed_id
		LEFT JOIN article_contents c ON c.article_id = a.id
		ORDER BY a.id`)
	if err != nil {
		return nil, fmt.Errorf("read legacy articles: %w", err)
	}
	defer rows.Close()

	stmts, err := prepareAll(ctx, tx, []string{
		`INSERT INTO articles (item_id, stream_id, url, title, image_url, published_at, server_read)
		 VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(item_id) DO NOTHING`,
		`INSERT INTO article_contents (item_id, content) VALUES (?, ?)`,
		`INSERT INTO fulltext_cache (item_id, content, cached_at) VALUES (?, ?, ?)`,
		`INSERT INTO title_translations (item_id, translated_title) VALUES (?, ?)`,
		`INSERT INTO summaries (item_id, summary, created_at) VALUES (?, ?, ?)`,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, s := range stmts {
			s.Close()
		}
	}()
	insArticle, insContent, insFull, insTitle, insSummary := stmts[0], stmts[1], stmts[2], stmts[3], stmts[4]

	itemIDs := map[int64]int64{}
	for rows.Next() {
		var (
			legacyID                                  int64
			rawItemID, stream, title, link, image     string
			published                                 sql.NullString
			read                                      int
			translated, summary, content, fullContent string
		)
		if err := rows.Scan(&legacyID, &rawItemID, &stream, &title, &link, &image,
			&published, &read, &translated, &summary, &content, &fullContent); err != nil {
			return nil, fmt.Errorf("read legacy article: %w", err)
		}
		itemID, err := freshrss.ParseItemID(rawItemID)
		if err != nil || stream == "" {
			rep.DroppedArticles++
			continue
		}
		res, err := insArticle.ExecContext(ctx, itemID, stream, link, title, image,
			publishedAt(published.String, itemID), read)
		if err != nil {
			return nil, fmt.Errorf("import article %d: %w", legacyID, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Another legacy row already took this item id.
			rep.DroppedArticles++
			continue
		}
		itemIDs[legacyID] = itemID
		rep.Articles++

		if content != "" {
			if _, err := insContent.ExecContext(ctx, itemID, content); err != nil {
				return nil, fmt.Errorf("import body of %d: %w", legacyID, err)
			}
			rep.Contents++
		}
		if fullContent != "" {
			if _, err := insFull.ExecContext(ctx, itemID, fullContent, importedAt); err != nil {
				return nil, fmt.Errorf("import full text of %d: %w", legacyID, err)
			}
		}
		// Carried as is, a value equal to the title included: it records that
		// the title was judged Chinese already (pitfall 12).
		if translated != "" {
			if _, err := insTitle.ExecContext(ctx, itemID, translated); err != nil {
				return nil, fmt.Errorf("import title translation of %d: %w", legacyID, err)
			}
			rep.Translations++
		}
		if summary != "" {
			if _, err := insSummary.ExecContext(ctx, itemID, summary, importedAt); err != nil {
				return nil, fmt.Errorf("import summary of %d: %w", legacyID, err)
			}
			rep.Summaries++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy articles: %w", err)
	}
	return itemIDs, nil
}

func prepareAll(ctx context.Context, tx *sql.Tx, queries []string) ([]*sql.Stmt, error) {
	stmts := make([]*sql.Stmt, 0, len(queries))
	for _, q := range queries {
		s, err := tx.PrepareContext(ctx, q)
		if err != nil {
			for _, p := range stmts {
				p.Close()
			}
			return nil, fmt.Errorf("prepare import: %w", err)
		}
		stmts = append(stmts, s)
	}
	return stmts, nil
}

// publishedLayouts are the forms a legacy published_at takes: the driver
// wrote Go's time.Time String form, a backfill wrote SQLite's datetime().
var publishedLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700",
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

// publishedAt turns a legacy published_at into the new column; what does not
// parse becomes the fetch time (spec D9).
func publishedAt(raw string, itemID int64) int64 {
	raw = strings.TrimSpace(raw)
	// time.Now's String carries a monotonic clock reading: " m=+0.001".
	if i := strings.Index(raw, " m="); i >= 0 {
		raw = raw[:i]
	}
	unix := ""
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		unix = raw
	} else {
		for _, layout := range publishedLayouts {
			if t, err := time.Parse(layout, raw); err == nil {
				unix = strconv.FormatInt(t.Unix(), 10)
				break
			}
		}
	}
	return database.NormalizePublishedAt(unix, itemID)
}

// copyIntents turns the legacy push queue into read intents: of the changes
// MrRSS had not sent and not given up, the latest per article; star changes
// are dropped (spec D12). seq follows the order of those latest changes.
func copyIntents(ctx context.Context, snap *sql.DB, tx *sql.Tx, itemIDs map[int64]int64, logf func(string, ...any)) (int, error) {
	ok, err := hasTable(ctx, snap, "freshrss_sync_queue")
	if err != nil || !ok {
		return 0, err
	}
	cond := "synced_at IS NULL"
	if ok, err := hasColumn(ctx, snap, "freshrss_sync_queue", "retry_count"); err != nil {
		return 0, err
	} else if ok {
		cond += " AND retry_count < " + strconv.Itoa(maxPushRetries)
	}
	rows, err := snap.QueryContext(ctx,
		`SELECT article_id, sync_action FROM freshrss_sync_queue WHERE `+cond+` ORDER BY created_at, id`)
	if err != nil {
		return 0, fmt.Errorf("read the legacy push queue: %w", err)
	}
	defer rows.Close()

	type latest struct {
		value int
		order int
	}
	last := map[int64]latest{}
	unknown := 0
	for i := 0; rows.Next(); i++ {
		var articleID int64
		var action string
		if err := rows.Scan(&articleID, &action); err != nil {
			return 0, fmt.Errorf("read the legacy push queue: %w", err)
		}
		var value int
		switch action {
		case "mark_read":
			value = 1
		case "mark_unread":
			value = 0
		default:
			continue
		}
		itemID, found := itemIDs[articleID]
		if !found {
			unknown++
			continue
		}
		last[itemID] = latest{value: value, order: i}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read the legacy push queue: %w", err)
	}
	if unknown > 0 {
		logf("legacy import: dropped %d unsent read changes of articles without a FreshRSS item id", unknown)
	}
	if len(last) == 0 {
		return 0, nil
	}

	ids := make([]int64, 0, len(last))
	for id := range last {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return last[ids[i]].order < last[ids[j]].order })

	// One intent per write, in order: each draws the next seq.
	for _, id := range ids {
		if err := syncer.PutReadTx(ctx, tx, []int64{id}, last[id].value); err != nil {
			return 0, fmt.Errorf("import intent: %w", err)
		}
	}
	return len(ids), nil
}

// legacySettings returns the settings to import, plain, and the credential
// keys that did not decrypt. Values the new schema would refuse are left out.
func legacySettings(ctx context.Context, snap *sql.DB, logf func(string, ...any)) (map[string]string, []string, error) {
	old := map[string]string{}
	rows, err := snap.QueryContext(ctx, `SELECT key, value FROM settings WHERE value IS NOT NULL`)
	if err != nil {
		return nil, nil, fmt.Errorf("read legacy settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, nil, fmt.Errorf("read legacy settings: %w", err)
		}
		old[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read legacy settings: %w", err)
	}

	values := map[string]string{}
	var refill []string
	for _, k := range plainSettings {
		if v := old[k]; v != "" {
			values[k] = v
		}
	}
	for _, k := range credentialSettings {
		if v, ok := openCredential(old[k]); !ok {
			refill = append(refill, k)
		} else if v != "" {
			values[k] = v
		}
	}

	// A minimized window was saved at a placeholder position; leaving both
	// out lets the shell center the window.
	x, errX := strconv.Atoi(old["window_x"])
	y, errY := strconv.Atoi(old["window_y"])
	if errX == nil && errY == nil && !settings.MinimizedWindowPos(x, y) {
		values["window_x"], values["window_y"] = old["window_x"], old["window_y"]
	}

	profile, err := summaryProfile(ctx, snap, old["ai_summary_profile_id"])
	if err != nil {
		return nil, nil, err
	}
	if profile != nil {
		if profile.endpoint != "" {
			values["llm_endpoint"] = profile.endpoint
		}
		if profile.model != "" {
			values["llm_model"] = profile.model
		}
		if v, ok := openCredential(profile.apiKey); !ok {
			refill = append(refill, "llm_api_key")
		} else if v != "" {
			values["llm_api_key"] = v
		}
	}

	for k, v := range values {
		if err := settings.Check(k, v); err != nil {
			// The error names the key and type, never the value.
			logf("legacy import: dropped setting: %v", err)
			delete(values, k)
		}
	}
	return values, refill, nil
}

// openCredential returns a legacy credential in plain text; ok is false when
// it is encrypted and does not decrypt on this machine.
func openCredential(v string) (string, bool) {
	if !strings.HasPrefix(v, legacyMarker) {
		return v, true
	}
	plain, err := crypto.DecryptLegacy(v)
	if err != nil {
		return "", false
	}
	return plain, true
}

type aiProfile struct {
	endpoint, model, apiKey string
}

// summaryProfile picks the one AI profile MrRSS used for summaries: the
// profile ai_summary_profile_id names, else the default one, else the one
// with the smallest id (MrRSS's ProfileProvider.GetProfileForFeature).
func summaryProfile(ctx context.Context, snap *sql.DB, configuredID string) (*aiProfile, error) {
	ok, err := hasTable(ctx, snap, "ai_profiles")
	if err != nil || !ok {
		return nil, err
	}
	const cols = `SELECT COALESCE(endpoint, ''), COALESCE(model, ''), COALESCE(api_key, '') FROM ai_profiles `
	var queries []string
	var args [][]any
	if id, err := strconv.ParseInt(configuredID, 10, 64); err == nil && id > 0 {
		queries, args = append(queries, cols+`WHERE id = ?`), append(args, []any{id})
	}
	queries, args = append(queries, cols+`WHERE is_default = 1 LIMIT 1`), append(args, nil)
	queries, args = append(queries, cols+`ORDER BY id LIMIT 1`), append(args, nil)
	for i, q := range queries {
		var p aiProfile
		err := snap.QueryRowContext(ctx, q, args[i]...).Scan(&p.endpoint, &p.model, &p.apiKey)
		if err == nil {
			return &p, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("read legacy AI profiles: %w", err)
		}
	}
	return nil, nil
}
