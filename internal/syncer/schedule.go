package syncer

import (
	"context"
	"sync"
	"time"
)

// FocusGap is how soon after a cycle started a window focus does not start
// another (spec D6).
const FocusGap = time.Minute

// Scheduler decides when cycles run (spec D6): once at start, every interval,
// and on Trigger (the tray's and the sidebar's sync now, resume from sleep)
// or TriggerOnFocus. Cycles run one at a time on Run's goroutine; triggers
// arriving meanwhile fold into one cycle after it. Every cycle restarts the
// interval.
type Scheduler struct {
	run      func(context.Context)
	interval func() time.Duration
	now      func() time.Time
	wake     chan struct{}

	mu sync.Mutex
	// lastStart is when the latest cycle started; zero before the first.
	lastStart time.Time
}

// NewScheduler returns a Scheduler that runs cycles with run and asks
// interval for the wait after each one, so a changed setting applies from the
// next wait.
func NewScheduler(run func(context.Context), interval func() time.Duration) *Scheduler {
	return &Scheduler{
		run:      run,
		interval: interval,
		now:      time.Now,
		wake:     make(chan struct{}, 1),
	}
}

// Run runs the first cycle, then one per interval or trigger, until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	for ctx.Err() == nil {
		s.mu.Lock()
		s.lastStart = s.now()
		s.mu.Unlock()
		// A trigger that arrived before this cycle started is served by it.
		select {
		case <-s.wake:
		default:
		}
		s.run(ctx)

		timer := time.NewTimer(s.interval())
		select {
		case <-ctx.Done():
		case <-timer.C:
		case <-s.wake:
		}
		timer.Stop()
	}
}

// Trigger asks for a cycle now, or right after the one running.
func (s *Scheduler) Trigger() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// TriggerOnFocus is Trigger for the window gaining focus, skipped when the
// latest cycle started less than FocusGap ago.
func (s *Scheduler) TriggerOnFocus() {
	s.mu.Lock()
	recent := !s.lastStart.IsZero() && s.now().Sub(s.lastStart) < FocusGap
	s.mu.Unlock()
	if !recent {
		s.Trigger()
	}
}
