package update

import (
	"context"
	"testing"
	"time"
)

func TestWatchNotifiesOncePerVersion(t *testing.T) {
	enabled := true
	answer := Result{CurrentVersion: "1.0.0"}
	checks := 0
	var told []string
	w := &Watch{
		Check:   func(context.Context) Result { checks++; return answer },
		Enabled: func() bool { return enabled },
		Notify:  func(r Result) { told = append(told, r.LatestVersion) },
	}
	ctx := context.Background()

	w.tick(ctx) // up to date
	answer = Result{CurrentVersion: "1.0.0", Message: "连不上 GitHub，请检查网络或代理设置。"}
	w.tick(ctx) // failed
	answer = Result{CurrentVersion: "1.0.0", LatestVersion: "1.1.0", UpdateAvailable: true}
	w.tick(ctx)
	w.tick(ctx) // same version again
	enabled = false
	answer.LatestVersion = "1.2.0"
	w.tick(ctx) // switched off: not even checked
	enabled = true
	w.tick(ctx)

	if checks != 5 {
		t.Errorf("checks = %d, want 5 (none while switched off)", checks)
	}
	if len(told) != 2 || told[0] != "1.1.0" || told[1] != "1.2.0" {
		t.Errorf("notified %v, want [1.1.0 1.2.0]", told)
	}
}

func TestWatchRunStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	checked := make(chan struct{}, 10)
	w := &Watch{
		Check:   func(context.Context) Result { checked <- struct{}{}; return Result{} },
		Enabled: func() bool { return true },
		Notify:  func(Result) {},
		Delay:   time.Millisecond,
		Every:   time.Hour,
	}
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	select {
	case <-checked:
	case <-time.After(5 * time.Second):
		t.Fatal("no check after the delay")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
