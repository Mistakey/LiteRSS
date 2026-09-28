package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/desktopapi"
	"LiteRSS/internal/syncer"
)

// fakeStatus is a status at rev whose WaitState waits for ctx unless since
// differs, recording what it was asked.
type fakeStatus struct {
	rev   uint64
	since chan uint64
}

func (f *fakeStatus) WaitState(ctx context.Context, since uint64) syncer.State {
	f.since <- since
	if since != f.rev {
		return syncer.State{Rev: f.rev}
	}
	<-ctx.Done()
	return syncer.State{Rev: f.rev}
}

func TestSyncStateLongPollDefaultsTo25Seconds(t *testing.T) {
	if LongPollTimeout != 25*time.Second {
		t.Fatalf("LongPollTimeout = %v, spec D14 says about 25 s", LongPollTimeout)
	}
}

func TestSyncStateTimesOutWithTheCurrentStatus(t *testing.T) {
	status := &fakeStatus{rev: 7, since: make(chan uint64, 1)}
	h := SyncState(status, 50*time.Millisecond)
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sync/state?since=7", nil))
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("answered before the timeout")
	}
	var st syncer.State
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || st.Rev != 7 || <-status.since != 7 {
		t.Fatalf("status %d, body rev %d", rec.Code, st.Rev)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the status must not be cached")
	}
}

func TestSyncStateSinceIsOptionalAndValidated(t *testing.T) {
	status := &fakeStatus{rev: 7, since: make(chan uint64, 1)}
	h := SyncState(status, time.Minute)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sync/state", nil))
	if rec.Code != http.StatusOK || <-status.since != 0 {
		t.Fatalf("without since: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sync/state?since=-1", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("since=-1: status %d, want 400", rec.Code)
	}
}

// TestClosingTheServiceLetsShutdownFinishAtOnce follows main's exit order:
// the sync service closes first and wakes the long poll, so the listener's
// Shutdown, which waits for requests in progress, is not held for 25 s.
func TestClosingTheServiceLetsShutdownFinishAtOnce(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := syncer.New(db, func() (syncer.Remote, error) { return nil, errors.New("no account") })
	server, err := desktopapi.Start("127.0.0.1:0", Handler(Deps{Sync: svc, SyncNow: func() {}}))
	if err != nil {
		t.Fatal(err)
	}

	status := make(chan int, 1)
	go func() {
		url := fmt.Sprintf("http://%s/api/sync/state?since=%d", server.Address(), svc.State().Rev)
		response, err := http.Get(url)
		if err != nil {
			status <- 0
			return
		}
		response.Body.Close()
		status <- response.StatusCode
	}()
	time.Sleep(100 * time.Millisecond) // let the poll start waiting

	start := time.Now()
	svc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v; the long poll was not woken", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("exit took %v", elapsed)
	}
	if got := <-status; got != http.StatusOK {
		t.Fatalf("the long poll ended with %d, want 200", got)
	}
}
