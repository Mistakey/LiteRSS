package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"LiteRSS/internal/browser"
	"LiteRSS/internal/database"
	"LiteRSS/internal/enrich"
	"LiteRSS/internal/library"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/syncer"
)

type testAPI struct {
	db        *database.DB
	lib       *library.Library
	svc       *syncer.Service
	enrich    *enrich.Service
	handler   http.Handler
	triggered int
	store     *settings.Store
	opened    []string
	updates   Updater
	window    fakeWindow
	autostart []bool
	// autostartErr is what the system answers a change to startup_on_boot.
	autostartErr error
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := syncer.New(db, func() (syncer.Remote, error) { return nil, errors.New("no FreshRSS in handler tests") })
	t.Cleanup(svc.Close)
	api := &testAPI{db: db, lib: library.New(db.DB), svc: svc, store: settings.New(db.DB),
		enrich: enrich.New(db.DB, noSettings{}, enrich.Clients{Web: &http.Client{}, API: &http.Client{}})}
	api.handler = Handler(api.deps())
	return api
}

func (a *testAPI) panel() *settings.Panel {
	panel := settings.NewPanel(a.store, &http.Client{}, func() { a.triggered++ })
	panel.Autostart = func(on bool) error {
		a.autostart = append(a.autostart, on)
		return a.autostartErr
	}
	return panel
}

func (a *testAPI) deps() Deps {
	return Deps{
		Sync:    &fakeStatus{rev: 1, since: make(chan uint64, 100)},
		SyncNow: func() { a.triggered++ },
		Library: a.lib,
		Intents: a.svc,
		Enrich:  a.enrich,

		Settings: a.panel(),
		Browser:  browser.New(func(u string) error { a.opened = append(a.opened, u); return nil }),
		Updates:  a.updates,
		Window:   &a.window,
	}
}

func (a *testAPI) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := a.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (a *testAPI) do(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func (a *testAPI) getJSON(t *testing.T, target string, v any) {
	t.Helper()
	rec := a.do(t, http.MethodGet, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", target, rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("GET %s: Cache-Control %q", target, got)
	}
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
}

var wildcard = regexp.MustCompile(`\{[^}]+\}`)

// concretePath fills a pattern's wildcards with an item id.
func concretePath(path string) string {
	return wildcard.ReplaceAllString(path, "1")
}

// TestRouteTable checks the method rule over the whole table (spec D13):
// a route that changes state is not GET, and a GET to it is answered 405
// without running its handler.
func TestRouteTable(t *testing.T) {
	api := newTestAPI(t)
	table := Table(api.deps())

	patterns := map[string]bool{}
	for _, r := range table {
		if !slices.Contains([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}, r.Method) {
			t.Errorf("%s: method must be GET, POST, PUT or DELETE", r.Pattern())
		}
		if r.Mutates == (r.Method == http.MethodGet) {
			t.Errorf("%s: Mutates is %v; state-changing routes, and only they, take POST, PUT or DELETE", r.Pattern(), r.Mutates)
		}
		if !strings.HasPrefix(r.Path, "/api/") {
			t.Errorf("%s: path outside /api/", r.Pattern())
		}
		if r.Handler == nil {
			t.Errorf("%s: no handler", r.Pattern())
		}
		if patterns[r.Pattern()] {
			t.Errorf("%s: listed twice", r.Pattern())
		}
		patterns[r.Pattern()] = true
	}
	// The routes known to change state; the rule above covers later ones.
	for _, p := range []string{
		"POST /api/sync/run",
		"POST /api/articles/{id}/read", "POST /api/articles/read", "POST /api/streams/read", "POST /api/undo",
		"POST /api/articles/{id}/fulltext", "POST /api/articles/translate-titles", "POST /api/articles/{id}/summary",
		"POST /api/settings/update", "POST /api/settings/freshrss/test", "POST /api/settings/llm/test",
		"POST /api/browser/open", "POST /api/update/start",
	} {
		if !patterns[p] {
			t.Errorf("state-changing route %s is not in the table", p)
		}
	}

	// Every handler records its runs, so a GET that reached one shows.
	ran := map[string]int{}
	probed := slices.Clone(table)
	for i, r := range probed {
		pattern, next := r.Pattern(), r.Handler
		probed[i].Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ran[pattern]++
			next.ServeHTTP(w, req)
		})
	}
	mux := Mount(probed)
	for _, r := range probed {
		if !r.Mutates {
			continue
		}
		t.Run(r.Pattern(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, concretePath(r.Path), nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET %s: %d, want 405", r.Path, rec.Code)
			}
			if len(ran) != 0 {
				t.Fatalf("GET %s ran %v", r.Path, ran)
			}
		})
	}
}

func TestSyncRun(t *testing.T) {
	api := newTestAPI(t)
	rec := api.do(t, http.MethodPost, "/api/sync/run")
	if rec.Code != http.StatusAccepted || api.triggered != 1 {
		t.Fatalf("POST sync/run: status %d, triggered %d", rec.Code, api.triggered)
	}
}

func TestVersion(t *testing.T) {
	var got map[string]string
	newTestAPI(t).getJSON(t, "/api/version", &got)
	if got["version"] == "" {
		t.Fatalf("version = %v", got)
	}
}

// TestArticleSnapshotIsStableWhenNewItemsArrive takes a snapshot, lets a
// sync add items, and reads the list on: the held snapshot's cards are the
// same members, and only a new snapshot has the new items (spec D8).
func TestArticleSnapshotIsStableWhenNewItemsArrive(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO feeds (stream_id, title) VALUES ('feed/1', 'One'), ('feed/2', 'Two')`)
	for i := int64(1); i <= 5; i++ {
		api.exec(t, `INSERT INTO articles (item_id, stream_id, published_at) VALUES (?, 'feed/1', ?)`, i*100, i*10)
	}
	api.exec(t, `INSERT INTO articles (item_id, stream_id, published_at, server_read) VALUES (600, 'feed/2', 60, 1)`)

	var snap library.Snapshot
	api.getJSON(t, "/api/articles", &snap)
	want := []int64{500, 400, 300, 200, 100}
	if !slices.Equal(snap.IDs, want) || snap.Newest != 500 {
		t.Fatalf("snapshot = %+v", snap)
	}

	// A sync brings an item fetched now but published long ago, and the
	// user reads one of the list.
	api.exec(t, `INSERT INTO articles (item_id, stream_id, published_at) VALUES (900, 'feed/1', 15)`)
	api.exec(t, `INSERT INTO pending_read (item_id, value, seq) VALUES (300, 1, 1)`)
	var cards []library.Card
	api.getJSON(t, "/api/articles/cards?ids="+joinIDs(snap.IDs), &cards)
	var got []int64
	for _, c := range cards {
		got = append(got, c.ID)
		if c.Read != (c.ID == 300) {
			t.Fatalf("card %d read = %v", c.ID, c.Read)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("cards of the held snapshot = %v, want %v", got, want)
	}

	var fresh library.Snapshot
	api.getJSON(t, "/api/articles?view=unread", &fresh)
	if !slices.Equal(fresh.IDs, []int64{500, 400, 200, 900, 100}) || fresh.Newest != 900 {
		t.Fatalf("a new snapshot = %+v", fresh)
	}
	var all library.Snapshot
	api.getJSON(t, "/api/articles?view=all&stream="+url.QueryEscape("feed/2"), &all)
	if !slices.Equal(all.IDs, []int64{600}) {
		t.Fatalf("feed/2, all = %+v", all)
	}
}

func TestArticleSnapshotRejectsBadInput(t *testing.T) {
	api := newTestAPI(t)
	for _, target := range []string{
		"/api/articles?view=starred",
		"/api/articles?stream=user/-/state/com.google/starred",
	} {
		if rec := api.do(t, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: %d, want 400", target, rec.Code)
		}
	}
	var empty library.Snapshot
	api.getJSON(t, "/api/articles?view=all", &empty)
	if empty.IDs == nil || len(empty.IDs) != 0 {
		t.Fatalf("empty library: %+v, want ids []", empty)
	}
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func TestArticleCards(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO articles (item_id, stream_id, title, published_at) VALUES (100, 'feed/1', 'A', 1), (200, 'feed/1', 'B', 2)`)
	api.exec(t, `INSERT INTO article_contents (item_id, content) VALUES (200, '<p>Hi <img src=x onerror=alert(1)>there</p>')`)

	var cards []library.Card
	api.getJSON(t, "/api/articles/cards?ids=200,300,100", &cards)
	if len(cards) != 2 || cards[0].ID != 200 || cards[1].ID != 100 || cards[0].Excerpt != "Hi there" {
		t.Fatalf("cards = %+v", cards)
	}
	api.getJSON(t, "/api/articles/cards", &cards)
	if cards == nil || len(cards) != 0 {
		t.Fatalf("no ids: %+v, want []", cards)
	}

	many := strings.Repeat("1,", library.MaxCards) + "1"
	for _, target := range []string{"/api/articles/cards?ids=1,x", "/api/articles/cards?ids=1,", "/api/articles/cards?ids=" + many} {
		if rec := api.do(t, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %.60s: %d, want 400", target, rec.Code)
		}
	}
}

func TestArticleContent(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO articles (item_id, stream_id, published_at) VALUES (100, 'feed/1', 1)`)
	api.exec(t, `INSERT INTO article_contents (item_id, content) VALUES (100, '<p>body</p>')`)
	api.exec(t, `INSERT INTO fulltext_cache (item_id, content, cached_at) VALUES (100, '<p>full</p>', 1)`)

	var got map[string]string
	api.getJSON(t, "/api/articles/100/content", &got)
	if got["content"] != "<p>body</p>" || got["fulltext"] != "<p>full</p>" {
		t.Fatalf("content = %v", got)
	}
	if rec := api.do(t, http.MethodGet, "/api/articles/300/content"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing article: %d, want 404", rec.Code)
	}
	if rec := api.do(t, http.MethodGet, "/api/articles/x/content"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d, want 400", rec.Code)
	}
}

func TestUnreadCountsFoldTheSameURL(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO articles (item_id, stream_id, url, published_at) VALUES
		(100, 'feed/1', 'https://x/1', 1), (200, 'feed/2', 'https://x/1', 1), (300, 'feed/2', 'https://x/2', 1)`)

	var counts library.Counts
	api.getJSON(t, "/api/unread-counts", &counts)
	if counts.Total != 2 || counts.Feeds["feed/1"] != 1 || counts.Feeds["feed/2"] != 2 || counts.Tags == nil {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestSubscriptions(t *testing.T) {
	api := newTestAPI(t)
	api.exec(t, `INSERT INTO feeds (stream_id, title) VALUES ('feed/1', 'One')`)

	var tree library.Tree
	api.getJSON(t, "/api/subscriptions", &tree)
	if len(tree.Categories) != 0 || tree.Categories == nil || len(tree.Feeds) != 1 || tree.Feeds[0].ID != "feed/1" {
		t.Fatalf("tree = %+v", tree)
	}
}
