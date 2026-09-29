package syncer

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/freshrss/freshrsstest"
)

const day = int64(24 * time.Hour / time.Microsecond)

type env struct {
	fake *freshrsstest.Server
	db   *database.DB
	svc  *Service
	now  int64 // microseconds, i.e. the id an item crawled right now gets
}

// newEnv wires a Service to a fake FreshRSS through handler, which may wrap
// the fake to act between requests.
func newEnv(t *testing.T, wrap func(http.Handler) http.Handler) *env {
	t.Helper()
	fake := freshrsstest.New("u", "p")
	var h http.Handler = fake
	if wrap != nil {
		h = wrap(fake)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	client := freshrss.NewClient(srv.URL, "u", "p")
	svc := New(db, func() (Remote, error) { return client, nil })
	t.Cleanup(svc.Close)
	return &env{fake: fake, db: db, svc: svc, now: time.Now().UnixMicro()}
}

func (e *env) cycle(t *testing.T) Result {
	t.Helper()
	res, err := e.svc.RunCycle(context.Background())
	if err != nil {
		t.Fatalf("RunCycle: %v", err)
	}
	return res
}

// pullOnly runs a cycle's pull and cleanup without its push, for tests whose
// hand-written intents must stay unsent.
func (e *env) pullOnly(t *testing.T) Result {
	t.Helper()
	remote, err := e.svc.remote()
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.pull(context.Background(), remote)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if res.Cleaned, err = e.svc.cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	return res
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// serverRead returns the mirror, or -1 when the article is not in the library.
func (e *env) serverRead(t *testing.T, id int64) int {
	t.Helper()
	var v int
	err := e.db.QueryRow(`SELECT server_read FROM articles WHERE item_id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestReadElsewhereIsMirroredAndNeverPushedBack(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One", Labels: []string{"Tech"}})
	a, b := e.now-2*day, e.now-day
	e.fake.AddItems(
		freshrsstest.Item{ID: a, FeedID: 1, Title: "a", URL: "https://x/a", Content: `<p>a</p><img src="https://img/a.png">`},
		freshrsstest.Item{ID: b, FeedID: 1, Title: "b", URL: "https://x/b"},
	)

	if res := e.cycle(t); res.Added != 2 {
		t.Fatalf("first cycle added %d, want 2", res.Added)
	}
	if e.serverRead(t, a) != 0 || e.serverRead(t, b) != 0 {
		t.Fatal("unread items should mirror as unread")
	}
	var img, content string
	if err := e.db.QueryRow(`SELECT a.image_url, c.content FROM articles a JOIN article_contents c USING (item_id) WHERE item_id = ?`, a).Scan(&img, &content); err != nil {
		t.Fatal(err)
	}
	if img != "https://img/a.png" || !strings.Contains(content, "<p>a</p>") {
		t.Fatalf("image %q content %q", img, content)
	}

	// The user marked b unread locally and it is still waiting to be pushed;
	// the pull must neither drop that intent nor act on it.
	e.exec(t, `INSERT INTO pending_read (item_id, value, seq) VALUES (?, 0, 1)`, b)
	e.fake.SetRead(a, true)
	e.fake.SetRead(b, true)

	e.pullOnly(t)
	if e.serverRead(t, a) != 1 || e.serverRead(t, b) != 1 {
		t.Fatal("items read elsewhere should mirror as read")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_read WHERE item_id = ?`, b); n != 1 {
		t.Fatal("pull dropped a pending intent")
	}
	if calls := e.fake.EditTags(); len(calls) != 0 {
		t.Fatalf("pull wrote to the server: %+v", calls)
	}
	if it, _ := e.fake.Item(a); !it.Read {
		t.Fatal("server state changed")
	}
}

func TestHighWaterScanFetchesNewItemsReadElsewhere(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	first := e.now - 3*day
	oldRead := e.now - 120*day
	oldUnread := e.now - 100*day
	e.fake.AddItems(
		freshrsstest.Item{ID: first, FeedID: 1, Title: "first"},
		freshrsstest.Item{ID: oldRead, FeedID: 1, Title: "old read", Read: true},
		freshrsstest.Item{ID: oldUnread, FeedID: 1, Title: "old unread"},
	)
	e.cycle(t)
	if e.serverRead(t, oldRead) != -1 {
		t.Fatal("a read item older than retention should not be fetched")
	}
	if e.serverRead(t, oldUnread) != 0 {
		t.Fatal("an unread item is fetched whatever its age")
	}

	// Crawled and read on another device while this instance was off.
	later := e.now - day
	e.fake.AddItems(freshrsstest.Item{ID: later, FeedID: 1, Title: "later", Read: true, Content: "<p>later</p>"})
	// Committed late by a concurrent refresh: smaller id, inside the overlap.
	late := first - int64(5*time.Minute/time.Microsecond)
	e.fake.AddItems(freshrsstest.Item{ID: late, FeedID: 1, Title: "late", Read: true})

	if res := e.cycle(t); res.Added != 2 {
		t.Fatalf("added %d, want 2", res.Added)
	}
	if e.serverRead(t, later) != 1 || e.serverRead(t, late) != 1 {
		t.Fatal("new items read elsewhere should be in the library as read")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM article_contents WHERE item_id = ?`, later); n != 1 {
		t.Fatal("content missing for the scanned item")
	}
}

// With paging forced, an unread boundary entry read elsewhere between two
// pages makes the server drop the next real result (pitfall 28): that entry
// mirrors as read for one cycle and is corrected by the next.
func TestContinuationBoundaryReadElsewhereCostsOneEntryForOneCycle(t *testing.T) {
	var (
		mu     sync.Mutex
		armed  bool
		onPage func()
	)
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			q := r.URL.Query()
			if strings.HasSuffix(r.URL.Path, "/stream/items/ids") && q.Get("xt") != "" && q.Get("c") == "" {
				mu.Lock()
				if armed {
					armed = false
					onPage()
				}
				mu.Unlock()
			}
		})
	})
	e.svc.idPageSize = 2
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	ids := []int64{e.now - 5*day, e.now - 4*day, e.now - 3*day, e.now - 2*day, e.now - day}
	for _, id := range ids {
		e.fake.AddItems(freshrsstest.Item{ID: id, FeedID: 1, Title: "t"})
	}
	e.cycle(t)
	for _, id := range ids {
		if e.serverRead(t, id) != 0 {
			t.Fatalf("item %d should be unread after an undisturbed cycle", id)
		}
	}

	boundary, dropped := ids[3], ids[2] // page 1 is ids[4], ids[3]
	mu.Lock()
	armed = true
	onPage = func() { e.fake.SetRead(boundary, true) }
	mu.Unlock()
	e.cycle(t)
	if e.serverRead(t, boundary) != 0 {
		t.Fatal("the boundary was unread when its page was fetched")
	}
	if e.serverRead(t, dropped) != 1 {
		t.Fatal("the entry after the boundary is dropped by the server and mirrors as read for this cycle")
	}
	for _, id := range []int64{ids[0], ids[1], ids[4]} {
		if e.serverRead(t, id) != 0 {
			t.Fatalf("item %d should stay unread", id)
		}
	}

	e.cycle(t)
	if e.serverRead(t, dropped) != 0 || e.serverRead(t, boundary) != 1 {
		t.Fatal("the next cycle should correct both entries")
	}
}

func TestUnsubscribedFeedLosesArticlesAndLocalData(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(
		freshrsstest.Feed{ID: 1, Title: "Keep", Labels: []string{"Tech"}},
		freshrsstest.Feed{ID: 2, Title: "Gone", Labels: []string{"News"}},
	)
	keep, gone := e.now-2*day, e.now-day
	e.fake.AddItems(
		freshrsstest.Item{ID: keep, FeedID: 1, Title: "keep"},
		freshrsstest.Item{ID: gone, FeedID: 2, Title: "gone"},
	)
	e.cycle(t)
	for _, id := range []int64{keep, gone} {
		e.exec(t, `INSERT INTO fulltext_cache (item_id, content, cached_at) VALUES (?, 'f', 1)`, id)
		e.exec(t, `INSERT INTO title_translations (item_id, translated_title) VALUES (?, '译')`, id)
		e.exec(t, `INSERT INTO summaries (item_id, summary, created_at) VALUES (?, 's', 1)`, id)
		e.exec(t, `INSERT INTO article_translations (item_id, source_hash, blocks, created_at) VALUES (?, 'h', '["译"]', 1)`, id)
		e.exec(t, `INSERT INTO pending_read (item_id, value, seq) VALUES (?, 1, 1)`, id)
	}
	e.exec(t, `INSERT INTO pending_mark_all (stream_id, ts, seq) VALUES ('feed/2', 1, 1), ('user/-/label/News', 1, 2), ('feed/1', 1, 3)`)

	e.fake.RemoveFeed(2)
	if res := e.pullOnly(t); res.Removed != 1 {
		t.Fatalf("removed %d, want 1", res.Removed)
	}
	if e.serverRead(t, gone) != -1 {
		t.Fatal("article of the unsubscribed feed should be deleted")
	}
	for _, table := range []string{"article_contents", "fulltext_cache", "title_translations", "summaries", "article_translations", "pending_read"} {
		if n := e.count(t, `SELECT COUNT(*) FROM `+table+` WHERE item_id = ?`, gone); n != 0 {
			t.Fatalf("%s row survived the unsubscribe", table)
		}
		if n := e.count(t, `SELECT COUNT(*) FROM `+table+` WHERE item_id = ?`, keep); n != 1 {
			t.Fatalf("%s row of the kept feed was deleted", table)
		}
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all`); n != 1 {
		t.Fatalf("pending_mark_all has %d rows, want only feed/1", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM feeds WHERE stream_id = 'feed/2'`); n != 0 {
		t.Fatal("unsubscribed feed kept")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM tags WHERE tag_id = 'user/-/label/News'`); n != 0 {
		t.Fatal("tag no longer on the server kept")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM feed_tags WHERE stream_id = 'feed/1' AND tag_id = 'user/-/label/Tech'`); n != 1 {
		t.Fatal("feed_tags of the kept feed missing")
	}
}

func TestEmptySubscriptionListKeepsLibrary(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	id := e.now - day
	e.fake.AddItems(freshrsstest.Item{ID: id, FeedID: 1, Title: "a"})
	e.cycle(t)

	e.fake.RemoveFeed(1)
	if res := e.cycle(t); res.Removed != 0 {
		t.Fatalf("removed %d with an empty subscription list", res.Removed)
	}
	if e.serverRead(t, id) == -1 {
		t.Fatal("article deleted although the server answered no subscriptions")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM feeds`); n != 1 {
		t.Fatal("feeds cleared although the server answered no subscriptions")
	}
}

// An import brings articles but no feeds, so right after it the library has
// no feeds of its own; an empty answer then must not wipe the import.
func TestEmptySubscriptionListKeepsImportedArticles(t *testing.T) {
	e := newEnv(t, nil)
	id := e.now - day
	e.exec(t, `INSERT INTO articles (item_id, stream_id, published_at) VALUES (?, 'feed/1', 1)`, id)
	e.exec(t, `INSERT INTO summaries (item_id, summary, created_at) VALUES (?, 's', 1)`, id)

	if res := e.cycle(t); res.Removed != 0 {
		t.Fatalf("removed %d imported articles on an empty subscription list", res.Removed)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM summaries WHERE item_id = ?`, id); n != 1 {
		t.Fatal("imported article and its summary should be kept")
	}
}

// A cycle cut short while filling in entries must not raise the high-water
// mark past entries it never stored: the next scan would stop above them and
// an entry read elsewhere would never reach the library.
func TestInterruptedFetchIsCompletedByNextCycle(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
		fail  = true
	)
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/stream/items/contents") {
				mu.Lock()
				calls++
				broken := fail && calls == 2
				mu.Unlock()
				if broken {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	e.svc.contentsBatch = 1
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	older, newer := e.now-3*day, e.now-day
	e.fake.AddItems(
		freshrsstest.Item{ID: older, FeedID: 1, Title: "older", Read: true},
		freshrsstest.Item{ID: newer, FeedID: 1, Title: "newer", Read: true},
	)
	if _, err := e.svc.RunCycle(context.Background()); err == nil {
		t.Fatal("cycle should fail when a contents batch fails")
	}
	mu.Lock()
	fail = false
	mu.Unlock()

	e.cycle(t)
	if e.serverRead(t, older) != 1 || e.serverRead(t, newer) != 1 {
		t.Fatal("both entries read elsewhere should be in the library after the next cycle")
	}
}

func TestCleanupDeletesOnlyOldReadArticlesWithoutIntent(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	oldRead := e.now - 95*day
	oldUnread := e.now - 94*day
	oldReadIntentUnread := e.now - 93*day
	oldReadIntentRead := e.now - 92*day
	recentRead := e.now - 89*day
	all := []int64{oldRead, oldUnread, oldReadIntentUnread, oldReadIntentRead, recentRead}
	// Seed the library as an import would: these are older than the scan window.
	for _, id := range all {
		e.fake.AddItems(freshrsstest.Item{ID: id, FeedID: 1, Title: "t", Read: id != oldUnread})
		e.exec(t, `INSERT INTO articles (item_id, stream_id, published_at, server_read) VALUES (?, 'feed/1', 1, 0)`, id)
		e.exec(t, `INSERT INTO article_contents (item_id, content) VALUES (?, 'body')`, id)
		e.exec(t, `INSERT INTO summaries (item_id, summary, created_at) VALUES (?, 's', 1)`, id)
		e.exec(t, `INSERT INTO article_translations (item_id, source_hash, blocks, created_at) VALUES (?, 'h', '["译"]', 1)`, id)
	}
	e.exec(t, `INSERT INTO pending_read (item_id, value, seq) VALUES (?, 0, 1), (?, 1, 2)`, oldReadIntentUnread, oldReadIntentRead)

	if res := e.pullOnly(t); res.Cleaned != 1 {
		t.Fatalf("cleaned %d, want 1", res.Cleaned)
	}
	if e.serverRead(t, oldRead) != -1 {
		t.Fatal("old read article without intent should be cleaned")
	}
	for _, table := range []string{"summaries", "article_translations"} {
		if n := e.count(t, `SELECT COUNT(*) FROM `+table+` WHERE item_id = ?`, oldRead); n != 0 {
			t.Fatalf("%s row of a cleaned article survived", table)
		}
	}
	for _, id := range []int64{oldUnread, oldReadIntentUnread, oldReadIntentRead, recentRead} {
		if e.serverRead(t, id) == -1 {
			t.Fatalf("article %d should be kept", id)
		}
	}
	if n := e.count(t, `PRAGMA freelist_count`); n != 0 {
		t.Fatalf("freelist_count = %d after incremental_vacuum", n)
	}
}

func TestFirstImage(t *testing.T) {
	cases := map[string]string{
		``:         "",
		`<p>x</p>`: "",
		`<img src="data:image/png;base64,AA"><img src='https://a/b.jpg'>`: "https://a/b.jpg",
		`<IMG alt=x SRC=http://a/c.png>`:                                  "http://a/c.png",
		`<img src="/relative.png">`:                                       "",
		`<img src="javascript:alert(1)">`:                                 "",
	}
	for in, want := range cases {
		if got := firstImage(in); got != want {
			t.Errorf("firstImage(%q) = %q, want %q", in, got, want)
		}
	}
}
