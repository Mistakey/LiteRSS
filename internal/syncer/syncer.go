// Package syncer keeps the local library in step with FreshRSS (spec D6, D9).
// FreshRSS owns subscriptions, tags, entries and the read state; the pull
// writes only those columns and never turns a read on the server back to
// unread. The user's own changes live in the intent tables, which the pull
// leaves alone.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
)

// Retention is how long a read article stays in the library, counted from its
// crawl time. It is fixed, not a setting (spec D9).
const Retention = 90 * 24 * time.Hour

// MetaLastSync is the meta key holding the Unix time of the last successful cycle.
const MetaLastSync = "last_sync_at"

// Remote is the part of the FreshRSS client a cycle uses; *freshrss.Client
// implements it.
type Remote interface {
	GetSubscriptions(ctx context.Context) ([]freshrss.Subscription, error)
	GetCategories(ctx context.Context) ([]freshrss.Category, error)
	StreamItemIDs(ctx context.Context, q freshrss.ItemIDQuery) (freshrss.ItemIDPage, error)
	StreamItemContents(ctx context.Context, ids []int64) ([]freshrss.Item, error)
	MarkAsRead(ctx context.Context, itemIDs []int64) error
	MarkAsUnread(ctx context.Context, itemIDs []int64) error
	MarkAllAsRead(ctx context.Context, streamID string, olderThan int64) error
}

// Service runs sync cycles against one library. Cycles never overlap.
type Service struct {
	db     *database.DB
	remote func() (Remote, error)
	now    func() time.Time

	// idPageSize is n for id requests: large enough that one request takes
	// a whole set, since paging can lose an entry (pitfall 28).
	idPageSize int
	// contentsBatch is how many ids one stream/items/contents request carries.
	contentsBatch int
	// pushBatch is how many ids one edit-tag request carries.
	pushBatch int
	// maxAttempts is how many rejections an intent survives before it is
	// dropped and the article follows the mirror again.
	maxAttempts int
	// debounce is how long after the last write the pusher waits; backoffMin
	// and backoffMax bound the retry delay after a failed push.
	debounce, backoffMin, backoffMax time.Duration

	// mu serializes cycles and pushes.
	mu sync.Mutex

	// intentMu serializes intent writes with each other and with the
	// pusher's bookkeeping; it is never held across a request.
	intentMu sync.Mutex
	// inflight holds the pending_mark_all rows being sent right now, which
	// an undo may no longer simply delete.
	inflight map[int64]bool
	undos    map[string]undoEntry

	// timerMu guards the push schedule.
	timerMu sync.Mutex
	timer   *time.Timer
	backoff time.Duration
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc

	// stateMu guards state and stateChanged, which is closed and replaced on
	// every change to wake the long polls waiting on it.
	stateMu      sync.Mutex
	stateOnce    sync.Once
	state        State
	stateChanged chan struct{}
	// pendingMu makes counting the pending intents and publishing the count
	// one step, so a slower count never overwrites a newer one.
	pendingMu sync.Mutex
}

// New returns a Service. remote is asked for a client at the start of every
// cycle, so a changed configuration takes effect on the next one.
func New(db *database.DB, remote func() (Remote, error)) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		db:            db,
		remote:        remote,
		now:           time.Now,
		idPageSize:    100000,
		contentsBatch: 100,
		pushBatch:     250,
		maxAttempts:   5,
		debounce:      time.Second,
		backoffMin:    5 * time.Second,
		backoffMax:    5 * time.Minute,
		inflight:      map[int64]bool{},
		undos:         map[string]undoEntry{},
		ctx:           ctx,
		cancel:        cancel,
		state:         State{Rev: 1},
		stateChanged:  make(chan struct{}),
	}
}

// Close stops the pusher, wakes every WaitState and waits for a push or cycle
// in progress. Intents not yet pushed stay in the library for the next run.
func (s *Service) Close() {
	s.timerMu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timerMu.Unlock()
	s.cancel()
	// Wait out a push or cycle in progress.
	s.mu.Lock()
	s.mu.Unlock()
}

// Result says what a cycle changed in the library.
type Result struct {
	// Added counts entries new to the library.
	Added int
	// Removed counts articles deleted because their feed was unsubscribed.
	Removed int
	// Cleaned counts read articles deleted for being past retention.
	Cleaned int
}

// RunCycle runs one sync cycle; a caller arriving while one runs waits for it
// to finish and then runs its own. It first sends intents, so the pull already sees them on the server. A failed push
// does not hold the pull back, since the pull never touches intents, but it
// fails the cycle. The status changes as the cycle starts and ends.
func (s *Service) RunCycle(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updateState(func(st *State) { st.Running = true })
	res, syncedAt, err := s.runCycle(ctx)
	s.updateWithPending(ctx, func(st *State) {
		st.Running = false
		st.NewItems = res.Added
		if err != nil {
			st.Error = err.Error()
			return
		}
		st.Error = ""
		st.LastSyncAt = syncedAt
	})
	return res, err
}

// runCycle is RunCycle's work; it returns the time recorded as the last sync.
func (s *Service) runCycle(ctx context.Context) (Result, int64, error) {
	remote, err := s.remote()
	if err != nil {
		return Result{}, 0, fmt.Errorf("sync: %w", err)
	}
	pushErr := s.push(ctx, remote)
	s.pushed(pushErr)
	res, err := s.pull(ctx, remote)
	if err != nil {
		return res, 0, fmt.Errorf("sync: %w", errors.Join(pushErr, err))
	}
	if res.Cleaned, err = s.cleanup(ctx); err != nil {
		return res, 0, fmt.Errorf("sync: %w", errors.Join(pushErr, err))
	}
	if pushErr != nil {
		return res, 0, fmt.Errorf("sync: %w", pushErr)
	}
	syncedAt := s.now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		MetaLastSync, strconv.FormatInt(syncedAt, 10)); err != nil {
		return res, 0, fmt.Errorf("sync: record last sync: %w", err)
	}
	return res, syncedAt, nil
}
