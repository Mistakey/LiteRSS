package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/freshrss/freshrsstest"
)

// pending returns an item's intent value, or -1 when it has none.
func (e *env) pending(t *testing.T, id int64) int {
	t.Helper()
	var v int
	err := e.db.QueryRow(`SELECT value FROM pending_read WHERE item_id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// displayRead is the read state the user sees.
func (e *env) displayRead(t *testing.T, id int64) int {
	t.Helper()
	var v int
	if err := e.db.QueryRow(`SELECT `+DisplayRead+` FROM articles a WHERE a.item_id = ?`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (e *env) push(t *testing.T) {
	t.Helper()
	remote, err := e.svc.remote()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.push(context.Background(), remote); err != nil {
		t.Fatalf("push: %v", err)
	}
}

func (e *env) serverItemRead(t *testing.T, id int64) bool {
	t.Helper()
	it, ok := e.fake.Item(id)
	if !ok {
		t.Fatalf("item %d not on the server", id)
	}
	return it.Read
}

// seed puts unread items of feed 1 on the server and into the library.
func (e *env) seed(t *testing.T, items ...freshrsstest.Item) {
	t.Helper()
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One", Labels: []string{"Tech"}}, freshrsstest.Feed{ID: 2, Title: "Two"})
	for i := range items {
		if items[i].FeedID == 0 {
			items[i].FeedID = 1
		}
	}
	e.fake.AddItems(items...)
	e.cycle(t)
}

func TestSetReadCoversTheURLGroupAndIsPushed(t *testing.T) {
	e := newEnv(t, nil)
	a, copyOfA, other := e.now-3*day, e.now-2*day, e.now-day
	e.seed(t,
		freshrsstest.Item{ID: a, URL: "https://x/a"},
		freshrsstest.Item{ID: copyOfA, URL: "https://x/a"},
		freshrsstest.Item{ID: other, URL: "https://x/b"},
	)

	if err := e.svc.SetRead(context.Background(), a, true); err != nil {
		t.Fatal(err)
	}
	if e.pending(t, a) != 1 || e.pending(t, copyOfA) != 1 || e.pending(t, other) != -1 {
		t.Fatal("the intent should cover exactly the URL group")
	}
	if e.displayRead(t, a) != 1 || e.displayRead(t, other) != 0 {
		t.Fatal("display should follow the intent")
	}

	e.push(t)
	if !e.serverItemRead(t, a) || !e.serverItemRead(t, copyOfA) || e.serverItemRead(t, other) {
		t.Fatal("server state after push")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_read`); n != 0 {
		t.Fatalf("%d intents left after a successful push", n)
	}
	// Without its intent the article must not fall back to a stale mirror.
	if e.serverRead(t, a) != 1 || e.displayRead(t, a) != 1 || e.displayRead(t, other) != 0 {
		t.Fatal("the mirror should take the pushed state")
	}
	calls := e.fake.EditTags()
	if len(calls) != 1 || len(calls[0].IDs) != 2 || calls[0].Add != freshrss.StreamRead {
		t.Fatalf("edit-tag calls %+v, want one read call with both ids", calls)
	}
}

// Intents go out before the pull, so the cycle's mirror already carries them.
func TestCyclePushesIntentsBeforePulling(t *testing.T) {
	e := newEnv(t, nil)
	id := e.now - day
	e.seed(t, freshrsstest.Item{ID: id, Read: true})

	if err := e.svc.SetRead(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	e.cycle(t)
	if e.serverItemRead(t, id) || e.serverRead(t, id) != 0 || e.pending(t, id) != -1 {
		t.Fatal("the unread intent should be on the server and mirrored after one cycle")
	}
}

func TestIntentWrittenWhilePushInFlightIsKept(t *testing.T) {
	var (
		once sync.Once
		svc  *Service
		id   int64
	)
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/edit-tag") {
				once.Do(func() {
					if err := svc.SetRead(context.Background(), id, false); err != nil {
						t.Error(err)
					}
				})
			}
			next.ServeHTTP(w, r)
		})
	})
	svc = e.svc
	id = e.now - day
	e.seed(t, freshrsstest.Item{ID: id})

	if err := e.svc.SetRead(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	e.push(t)
	if !e.serverItemRead(t, id) {
		t.Fatal("the read intent was pushed")
	}
	if e.pending(t, id) != 0 {
		t.Fatal("the unread intent written during the push was deleted")
	}
	e.push(t)
	if e.serverItemRead(t, id) || e.pending(t, id) != -1 {
		t.Fatal("the later intent should be pushed on the next pass")
	}
}

func TestRejectedItemIsBisectedAndDroppedAtTheLimit(t *testing.T) {
	var (
		mu       sync.Mutex
		editTags int
	)
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/edit-tag") {
				mu.Lock()
				editTags++
				mu.Unlock()
			}
			next.ServeHTTP(w, r)
		})
	})
	e.svc.maxAttempts = 3
	var ids []int64
	var items []freshrsstest.Item
	for i := range 5 {
		id := e.now - int64(5-i)*day
		ids = append(ids, id)
		items = append(items, freshrsstest.Item{ID: id})
	}
	e.seed(t, items...)
	bad := ids[2]
	e.fake.RejectItem(bad)

	for _, id := range ids {
		if err := e.svc.SetRead(context.Background(), id, true); err != nil {
			t.Fatal(err)
		}
	}
	e.push(t)
	for _, id := range ids {
		if id != bad && !e.serverItemRead(t, id) {
			t.Fatalf("item %d should be pushed despite the rejected one", id)
		}
	}
	var attempts int
	var lastErr string
	if err := e.db.QueryRow(`SELECT attempts, last_error FROM pending_read WHERE item_id = ?`, bad).Scan(&attempts, &lastErr); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastErr == "" {
		t.Fatalf("attempts %d last_error %q", attempts, lastErr)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_read`); n != 1 {
		t.Fatalf("%d intents left, want only the rejected one", n)
	}

	e.push(t)
	e.push(t)
	if e.pending(t, bad) != -1 {
		t.Fatal("the intent should be dropped after maxAttempts rejections")
	}
	mu.Lock()
	before := editTags
	mu.Unlock()
	e.push(t)
	mu.Lock()
	defer mu.Unlock()
	if editTags != before {
		t.Fatal("a dropped intent must not be retried")
	}
}

func TestTransientFailureKeepsIntentsWithoutSpendingAttempts(t *testing.T) {
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/edit-tag") {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	id := e.now - day
	e.seed(t, freshrsstest.Item{ID: id})
	if err := e.svc.SetRead(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	remote, _ := e.svc.remote()
	if err := e.svc.push(context.Background(), remote); err == nil {
		t.Fatal("push should report a server failure")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_read WHERE item_id = ? AND attempts = 0`, id); n != 1 {
		t.Fatal("a server failure must keep the intent and not count against it")
	}
}

func TestMarkItemsReadWritesOnlyDisplayedUnread(t *testing.T) {
	e := newEnv(t, nil)
	unread := e.now - 6*day
	unreadCopy := e.now - 5*day // same URL, not in the request
	serverRead := e.now - 4*day
	intentRead := e.now - 3*day   // server unread, shown read by an intent
	intentUnread := e.now - 2*day // server read, shown unread by an intent
	e.seed(t,
		freshrsstest.Item{ID: unread, URL: "https://x/u"},
		freshrsstest.Item{ID: unreadCopy, URL: "https://x/u"},
		freshrsstest.Item{ID: serverRead, Read: true},
		freshrsstest.Item{ID: intentRead},
		freshrsstest.Item{ID: intentUnread, Read: true},
	)
	ctx := context.Background()
	if err := e.svc.SetRead(ctx, intentRead, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetRead(ctx, intentUnread, false); err != nil {
		t.Fatal(err)
	}
	var seqBefore int
	if err := e.db.QueryRow(`SELECT seq FROM pending_read WHERE item_id = ?`, intentRead).Scan(&seqBefore); err != nil {
		t.Fatal(err)
	}

	b, err := e.svc.MarkItemsRead(ctx, []int64{unread, serverRead, intentRead, intentUnread})
	if err != nil {
		t.Fatal(err)
	}
	if b.Count != 3 || b.Token == "" {
		t.Fatalf("batch %+v, want 3 changed and a token", b)
	}
	for _, id := range []int64{unread, unreadCopy, intentUnread} {
		if e.pending(t, id) != 1 {
			t.Fatalf("item %d should get a read intent", id)
		}
	}
	if e.pending(t, serverRead) != -1 {
		t.Fatal("an item already read must get no intent")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_read WHERE item_id = ? AND seq = ?`, intentRead, seqBefore); n != 1 {
		t.Fatal("an item already shown read must be left alone")
	}
}

func TestMarkStreamReadPushesMarkAllAtTheNewestFetchTime(t *testing.T) {
	e := newEnv(t, nil)
	older, newest := e.now-3*day, e.now-2*day
	elsewhere := e.now - day // feed 2
	e.seed(t,
		freshrsstest.Item{ID: older},
		freshrsstest.Item{ID: newest},
		freshrsstest.Item{ID: elsewhere, FeedID: 2},
	)
	ctx := context.Background()
	// An unread intent the mark-all supersedes.
	if err := e.svc.SetRead(ctx, older, false); err != nil {
		t.Fatal(err)
	}

	b, err := e.svc.MarkStreamRead(ctx, "feed/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if b.Count != 2 {
		t.Fatalf("changed %d, want 2", b.Count)
	}
	if e.displayRead(t, older) != 1 || e.displayRead(t, newest) != 1 || e.displayRead(t, elsewhere) != 0 {
		t.Fatal("display after mark-all")
	}
	if e.pending(t, older) != -1 {
		t.Fatal("the covered unread intent should be superseded")
	}

	// Crawled after the user looked: above ts, so it must stay unread.
	later := e.fake.NextID()
	e.fake.AddItems(freshrsstest.Item{ID: later, FeedID: 1})

	e.push(t)
	got := e.fake.MarkAlls()
	if len(got) != 1 || got[0].Stream != "feed/1" || got[0].TS != newest {
		t.Fatalf("mark-all calls %+v, want feed/1 at %d", got, newest)
	}
	if !e.serverItemRead(t, older) || !e.serverItemRead(t, newest) || e.serverItemRead(t, later) || e.serverItemRead(t, elsewhere) {
		t.Fatal("server state after mark-all")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all`); n != 0 {
		t.Fatal("pushed mark-all intent left behind")
	}
	if e.displayRead(t, older) != 1 || e.displayRead(t, newest) != 1 || e.displayRead(t, elsewhere) != 0 {
		t.Fatal("the mirror should take the pushed mark-all")
	}
}

func TestMarkStreamReadTakesTheGivenTSAndLabels(t *testing.T) {
	e := newEnv(t, nil)
	a, b := e.now-3*day, e.now-2*day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: b})

	res, err := e.svc.MarkStreamRead(context.Background(), freshrss.LabelPrefix+"Tech", a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 || e.displayRead(t, a) != 1 || e.displayRead(t, b) != 0 {
		t.Fatal("only items up to the given ts are covered")
	}
	if _, err := e.svc.MarkStreamRead(context.Background(), "bogus", 0); err == nil {
		t.Fatal("an unknown stream id should be refused")
	}
}

func TestUndoOfUnpushedMarkAllDeletesIt(t *testing.T) {
	e := newEnv(t, nil)
	a, readByIntent := e.now-3*day, e.now-2*day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: readByIntent, Read: true})
	ctx := context.Background()
	if err := e.svc.SetRead(ctx, readByIntent, false); err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.MarkStreamRead(ctx, freshrss.StreamReadingList, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Undo(ctx, b.Token); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all`); n != 0 {
		t.Fatal("undo should delete the unpushed mark-all")
	}
	if e.pending(t, a) != -1 {
		t.Fatal("an item the server has unread needs no intent")
	}
	if e.displayRead(t, a) != 0 || e.displayRead(t, readByIntent) != 0 {
		t.Fatal("both items should show unread again")
	}
	e.push(t)
	if len(e.fake.MarkAlls()) != 0 || e.serverItemRead(t, readByIntent) {
		t.Fatal("nothing but the unread intent should reach the server")
	}
}

func TestUndoOfPushedMarkAllWritesUnreadIntents(t *testing.T) {
	e := newEnv(t, nil)
	a, b := e.now-3*day, e.now-2*day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: b})
	ctx := context.Background()
	res, err := e.svc.MarkStreamRead(ctx, "feed/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	e.push(t)

	if err := e.svc.Undo(ctx, res.Token); err != nil {
		t.Fatal(err)
	}
	if e.pending(t, a) != 0 || e.pending(t, b) != 0 {
		t.Fatal("undo after the push should write unread intents")
	}
	e.push(t)
	if e.serverItemRead(t, a) || e.serverItemRead(t, b) {
		t.Fatal("the server should have both unread again")
	}
}

func TestUndoOfBatchByID(t *testing.T) {
	e := newEnv(t, nil)
	a, b, kept := e.now-3*day, e.now-2*day, e.now-day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: b}, freshrsstest.Item{ID: kept, Read: true})
	ctx := context.Background()
	res, err := e.svc.MarkItemsRead(ctx, []int64{a, b, kept})
	if err != nil {
		t.Fatal(err)
	}
	e.push(t)
	if err := e.svc.Undo(ctx, res.Token); err != nil {
		t.Fatal(err)
	}
	e.push(t)
	if e.serverItemRead(t, a) || e.serverItemRead(t, b) || !e.serverItemRead(t, kept) {
		t.Fatal("undo should restore exactly what the batch changed")
	}
}

func TestUndoTokenExpiresAndIsSingleUse(t *testing.T) {
	e := newEnv(t, nil)
	a, b := e.now-3*day, e.now-2*day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: b})
	ctx := context.Background()
	clock := time.Now()
	e.svc.now = func() time.Time { return clock }

	first, err := e.svc.MarkItemsRead(ctx, []int64{a})
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(UndoTTL + time.Second)
	if err := e.svc.Undo(ctx, first.Token); !errors.Is(err, ErrUndoExpired) {
		t.Fatalf("undo after the TTL: %v, want ErrUndoExpired", err)
	}
	if e.displayRead(t, a) != 1 {
		t.Fatal("an expired undo must change nothing")
	}

	second, err := e.svc.MarkItemsRead(ctx, []int64{b})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Undo(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Undo(ctx, second.Token); !errors.Is(err, ErrUndoExpired) {
		t.Fatalf("second undo: %v, want ErrUndoExpired", err)
	}
}

func TestWritesArePushedAfterTheDebounce(t *testing.T) {
	e := newEnv(t, nil)
	e.svc.debounce = 100 * time.Millisecond
	a, b := e.now-2*day, e.now-day
	e.seed(t, freshrsstest.Item{ID: a}, freshrsstest.Item{ID: b})
	ctx := context.Background()
	if err := e.svc.SetRead(ctx, a, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetRead(ctx, b, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.count(t, `SELECT COUNT(*) FROM pending_read`) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("intents were not pushed after the debounce")
		}
		time.Sleep(20 * time.Millisecond)
	}
	calls := e.fake.EditTags()
	var ids []int64
	for _, c := range calls {
		ids = append(ids, c.IDs...)
	}
	slices.Sort(ids)
	if len(calls) != 1 || !slices.Equal(ids, []int64{a, b}) {
		t.Fatalf("edit-tag calls %+v, want both writes in one request", calls)
	}
}

// An undo arriving while the mark-all request is on its way cannot know
// whether the server applied it, so it writes unread intents.
func TestUndoWhileMarkAllInFlightWritesUnreadIntents(t *testing.T) {
	var (
		once  sync.Once
		svc   *Service
		token string
	)
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/mark-all-as-read") {
				once.Do(func() {
					if err := svc.Undo(context.Background(), token); err != nil {
						t.Error(err)
					}
				})
			}
			next.ServeHTTP(w, r)
		})
	})
	svc = e.svc
	a := e.now - day
	e.seed(t, freshrsstest.Item{ID: a})
	res, err := e.svc.MarkStreamRead(context.Background(), "feed/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	token = res.Token

	// The unread intent is written during the push and goes out in the same
	// pass, after the mark-all.
	e.push(t)
	if e.serverItemRead(t, a) || e.pending(t, a) != -1 {
		t.Fatal("the undo should win over the mark-all that was in flight")
	}
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all`); n != 0 {
		t.Fatal("the sent mark-all intent should be gone")
	}
}

// Read intents a mark-all covers agree with it and survive, so undoing a
// mark-all that was never sent leaves the user's earlier reads in place.
func TestUndoOfMarkAllKeepsEarlierReadIntents(t *testing.T) {
	e := newEnv(t, nil)
	readEarlier, other := e.now-3*day, e.now-2*day
	e.seed(t, freshrsstest.Item{ID: readEarlier}, freshrsstest.Item{ID: other})
	ctx := context.Background()
	if err := e.svc.SetRead(ctx, readEarlier, true); err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.MarkStreamRead(ctx, "feed/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Undo(ctx, b.Token); err != nil {
		t.Fatal(err)
	}
	if e.displayRead(t, readEarlier) != 1 || e.displayRead(t, other) != 0 {
		t.Fatal("undo should restore the state before the mark-all, earlier read included")
	}
	e.push(t)
	if !e.serverItemRead(t, readEarlier) || e.serverItemRead(t, other) {
		t.Fatal("the earlier read should still reach the server")
	}
}

// A mark-all covers the URL group like any read: a copy in another feed is
// marked too, and undone with it.
func TestMarkStreamReadCoversCopiesInOtherFeeds(t *testing.T) {
	e := newEnv(t, nil)
	inFeed, copyElsewhere := e.now-3*day, e.now-2*day
	e.seed(t,
		freshrsstest.Item{ID: inFeed, URL: "https://x/same"},
		freshrsstest.Item{ID: copyElsewhere, FeedID: 2, URL: "https://x/same"},
	)
	ctx := context.Background()
	b, err := e.svc.MarkStreamRead(ctx, "feed/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if b.Count != 2 || e.displayRead(t, copyElsewhere) != 1 {
		t.Fatalf("batch %+v: the copy in feed 2 should be marked too", b)
	}
	e.push(t)
	if !e.serverItemRead(t, copyElsewhere) {
		t.Fatal("the copy should be read on the server")
	}
	if err := e.svc.Undo(ctx, b.Token); err != nil {
		t.Fatal(err)
	}
	e.push(t)
	if e.serverItemRead(t, inFeed) || e.serverItemRead(t, copyElsewhere) {
		t.Fatal("undo should restore both")
	}
}

func TestItemIntentsAreSplitIntoRequestsOfPushBatch(t *testing.T) {
	e := newEnv(t, nil)
	e.svc.pushBatch = 2
	var items []freshrsstest.Item
	for i := range 5 {
		items = append(items, freshrsstest.Item{ID: e.now - int64(i+1)*day})
	}
	e.seed(t, items...)
	ctx := context.Background()
	for _, it := range items[:3] {
		if err := e.svc.SetRead(ctx, it.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, it := range items[3:] {
		if err := e.svc.SetRead(ctx, it.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	e.push(t)
	var sizes []string
	for _, c := range e.fake.EditTags() {
		kind := "read"
		if c.Remove != "" {
			kind = "unread"
		}
		sizes = append(sizes, fmt.Sprintf("%s:%d", kind, len(c.IDs)))
	}
	if want := []string{"read:2", "read:1", "unread:2"}; !slices.Equal(sizes, want) {
		t.Fatalf("requests %v, want %v", sizes, want)
	}
}

func TestRejectedMarkAllIsDroppedAtTheLimit(t *testing.T) {
	e := newEnv(t, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/mark-all-as-read") {
				http.Error(w, "no", http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	e.svc.maxAttempts = 2
	id := e.now - day
	e.seed(t, freshrsstest.Item{ID: id})
	if _, err := e.svc.MarkStreamRead(context.Background(), "feed/1", 0); err != nil {
		t.Fatal(err)
	}
	e.push(t)
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all WHERE attempts = 1 AND last_error <> ''`); n != 1 {
		t.Fatal("a rejected mark-all should spend one attempt")
	}
	e.push(t)
	if n := e.count(t, `SELECT COUNT(*) FROM pending_mark_all`); n != 0 {
		t.Fatal("the mark-all should be dropped at the limit")
	}
	if e.displayRead(t, id) != 0 {
		t.Fatal("after the drop the article follows the mirror")
	}
}
