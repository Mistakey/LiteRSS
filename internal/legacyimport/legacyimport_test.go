package legacyimport

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/freshrss/freshrsstest"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/syncer"
)

// legacySchema is the legacy MrRSS schema as legacy-final builds it:
// initSchema's tables, then the columns runMigrations adds, the AI profiles,
// the FreshRSS push queue and the settings table.
var legacySchema = []string{
	`CREATE TABLE IF NOT EXISTS feeds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		url TEXT,
		link TEXT DEFAULT '',
		description TEXT,
		category TEXT DEFAULT '',
		image_url TEXT DEFAULT '',
		last_updated DATETIME
	)`,
	`CREATE TABLE IF NOT EXISTS articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		feed_id INTEGER,
		title TEXT,
		url TEXT,
		image_url TEXT,
		audio_url TEXT DEFAULT '',
		video_url TEXT DEFAULT '',
		translated_title TEXT,
		published_at DATETIME,
		is_read BOOLEAN DEFAULT 0,
		is_favorite BOOLEAN DEFAULT 0,
		is_hidden BOOLEAN DEFAULT 0,
		is_read_later BOOLEAN DEFAULT 0,
		summary TEXT DEFAULT '',
		original_summary TEXT DEFAULT '',
		unique_id TEXT UNIQUE,
		FOREIGN KEY(feed_id) REFERENCES feeds(id)
	)`,
	`CREATE TABLE IF NOT EXISTS translation_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_text_hash TEXT NOT NULL,
		source_text TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		translated_text TEXT NOT NULL,
		provider TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(source_text_hash, target_lang, provider)
	)`,
	`CREATE TABLE IF NOT EXISTS article_contents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		article_id INTEGER NOT NULL UNIQUE,
		content TEXT NOT NULL,
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		full_content TEXT NOT NULL DEFAULT '',
		full_content_fetched_at DATETIME,
		FOREIGN KEY(article_id) REFERENCES articles(id) ON DELETE CASCADE
	)`,
	`ALTER TABLE articles ADD COLUMN content TEXT DEFAULT ''`,
	`ALTER TABLE feeds ADD COLUMN freshrss_stream_id TEXT DEFAULT ''`,
	`ALTER TABLE articles ADD COLUMN freshrss_item_id TEXT DEFAULT ''`,
	`ALTER TABLE articles ADD COLUMN author TEXT DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS ai_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		api_key TEXT DEFAULT '',
		endpoint TEXT NOT NULL,
		model TEXT NOT NULL,
		custom_headers TEXT DEFAULT '',
		is_default BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS freshrss_sync_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		article_id INTEGER NOT NULL,
		article_url TEXT NOT NULL,
		sync_action TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		synced_at INTEGER,
		sync_error TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT
	)`,
}

// legacyMachineID and legacyEncrypt are legacy-final's crypto.GetMachineID
// and crypto.Encrypt, kept verbatim but for the standard library's pbkdf2
// (the same function), so the fixture pins the format MrRSS wrote.
func legacyMachineID() string {
	hostname, _ := os.Hostname()
	machineUUID := ""
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if data, err := os.ReadFile(path); err == nil {
			machineUUID = string(data)
			break
		}
	}
	return fmt.Sprintf("%s-%s-%s-%s", hostname, runtime.GOOS, runtime.GOARCH, machineUUID)
}

func legacyEncrypt(t *testing.T, machineID, plaintext string) string {
	t.Helper()
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		t.Fatal(err)
	}
	key, err := pbkdf2.Key(sha256.New, machineID, salt, 600000, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	result := append(append(append([]byte{}, salt...), nonce...), ciphertext...)
	return "MrRSS-v1:" + base64.StdEncoding.EncodeToString(result)
}

func longID(id int64) string {
	return fmt.Sprintf("tag:google.com,2005:reader/item/%016x", id)
}

// Item ids of the fixture: fetch times in microseconds, recent enough to
// outlive the 90-day retention of a first cycle.
var (
	fetchBase = time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	idA       = fetchBase.UnixMicro()                  // English, translated, summarized, read
	idB       = fetchBase.Add(time.Minute).UnixMicro() // judged Chinese, same URL as C
	idC       = fetchBase.Add(2 * time.Minute).UnixMicro()
	idD       = fetchBase.Add(3 * time.Minute).UnixMicro() // queue: read then unread
	idE       = fetchBase.Add(4 * time.Minute).UnixMicro() // queue: star only
)

// fixture writes a legacy library at path: every kind of row the import
// keeps, converts or drops.
func fixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, s := range legacySchema {
		exec(s)
	}
	exec(`INSERT INTO feeds (id, title, url, freshrss_stream_id) VALUES (1, 'Go Blog', 'https://go.dev/blog/feed.atom', 'feed/7'),
	      (2, '少数派', 'https://sspai.com/feed', 'feed/8'), (3, 'local', 'https://local.example/feed', '')`)

	type art struct {
		id                                 int64
		feed                               int
		item, title, url, translated, summ string
		published                          any
		read                               bool
	}
	arts := []art{
		{1, 1, longID(idA), "Go 1.27 is released", "https://go.dev/blog/go1.27", "Go 1.27 发布", "Go 1.27 带来了新特性。",
			time.Date(2026, 9, 26, 21, 48, 3, 0, time.FixedZone("CST", 8*3600)), true},
		{2, 2, longID(idB), "少数派周报", "https://sspai.com/post/1", "少数派周报", "", "2026-09-25 10:00:00", true},
		{3, 1, longID(idC), "Weekly digest", "https://sspai.com/post/1", "", "", "not a date", false},
		{4, 1, longID(idD), "Queued twice", "https://go.dev/blog/queued", "", "", nil, true},
		{5, 1, longID(idE), "Starred only", "https://go.dev/blog/starred", "", "", "2001-01-01 00:00:00 +0000 UTC", true},
		{6, 1, "", "No item id", "https://go.dev/blog/lost", "没有 ID", "", nil, true},
		{7, 3, longID(idE + 1), "No stream", "https://local.example/1", "", "", nil, true},
	}
	for _, a := range arts {
		exec(`INSERT INTO articles (id, feed_id, title, url, image_url, translated_title, published_at, is_read, summary, freshrss_item_id, is_favorite, audio_url)
		      VALUES (?, ?, ?, ?, 'https://img.example/a.png', NULLIF(?, ''), ?, ?, ?, ?, 1, 'https://audio.example/a.mp3')`,
			a.id, a.feed, a.title, a.url, a.translated, a.published, a.read, a.summ, a.item)
	}
	exec(`INSERT INTO article_contents (article_id, content, full_content) VALUES
	      (1, '<p>Go body</p>', '<p>Go full text</p>'), (2, '<p>周报正文</p>', ''), (6, '<p>lost</p>', '')`)
	exec(`INSERT INTO translation_cache (source_text_hash, source_text, target_lang, translated_text, provider)
	      VALUES ('h', 'Weekly digest', 'zh', '每周摘要', 'baidu')`)

	exec(`INSERT INTO freshrss_sync_queue (article_id, article_url, sync_action, created_at, synced_at, retry_count) VALUES
	      (4, 'u', 'mark_read',   100, NULL, 0),
	      (1, 'u', 'mark_unread', 101, NULL, 0),
	      (4, 'u', 'mark_unread', 102, NULL, 0),
	      (5, 'u', 'star',        103, NULL, 0),
	      (2, 'u', 'mark_unread', 104, 105,  0),
	      (3, 'u', 'mark_read',   106, NULL, 5),
	      (6, 'u', 'mark_read',   107, NULL, 0)`)

	here := legacyMachineID()
	settingsRows := map[string]string{
		"freshrss_server_url":         "https://rss.example",
		"freshrss_username":           "reader",
		"freshrss_api_password":       legacyEncrypt(t, here, "fresh-secret"),
		"freshrss_auto_sync_interval": "15",
		"freshrss_last_sync_time":     "2026-09-26T12:00:00Z",
		"baidu_app_id":                "2026",
		"baidu_secret_key":            legacyEncrypt(t, "another-host-windows-amd64-", "baidu-secret"),
		"proxy_mode":                  "manual",
		"proxy_host":                  "10.0.0.2",
		"proxy_username":              "plain-user", // never read by MrRSS, so never encrypted
		"proxy_password":              "",
		"close_to_tray":               "false",
		"window_x":                    "-21333",
		"window_y":                    "-21333",
		"window_width":                "1400",
		"window_maximized":            "not-a-bool",
		"ai_endpoint":                 "https://api.openai.com/v1",
		"ai_model":                    "gpt-4o-mini",
		"ai_usage_tokens":             "1234",
		"ai_summary_prompt":           "Summarize",
		"ai_summary_profile_id":       "",
		"notion_token":                "secret",
	}
	for k, v := range settingsRows {
		exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, k, v)
	}
	exec(`INSERT INTO ai_profiles (id, name, api_key, endpoint, model, is_default) VALUES
	      (1, 'first', '', 'https://first.example/v1', 'first-model', 0),
	      (2, 'deepseek', ?, 'https://api.deepseek.com/v1', 'deepseek-flash', 1)`,
		legacyEncrypt(t, here, "sk-deepseek"))
}

type env struct {
	legacyPath string
	dataDir    string
	db         *database.DB
	store      *settings.Store
	im         *Importer
	logs       []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{legacyPath: filepath.Join(root, "MrRSS", "rss.db"), dataDir: filepath.Join(root, "LiteRSS")}
	for _, d := range []string{filepath.Dir(e.legacyPath), e.dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fixture(t, e.legacyPath)
	db, err := database.Open(context.Background(), filepath.Join(e.dataDir, "literss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e.db, e.store = db, settings.New(db.DB)
	e.im = &Importer{
		DB: db, Store: e.store, DataDir: e.dataDir,
		Candidates:    []string{filepath.Join(root, "missing.db"), e.legacyPath},
		LegacyRunning: func() bool { return false },
		now:           time.Now,
		logf:          func(f string, a ...any) { e.logs = append(e.logs, fmt.Sprintf(f, a...)) },
	}
	return e
}

func (e *env) record(t *testing.T) (Report, bool) {
	t.Helper()
	var raw string
	err := e.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, MetaLegacyImport).Scan(&raw)
	if err == sql.ErrNoRows {
		return Report{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatal(err)
	}
	return rep, true
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestImportCopiesTheLegacyLibrary(t *testing.T) {
	e := newEnv(t)
	before, err := os.ReadFile(e.legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	outcome, rep, err := e.im.Run(context.Background())
	if err != nil || outcome != Finished || rep.Status != StatusDone {
		t.Fatalf("Run = %v, %+v, %v", outcome, rep, err)
	}

	t.Run("legacy file is untouched", func(t *testing.T) {
		after, err := os.ReadFile(e.legacyPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Error("the legacy library changed")
		}
		leftovers, _ := filepath.Glob(filepath.Join(e.dataDir, "legacy-snapshot-*"))
		if len(leftovers) > 0 {
			t.Errorf("snapshot left behind: %v", leftovers)
		}
	})

	t.Run("articles", func(t *testing.T) {
		// Article 6 has no item id and article 7 no stream: both dropped.
		if rep.Articles != 5 || rep.DroppedArticles != 2 {
			t.Errorf("articles %d, dropped %d; want 5, 2", rep.Articles, rep.DroppedArticles)
		}
		if n := e.count(t, `SELECT COUNT(*) FROM articles WHERE title IN ('No item id', 'No stream')`); n != 0 {
			t.Errorf("%d unlocatable articles imported", n)
		}
		// B and C share a URL and stay two articles.
		if n := e.count(t, `SELECT COUNT(*) FROM articles WHERE url = 'https://sspai.com/post/1'`); n != 2 {
			t.Errorf("same-URL articles: %d, want 2", n)
		}
		var stream string
		var read int
		var published int64
		if err := e.db.QueryRow(`SELECT stream_id, server_read, published_at FROM articles WHERE item_id = ?`, idA).
			Scan(&stream, &read, &published); err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 9, 26, 13, 48, 3, 0, time.UTC).Unix()
		if stream != "feed/7" || read != 1 || published != want {
			t.Errorf("A = %s, read %d, published %d; want feed/7, 1, %d", stream, read, published, want)
		}
		// An unparsable date falls back to the fetch time; an old one stays.
		checkPublished := func(id, want int64) {
			t.Helper()
			var got int64
			if err := e.db.QueryRow(`SELECT published_at FROM articles WHERE item_id = ?`, id).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("published_at of %d = %d, want %d", id, got, want)
			}
		}
		checkPublished(idC, database.FetchedAt(idC).Unix())
		checkPublished(idD, database.FetchedAt(idD).Unix())
		checkPublished(idE, time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
		checkPublished(idB, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC).Unix())
	})

	t.Run("bodies, translations and summaries", func(t *testing.T) {
		if rep.Contents != 2 || rep.Translations != 2 || rep.Summaries != 1 {
			t.Errorf("contents %d, translations %d, summaries %d; want 2, 2, 1", rep.Contents, rep.Translations, rep.Summaries)
		}
		var title string
		if err := e.db.QueryRow(`SELECT translated_title FROM title_translations WHERE item_id = ?`, idB).Scan(&title); err != nil {
			t.Fatal(err)
		}
		if title != "少数派周报" {
			t.Errorf("judged-Chinese translation = %q, want the title itself", title)
		}
		if n := e.count(t, `SELECT COUNT(*) FROM fulltext_cache WHERE item_id = ? AND content = '<p>Go full text</p>'`, idA); n != 1 {
			t.Error("full text not imported apart from the RSS body")
		}
		if n := e.count(t, `SELECT COUNT(*) FROM article_contents WHERE item_id = ? AND content = '<p>Go body</p>'`, idA); n != 1 {
			t.Error("RSS body not imported")
		}
		if n := e.count(t, `SELECT COUNT(*) FROM title_translations WHERE translated_title = '每周摘要'`); n != 0 {
			t.Error("translation cache imported")
		}
	})

	t.Run("unsent queue becomes read intents", func(t *testing.T) {
		rows, err := e.db.Query(`SELECT item_id, value, seq FROM pending_read ORDER BY seq`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var id, value, seq int64
			if err := rows.Scan(&id, &value, &seq); err != nil {
				t.Fatal(err)
			}
			got = append(got, fmt.Sprintf("%d=%d@%d", id, value, seq))
		}
		// A unread (row 101), then D's last change, unread (row 102). The star,
		// the synced, the given-up and the id-less rows are gone.
		want := []string{fmt.Sprintf("%d=0@1", idA), fmt.Sprintf("%d=0@2", idD)}
		if !slices.Equal(got, want) {
			t.Errorf("intents = %v, want %v", got, want)
		}
		if rep.Intents != 2 {
			t.Errorf("report intents = %d", rep.Intents)
		}
		if n := e.count(t, `SELECT CAST(value AS INTEGER) FROM meta WHERE key = ?`, syncer.MetaIntentSeq); n != 2 {
			t.Errorf("intent counter = %d, want 2", n)
		}
	})

	t.Run("settings", func(t *testing.T) {
		values, err := e.store.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"freshrss_server_url":         "https://rss.example",
			"freshrss_username":           "reader",
			"freshrss_api_password":       "fresh-secret",
			"freshrss_auto_sync_interval": "15",
			"baidu_app_id":                "2026",
			"baidu_secret_key":            "", // did not decrypt: to refill
			"proxy_mode":                  "manual",
			"proxy_host":                  "10.0.0.2",
			"proxy_username":              "plain-user",
			"close_to_tray":               "false",
			"window_x":                    "0", // minimized placeholder dropped
			"window_y":                    "0",
			"window_width":                "1400",
			"window_maximized":            "false", // unparsable, left at default
			"llm_endpoint":                "https://api.deepseek.com/v1",
			"llm_model":                   "deepseek-flash",
			"llm_api_key":                 "sk-deepseek",
		}
		for k, v := range want {
			if values[k] != v {
				t.Errorf("%s = %q, want %q", k, values[k], v)
			}
		}
		if !slices.Equal(rep.Refill, []string{"baidu_secret_key"}) {
			t.Errorf("refill = %v", rep.Refill)
		}
		// Only schema keys are stored, credentials encrypted with the new scheme.
		rows, err := e.db.Query(`SELECT key, value FROM settings`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			if _, ok := want[k]; !ok {
				t.Errorf("stored key %q outside the import whitelist", k)
			}
			if strings.HasPrefix(v, "MrRSS-v1:") && runtime.GOOS == "windows" {
				t.Errorf("%s kept the legacy encryption", k)
			}
			if strings.Contains(v, "secret") || strings.Contains(v, "sk-") {
				t.Errorf("%s stored in plain text", k)
			}
		}
	})

	t.Run("record", func(t *testing.T) {
		got, ok := e.record(t)
		if !ok || got.Status != StatusDone || got.Source != e.legacyPath || got.Articles != 5 || got.At == 0 {
			t.Errorf("record = %+v", got)
		}
		// Once recorded, the legacy library is not looked at again.
		e.im.Candidates = nil
		if outcome, _, err := e.im.Run(context.Background()); err != nil || outcome != Recorded {
			t.Errorf("second Run = %v, %v", outcome, err)
		}
	})
}

func TestImportWaitsWhileLegacyRuns(t *testing.T) {
	e := newEnv(t)
	running := true
	e.im.LegacyRunning = func() bool { return running }

	outcome, _, err := e.im.Run(context.Background())
	if err != nil || outcome != Deferred {
		t.Fatalf("Run = %v, %v; want Deferred", outcome, err)
	}
	if _, ok := e.record(t); ok {
		t.Error("a deferred import wrote legacy_import")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM articles`); n != 0 {
		t.Errorf("a deferred import wrote %d articles", n)
	}

	running = false
	if outcome, rep, err := e.im.Run(context.Background()); err != nil || outcome != Finished || rep.Status != StatusDone {
		t.Fatalf("Run after quitting = %v, %+v, %v", outcome, rep, err)
	}
}

func TestImportRunsBeforeTheFirstCycle(t *testing.T) {
	e := newEnv(t)
	running := true
	e.im.LegacyRunning = func() bool { return running }

	fake := freshrsstest.New("u", "p")
	fake.AddFeeds(freshrsstest.Feed{ID: 7, Title: "Go Blog"}, freshrsstest.Feed{ID: 8, Title: "少数派"})
	fake.AddItems(freshrsstest.Item{ID: idA, FeedID: 7, Title: "Go 1.27 is released", Read: true},
		freshrsstest.Item{ID: idD, FeedID: 7, Title: "Queued twice", Read: true})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	client := freshrss.NewClient(srv.URL, "u", "p")
	svc := syncer.New(e.db, func() (syncer.Remote, error) { return client, nil })
	defer svc.Close()

	cycles, deferrals := 0, 0
	run := e.im.Gate(func(ctx context.Context) {
		cycles++
		if _, ok := e.record(t); !ok {
			t.Error("a cycle ran before the import was recorded")
		}
		if _, err := svc.RunCycle(ctx); err != nil {
			t.Errorf("cycle: %v", err)
		}
	}, func() { deferrals++ })

	ctx := context.Background()
	run(ctx)
	if cycles != 0 || deferrals != 1 {
		t.Fatalf("while MrRSS runs: %d cycles, %d deferrals; want 0, 1", cycles, deferrals)
	}
	if len(fake.EditTags()) != 0 {
		t.Error("synced while the import waited")
	}

	running = false
	run(ctx)
	if cycles != 1 {
		t.Fatalf("cycles = %d, want 1", cycles)
	}
	// The first cycle pushed the imported unread intents before pulling, so
	// the server now has them unread and the pull did not undo them.
	for _, id := range []int64{idA, idD} {
		if it, _ := fake.Item(id); it.Read {
			t.Errorf("imported unread intent for %d not pushed", id)
		}
	}
	var read int
	if err := e.db.QueryRow(`SELECT server_read FROM articles WHERE item_id = ?`, idA).Scan(&read); err != nil || read != 0 {
		t.Errorf("A after the first cycle: server_read %d, %v", read, err)
	}

	run(ctx)
	if cycles != 2 {
		t.Errorf("later cycles are not gated: %d", cycles)
	}
}

func TestImportRecordsSkipsAndFailures(t *testing.T) {
	t.Run("no legacy library", func(t *testing.T) {
		e := newEnv(t)
		e.im.Candidates = []string{filepath.Join(t.TempDir(), "rss.db")}
		e.im.LegacyRunning = func() bool { t.Error("probed without a legacy library"); return false }
		if outcome, rep, err := e.im.Run(context.Background()); err != nil || outcome != Finished || rep.Status != StatusSkipped {
			t.Fatalf("Run = %v, %+v, %v", outcome, rep, err)
		}
		if rep, _ := e.record(t); rep.Status != StatusSkipped {
			t.Errorf("record = %+v", rep)
		}
	})

	t.Run("library already synced", func(t *testing.T) {
		e := newEnv(t)
		if _, err := e.db.Exec(`INSERT INTO meta (key, value) VALUES ('last_sync_at', '1')`); err != nil {
			t.Fatal(err)
		}
		if _, rep, err := e.im.Run(context.Background()); err != nil || rep.Status != StatusSkipped {
			t.Fatalf("Run = %+v, %v", rep, err)
		}
		if n := e.count(t, `SELECT COUNT(*) FROM articles`); n != 0 {
			t.Errorf("imported %d articles into a synced library", n)
		}
	})

	t.Run("not a MrRSS library", func(t *testing.T) {
		e := newEnv(t)
		other := filepath.Join(t.TempDir(), "rss.db")
		db, err := sql.Open("sqlite", other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT)`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		e.im.Candidates = []string{other}
		outcome, rep, err := e.im.Run(context.Background())
		if err != nil || outcome != Finished || rep.Status != StatusFailed || rep.Reason == "" {
			t.Fatalf("Run = %v, %+v, %v", outcome, rep, err)
		}
		// A failed import does not hold syncing back.
		cycles := 0
		e.im.Gate(func(context.Context) { cycles++ }, nil)(context.Background())
		if cycles != 1 {
			t.Error("the cycle did not run after a failed import")
		}
	})
}

func TestSummaryProfileFollowsTheLegacyChain(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		defaultID  int
		want       string
	}{
		{"configured profile", "3", 2, "third"},
		{"configured id missing, default", "9", 2, "second"},
		{"no default, smallest id", "", 0, "first"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "p.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, s := range legacySchema {
				if _, err := db.Exec(s); err != nil {
					t.Fatal(err)
				}
			}
			for i, name := range []string{"first", "second", "third"} {
				if _, err := db.Exec(`INSERT INTO ai_profiles (id, name, endpoint, model, is_default) VALUES (?, ?, 'e', ?, ?)`,
					i+1, name, name, i+1 == tt.defaultID); err != nil {
					t.Fatal(err)
				}
			}
			p, err := summaryProfile(context.Background(), db, tt.configured)
			if err != nil || p == nil || p.model != tt.want {
				t.Errorf("summaryProfile = %+v, %v; want %s", p, err, tt.want)
			}
		})
	}
}

func TestPublishedAt(t *testing.T) {
	id := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMicro()
	fetched := database.FetchedAt(id).Unix()
	tests := []struct {
		raw  string
		want int64
	}{
		{"2026-09-25 21:48:03 +0800 CST", time.Date(2026, 9, 25, 13, 48, 3, 0, time.UTC).Unix()},
		{"2026-09-25 21:48:03.123456 +0800 CST m=+0.012", time.Date(2026, 9, 25, 13, 48, 3, 0, time.UTC).Unix()},
		{"2026-09-25T10:00:00Z", time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC).Unix()},
		{"2026-09-25 10:00:00", time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC).Unix()},
		{"1758794400", 1758794400},
		{"", fetched},
		{"garbage", fetched},
		{"0001-01-01 00:00:00 +0000 UTC", fetched},
		{"2026-12-01 00:00:00 +0000 UTC", fetched}, // more than a day after the fetch
	}
	for _, tt := range tests {
		if got := publishedAt(tt.raw, id); got != tt.want {
			t.Errorf("publishedAt(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestCandidates(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == PathEnv {
				return v
			}
			return ""
		}
	}
	if got := Candidates(env("D:/old/rss.db"), false, "C:/cfg", "C:/app", true); !slices.Equal(got, []string{"D:/old/rss.db"}) {
		t.Errorf("development candidates = %v, want only the environment's", got)
	}
	if got := Candidates(env(""), false, "C:/cfg", "C:/app", true); len(got) != 0 {
		t.Errorf("development candidates without the variable = %v", got)
	}
	got := Candidates(env(""), true, "C:/cfg", "C:/app", true)
	want := []string{filepath.Join("C:/app", "data", "rss.db")}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		want = append([]string{filepath.Join("C:/cfg", "MrRSS", "rss.db")}, want...)
	}
	if !slices.Equal(got, want) {
		t.Errorf("production candidates = %v, want %v", got, want)
	}
	if got := Candidates(env(""), true, "C:/cfg", "C:/app", false); slices.Contains(got, filepath.Join("C:/app", "data", "rss.db")) {
		t.Error("an installed build looks beside its executable")
	}
}
