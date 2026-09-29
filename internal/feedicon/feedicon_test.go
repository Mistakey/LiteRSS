package feedicon

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"LiteRSS/internal/database"
	"LiteRSS/internal/freshrss"
)

var png = freshrss.Icon{Data: []byte("\x89PNG\r\n\x1a\nicon"), ContentType: "image/png"}

// fakeSource answers every iconUrl with icon and err, counting the calls.
type fakeSource struct {
	mu    sync.Mutex
	icon  freshrss.Icon
	err   error
	calls atomic.Int32
	// gate, when set, holds each call until it is closed.
	gate chan struct{}
}

func (f *fakeSource) Icon(ctx context.Context, _ string) (freshrss.Icon, error) {
	f.calls.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.icon, f.err
}

func (f *fakeSource) answer(icon freshrss.Icon, err error) {
	f.mu.Lock()
	f.icon, f.err = icon, err
	f.mu.Unlock()
}

type env struct {
	db  *database.DB
	src *fakeSource
	svc *Service
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "lib.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{db: db, src: &fakeSource{icon: png}, now: time.Unix(1_800_000_000, 0)}
	e.svc = New(db.DB, func() (Source, error) { return e.src, nil })
	e.svc.now = func() time.Time { return e.now }
	e.exec(t, `INSERT INTO feeds (stream_id, title, icon_url) VALUES
		('feed/1', 'One', 'http://rss/f.php?h=1'), ('feed/2', 'Two', '')`)
	return e
}

func (e *env) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func (e *env) icon(t *testing.T, streamID string) (freshrss.Icon, error) {
	t.Helper()
	return e.svc.Icon(context.Background(), streamID)
}

func TestIconIsFetchedOnceAndKept(t *testing.T) {
	e := newEnv(t)
	for range 2 {
		icon, err := e.icon(t, "feed/1")
		if err != nil || string(icon.Data) != string(png.Data) || icon.ContentType != "image/png" {
			t.Fatalf("Icon = %q %q, %v; want the PNG", icon.Data, icon.ContentType, err)
		}
	}
	if n := e.src.calls.Load(); n != 1 {
		t.Errorf("FreshRSS asked %d times, want 1", n)
	}
}

func TestChangedIconURLFetchesAgain(t *testing.T) {
	e := newEnv(t)
	if _, err := e.icon(t, "feed/1"); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE feeds SET icon_url = 'http://rss/f.php?h=1&t=2' WHERE stream_id = 'feed/1'`)
	newer := freshrss.Icon{Data: []byte("GIF89a new"), ContentType: "image/gif"}
	e.src.answer(newer, nil)
	icon, err := e.icon(t, "feed/1")
	if err != nil || string(icon.Data) != string(newer.Data) {
		t.Errorf("Icon after iconUrl change = %q, %v; want the new icon", icon.Data, err)
	}
}

func TestUnknownFeedOrNoIconURLIsNoIcon(t *testing.T) {
	e := newEnv(t)
	for _, id := range []string{"feed/9", "feed/2"} {
		if _, err := e.icon(t, id); !errors.Is(err, ErrNoIcon) {
			t.Errorf("Icon(%s) = %v, want ErrNoIcon", id, err)
		}
	}
	if n := e.src.calls.Load(); n != 0 {
		t.Errorf("FreshRSS asked %d times, want 0", n)
	}
}

// FreshRSS having no icon (its placeholder, or a refusal) is remembered for
// NoIconRetry, then asked again.
func TestNoIconIsRememberedThenRetried(t *testing.T) {
	for name, answer := range map[string]error{
		"placeholder": freshrss.ErrNoIcon,
		"refused":     &freshrss.APIError{Op: "favicon", StatusCode: 404},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.src.answer(freshrss.Icon{}, answer)
			for range 2 {
				if _, err := e.icon(t, "feed/1"); !errors.Is(err, ErrNoIcon) {
					t.Fatalf("Icon = %v, want ErrNoIcon", err)
				}
			}
			if n := e.src.calls.Load(); n != 1 {
				t.Errorf("FreshRSS asked %d times within the retry window, want 1", n)
			}

			e.now = e.now.Add(NoIconRetry)
			e.src.answer(png, nil)
			if icon, err := e.icon(t, "feed/1"); err != nil || len(icon.Data) == 0 {
				t.Errorf("Icon after the retry window = %v, want the icon", err)
			}
		})
	}
}

// Not reaching FreshRSS says nothing about the icon: the next request asks
// again.
func TestUnavailableIsNotRemembered(t *testing.T) {
	e := newEnv(t)
	e.src.answer(freshrss.Icon{}, errors.New("dial tcp: connection refused"))
	if _, err := e.icon(t, "feed/1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Icon = %v, want ErrUnavailable", err)
	}
	e.src.answer(png, nil)
	if _, err := e.icon(t, "feed/1"); err != nil {
		t.Errorf("Icon once FreshRSS answers = %v", err)
	}

	unconfigured := New(e.db.DB, func() (Source, error) { return nil, errors.New("not configured") })
	if _, err := unconfigured.Icon(context.Background(), "feed/2"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("feed without iconUrl, unconfigured = %v, want ErrNoIcon", err)
	}
	e.exec(t, `DELETE FROM feed_icons`)
	if _, err := unconfigured.Icon(context.Background(), "feed/1"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Icon without an account = %v, want ErrUnavailable", err)
	}
}

// A feed unsubscribed while its icon was being fetched keeps no row.
func TestIconOfALeftFeedIsNotKept(t *testing.T) {
	e := newEnv(t)
	e.src.gate = make(chan struct{})
	done := make(chan error)
	go func() { _, err := e.icon(t, "feed/1"); done <- err }()
	for e.src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	e.exec(t, `DELETE FROM feeds WHERE stream_id = 'feed/1'`)
	close(e.src.gate)
	if err := <-done; err != nil {
		t.Fatalf("Icon = %v", err)
	}
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM feed_icons`).Scan(&n); err != nil || n != 0 {
		t.Errorf("icon rows = %d, %v; want none", n, err)
	}
}

func TestConcurrentRequestsShareOneFetch(t *testing.T) {
	e := newEnv(t)
	e.src.gate = make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Icon(context.Background(), "feed/1")
			errs <- err
		}()
	}
	for e.src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(e.src.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Icon = %v", err)
		}
	}
	if n := e.src.calls.Load(); n != 1 {
		t.Errorf("FreshRSS asked %d times, want 1", n)
	}
}
