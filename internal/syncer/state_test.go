package syncer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"LiteRSS/internal/freshrss/freshrsstest"
)

// waitAsync starts WaitState on its own goroutine.
func waitAsync(svc *Service, ctx context.Context, since uint64) <-chan State {
	got := make(chan State, 1)
	go func() { got <- svc.WaitState(ctx, since) }()
	return got
}

func expectState(t *testing.T, got <-chan State, within time.Duration) State {
	t.Helper()
	select {
	case st := <-got:
		return st
	case <-time.After(within):
		t.Fatalf("WaitState did not return within %v", within)
		return State{}
	}
}

func expectBlocked(t *testing.T, got <-chan State) {
	t.Helper()
	select {
	case st := <-got:
		t.Fatalf("WaitState returned %+v without a change", st)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWaitStateReturnsAtOnceForAnotherRev(t *testing.T) {
	e := newEnv(t, nil)
	st := e.svc.WaitState(context.Background(), 0)
	if st.Rev == 0 {
		t.Fatal("the first status must not have rev 0, or a first long poll would wait")
	}
	// A client that saw a previous run's rev gets the current status too.
	if again := e.svc.WaitState(context.Background(), st.Rev+5); again != st {
		t.Fatalf("got %+v, want %+v", again, st)
	}
}

func TestWaitStateReturnsOnChange(t *testing.T) {
	e := newEnv(t, nil)
	e.fake.AddFeeds(freshrsstest.Feed{ID: 1, Title: "One"})
	e.fake.AddItems(freshrsstest.Item{ID: e.now - day, FeedID: 1, URL: "https://x/a"})
	st := e.svc.State()

	got := waitAsync(e.svc, context.Background(), st.Rev)
	expectBlocked(t, got)
	e.cycle(t)
	woke := expectState(t, got, time.Second)
	if woke.Rev <= st.Rev {
		t.Fatalf("woke with rev %d, had %d", woke.Rev, st.Rev)
	}

	// The cycle changed the status twice: running, then done.
	done := e.svc.State()
	if done.Rev != st.Rev+2 || done.Running || done.NewItems != 1 || done.LastSyncAt == 0 || done.Error != "" {
		t.Fatalf("after the cycle: %+v (started at rev %d)", done, st.Rev)
	}
}

func TestWaitStateTimesOutWithTheSameStatus(t *testing.T) {
	e := newEnv(t, nil)
	st := e.svc.State()
	// Read the clock before the deadline is set: taken after, it runs short of
	// the timeout by the gap, which the coarse Windows clock can round below it.
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	got := e.svc.WaitState(ctx, st.Rev)
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("returned before its context ended")
	}
	if got != st {
		t.Fatalf("got %+v, want the unchanged %+v", got, st)
	}
}

func TestCloseWakesWaitState(t *testing.T) {
	e := newEnv(t, nil)
	got := waitAsync(e.svc, context.Background(), e.svc.State().Rev)
	expectBlocked(t, got)
	e.svc.Close()
	expectState(t, got, time.Second)
}

func TestIntentWritesAndPushesUpdatePending(t *testing.T) {
	e := newEnv(t, nil)
	a := e.now - day
	e.seed(t, freshrsstest.Item{ID: a, URL: "https://x/a"})
	st := e.svc.State()

	got := waitAsync(e.svc, context.Background(), st.Rev)
	if err := e.svc.SetRead(context.Background(), a, true); err != nil {
		t.Fatal(err)
	}
	if woke := expectState(t, got, time.Second); woke.Pending != 1 {
		t.Fatalf("after the write: %+v", woke)
	}
	e.svc.pushNow()
	if st := e.svc.State(); st.Pending != 0 {
		t.Fatalf("after the push: %+v", st)
	}
}

func TestFailedCycleKeepsLastSyncAndReportsTheError(t *testing.T) {
	e := newEnv(t, nil)
	e.cycle(t)
	ok := e.svc.State()

	remote := e.svc.remote
	e.svc.remote = func() (Remote, error) { return nil, errors.New("FreshRSS is not configured") }
	if _, err := e.svc.RunCycle(context.Background()); err == nil {
		t.Fatal("want an error")
	}
	st := e.svc.State()
	if st.Error == "" || st.LastSyncAt != ok.LastSyncAt || st.Running {
		t.Fatalf("after a failure: %+v", st)
	}

	e.svc.remote = remote
	e.cycle(t)
	if st := e.svc.State(); st.Error != "" {
		t.Fatalf("a success must clear the error: %+v", st)
	}
}

func TestConcurrentWritesLeaveTheLatestPendingCount(t *testing.T) {
	e := newEnv(t, nil)
	e.svc.debounce = time.Hour
	var items []freshrsstest.Item
	for i := range 20 {
		items = append(items, freshrsstest.Item{ID: e.now - day + int64(i), URL: fmt.Sprintf("https://x/%d", i)})
	}
	e.seed(t, items...)

	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.svc.SetRead(context.Background(), it.ID, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if st := e.svc.State(); st.Pending != len(items) {
		t.Fatalf("pending %d after %d concurrent writes", st.Pending, len(items))
	}
}
