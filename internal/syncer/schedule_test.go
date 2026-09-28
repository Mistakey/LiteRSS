package syncer

import (
	"context"
	"sync"
	"testing"
	"time"
)

// scheduleEnv runs a Scheduler whose cycles only report that they ran, on a
// clock the test moves.
type scheduleEnv struct {
	sched  *Scheduler
	cycles chan struct{}

	mu    sync.Mutex
	clock time.Time
}

func newScheduleEnv(t *testing.T, interval time.Duration) *scheduleEnv {
	t.Helper()
	e := &scheduleEnv{cycles: make(chan struct{}, 16), clock: time.Unix(1_800_000_000, 0)}
	e.sched = NewScheduler(
		func(context.Context) { e.cycles <- struct{}{} },
		func() time.Duration { return interval },
	)
	e.sched.now = func() time.Time {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.clock
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.sched.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return e
}

func (e *scheduleEnv) advance(d time.Duration) {
	e.mu.Lock()
	e.clock = e.clock.Add(d)
	e.mu.Unlock()
}

func (e *scheduleEnv) expectCycle(t *testing.T) {
	t.Helper()
	select {
	case <-e.cycles:
	case <-time.After(time.Second):
		t.Fatal("no cycle ran")
	}
}

func (e *scheduleEnv) expectNoCycle(t *testing.T) {
	t.Helper()
	select {
	case <-e.cycles:
		t.Fatal("a cycle ran")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSchedulerRunsAtStartAndEveryInterval(t *testing.T) {
	e := newScheduleEnv(t, 30*time.Millisecond)
	e.expectCycle(t)
	e.expectCycle(t)
	e.expectCycle(t)
}

func TestTriggerRunsACycleNow(t *testing.T) {
	e := newScheduleEnv(t, time.Hour)
	e.expectCycle(t)
	e.sched.Trigger()
	e.expectCycle(t)
	// Triggers do not wait for a gap.
	e.sched.Trigger()
	e.expectCycle(t)
}

func TestFocusWithinAMinuteOfTheLastCycleIsSkipped(t *testing.T) {
	e := newScheduleEnv(t, time.Hour)
	e.expectCycle(t)

	e.advance(FocusGap - time.Second)
	e.sched.TriggerOnFocus()
	e.expectNoCycle(t)

	e.advance(time.Second)
	e.sched.TriggerOnFocus()
	e.expectCycle(t)
}

func TestTriggersDuringACycleFoldIntoOne(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 16)
	sched := NewScheduler(func(context.Context) {
		started <- struct{}{}
		<-release
	}, func() time.Duration { return time.Hour })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sched.Run(ctx); close(done) }()
	defer func() { cancel(); close(release); <-done }()

	<-started
	sched.Trigger()
	sched.Trigger()
	sched.Trigger()
	release <- struct{}{}
	<-started
	release <- struct{}{}
	select {
	case <-started:
		t.Fatal("three triggers during one cycle ran more than one cycle after it")
	case <-time.After(50 * time.Millisecond):
	}
}
