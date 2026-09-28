package routes

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"LiteRSS/internal/syncer"
)

// LongPollTimeout is how long GET /api/sync/state holds a request whose rev
// is current (spec D14). Listeners serving it must not have a shorter write
// timeout (pitfall 31).
const LongPollTimeout = 25 * time.Second

// SyncStatus is the part of the sync service the status route reads;
// *syncer.Service implements it.
type SyncStatus interface {
	WaitState(ctx context.Context, since uint64) syncer.State
}

// SyncState answers GET /api/sync/state?since=<rev>: at once when the status
// has moved past since (a missing since is 0), else when it next changes or
// after timeout, with the status as it then is.
func SyncState(status SyncStatus, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var since uint64
		if raw := r.URL.Query().Get("since"); raw != "" {
			var err error
			if since, err = strconv.ParseUint(raw, 10, 64); err != nil {
				http.Error(w, "since must be a rev", http.StatusBadRequest)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		writeJSON(w, status.WaitState(ctx, since))
	})
}

// SyncRun answers POST /api/sync/run by asking for a cycle, which the
// sidebar's status and the tray call; the status long poll reports its
// progress.
func SyncRun(syncNow func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		syncNow()
		w.WriteHeader(http.StatusAccepted)
	})
}
