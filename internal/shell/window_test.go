package shell

import (
	"context"
	"path/filepath"
	"testing"

	"LiteRSS/internal/database"
	"LiteRSS/internal/settings"
)

func openStore(t *testing.T) *settings.Store {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "literss.db"))
	if err != nil {
		t.Fatalf("database.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return settings.New(db.DB)
}

func TestWindowNeverSavedOpensCentered(t *testing.T) {
	w, placed, err := LoadWindow(context.Background(), openStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if placed || w.Width != 1024 || w.Height != 768 || w.Maximized {
		t.Errorf("LoadWindow = %+v, placed %v; want default size, not placed", w, placed)
	}
}

func TestWindowRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	saved := Window{X: -1900, Y: 40, Width: 1200, Height: 900}
	if err := SaveWindow(ctx, store, saved, false); err != nil {
		t.Fatal(err)
	}
	w, placed, err := LoadWindow(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if !placed || w != saved {
		t.Errorf("LoadWindow = %+v, placed %v; want %+v placed", w, placed, saved)
	}
}

// The minimized placeholder position is neither saved nor applied
// (spec D4, pitfall 33).
func TestWindowSentinelFiltered(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	normal := Window{X: 100, Y: 80, Width: 1100, Height: 800}
	if err := SaveWindow(ctx, store, normal, false); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		w         Window
		minimized bool
	}{
		{"minimized flag", Window{X: 100, Y: 80, Width: 160, Height: 28}, true},
		{"placeholder at 100%", Window{X: -32000, Y: -32000, Width: 1100, Height: 800}, false},
		{"placeholder at 150%", Window{X: -21333, Y: -21333, Width: 1100, Height: 800}, false},
	} {
		if err := SaveWindow(ctx, store, tc.w, tc.minimized); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if w, _, _ := LoadWindow(ctx, store); w != normal {
			t.Errorf("%s overwrote the placement: %+v, want %+v", tc.name, w, normal)
		}
	}

	// A placeholder already in the library (an older build) is not applied.
	if err := store.Update(ctx, map[string]string{"window_x": "-32000", "window_y": "-32000"}); err != nil {
		t.Fatal(err)
	}
	if _, placed, _ := LoadWindow(ctx, store); placed {
		t.Error("stored placeholder position was applied")
	}
}

func TestWindowMaximizedKeepsNormalBounds(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	normal := Window{X: 100, Y: 80, Width: 1100, Height: 800}
	if err := SaveWindow(ctx, store, normal, false); err != nil {
		t.Fatal(err)
	}
	if err := SaveWindow(ctx, store, Window{X: -8, Y: -8, Width: 2576, Height: 1416, Maximized: true}, false); err != nil {
		t.Fatal(err)
	}
	w, placed, _ := LoadWindow(ctx, store)
	want := normal
	want.Maximized = true
	if !placed || w != want {
		t.Errorf("LoadWindow = %+v, placed %v; want %+v placed", w, placed, want)
	}
}

func TestWindowTooSmallIgnored(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	if err := SaveWindow(ctx, store, Window{X: 10, Y: 10, Width: 200, Height: 100}, false); err != nil {
		t.Fatal(err)
	}
	if _, placed, _ := LoadWindow(ctx, store); placed {
		t.Error("a too-small window was saved and applied")
	}
}
