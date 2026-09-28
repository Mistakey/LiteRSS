package update

import (
	"context"
	"time"
)

// Watch is the automatic check (update_check_enabled): shortly after start
// and then once a day, it checks and hands a newer release to Notify, once
// per version for the life of the process.
type Watch struct {
	// Check is (*Checker).Check.
	Check func(context.Context) Result
	// Enabled reads update_check_enabled before every check.
	Enabled func() bool
	// Notify tells the user; it may block until they answer.
	Notify func(Result)
	// Delay is the wait before the first check, Every the wait between checks.
	Delay, Every time.Duration

	notified string
}

// Run checks until ctx ends.
func (w *Watch) Run(ctx context.Context) {
	timer := time.NewTimer(w.Delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		w.tick(ctx)
		timer.Reset(w.Every)
	}
}

func (w *Watch) tick(ctx context.Context) {
	if !w.Enabled() {
		return
	}
	res := w.Check(ctx)
	if !res.UpdateAvailable || res.LatestVersion == w.notified || ctx.Err() != nil {
		return
	}
	w.notified = res.LatestVersion
	w.Notify(res)
}
