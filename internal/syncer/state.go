package syncer

import (
	"context"
	"fmt"
	"log"
	"strconv"
)

// State is the sync status the frontend long-polls (spec D14). Rev grows by
// one on every change, so a client that has seen a Rev waits for the next.
type State struct {
	Rev     uint64 `json:"rev"`
	Running bool   `json:"running"`
	// NewItems counts the entries the last cycle added to the library.
	NewItems int `json:"new_items"`
	// Pending counts intent rows not yet pushed.
	Pending int `json:"pending"`
	// LastSyncAt is the Unix time of the last successful cycle, 0 for never.
	LastSyncAt int64 `json:"last_sync_at"`
	// Error is why the last cycle failed, empty after a success.
	Error string `json:"error"`
	// LegacyRunning says the last cycle found the legacy MrRSS running, which
	// turns read articles back to unread (spec D18).
	LegacyRunning bool `json:"legacy_running"`
}

// State returns the current sync status.
func (s *Service) State() State {
	s.loadState()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}

// WaitState returns the status at once when its Rev differs from since, which
// covers a first request with since 0 and a client that saw a previous run.
// Otherwise it waits for the next change, for ctx to end or for Close, and
// returns the status as it then is: unchanged unless a change woke it.
func (s *Service) WaitState(ctx context.Context, since uint64) State {
	s.loadState()
	s.stateMu.Lock()
	st, changed := s.state, s.stateChanged
	s.stateMu.Unlock()
	if st.Rev != since {
		return st
	}
	select {
	case <-changed:
	case <-ctx.Done():
	case <-s.ctx.Done():
	}
	return s.State()
}

// NoteLegacyRunning puts the legacy MrRSS into the status outside a cycle:
// the legacy import holds cycles back while it runs (spec D12), and the next
// cycle probes again.
func (s *Service) NoteLegacyRunning() {
	s.updateState(func(st *State) { st.LegacyRunning = true })
}

// loadState fills the status from the library once, before its first use.
func (s *Service) loadState() {
	s.stateOnce.Do(func() {
		ctx := context.Background()
		var last string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, MetaLastSync).Scan(&last); err == nil {
			s.state.LastSyncAt, _ = strconv.ParseInt(last, 10, 64)
		}
		pending, err := s.countPending(ctx)
		if err != nil {
			log.Printf("sync: %v", err)
		}
		s.state.Pending = pending
	})
}

// updateState applies fn to the status; a change bumps Rev and wakes every
// waiter.
func (s *Service) updateState(fn func(*State)) {
	s.loadState()
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	next := s.state
	fn(&next)
	next.Rev = s.state.Rev
	if next == s.state {
		return
	}
	next.Rev++
	s.state = next
	close(s.stateChanged)
	s.stateChanged = make(chan struct{})
}

// refreshPending brings the status's pending count up to date after intents
// were written or pushed.
func (s *Service) refreshPending(ctx context.Context) {
	s.updateWithPending(ctx, func(*State) {})
}

// updateWithPending applies fn and a fresh pending count in one change. A
// count that fails leaves the old one.
func (s *Service) updateWithPending(ctx context.Context, fn func(*State)) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	pending, err := s.countPending(ctx)
	if err != nil {
		log.Printf("sync: %v", err)
	}
	s.updateState(func(st *State) {
		fn(st)
		if err == nil {
			st.Pending = pending
		}
	})
}

func (s *Service) countPending(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM pending_read) + (SELECT COUNT(*) FROM pending_mark_all)`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count pending intents: %w", err)
	}
	return n, nil
}
