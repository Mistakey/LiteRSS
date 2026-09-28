// Package shell holds what the desktop shell remembers and changes outside
// the window (spec D4): the saved window placement and launch at login. The
// Wails wiring that calls it lives in main.
package shell

import (
	"context"
	"strconv"

	"LiteRSS/internal/config"
	"LiteRSS/internal/settings"
)

// Smallest saved size that is still applied; anything smaller is a window
// caught mid-transition, not one the user sized.
const (
	minWindowWidth  = 400
	minWindowHeight = 300
)

// Window is the main window's placement: the normal (restored) bounds and
// whether it was maximized over them.
type Window struct {
	X, Y          int
	Width, Height int
	Maximized     bool
}

// LoadWindow reads the saved placement. placed is false when the saved
// bounds must not be applied: never saved (the schema default sits at 0,0),
// the minimized placeholder (pitfall 33) or a size too small to be real; the
// window then opens centered at the returned size.
func LoadWindow(ctx context.Context, store *settings.Store) (w Window, placed bool, err error) {
	values, err := store.Load(ctx)
	if err != nil {
		return Window{}, false, err
	}
	w.X, _ = strconv.Atoi(values["window_x"])
	w.Y, _ = strconv.Atoi(values["window_y"])
	w.Width, _ = strconv.Atoi(values["window_width"])
	w.Height, _ = strconv.Atoi(values["window_height"])
	w.Maximized = values["window_maximized"] == "true"
	if w.Width < minWindowWidth || w.Height < minWindowHeight {
		w.Width, _ = strconv.Atoi(config.GetString("window_width"))
		w.Height, _ = strconv.Atoi(config.GetString("window_height"))
		return w, false, nil
	}
	placed = !(w.X == 0 && w.Y == 0) && !settings.MinimizedWindowPos(w.X, w.Y)
	return w, placed, nil
}

// SaveWindow stores the placement read from the window as it is closed or
// hidden. A minimized window reports the placeholder position and a
// title-bar size, so nothing is saved; a maximized one reports the screen,
// so only the flag is saved and the last normal bounds stay.
func SaveWindow(ctx context.Context, store *settings.Store, w Window, minimized bool) error {
	values := windowValues(w, minimized)
	if len(values) == 0 {
		return nil
	}
	return store.Update(ctx, values)
}

func windowValues(w Window, minimized bool) map[string]string {
	if minimized || settings.MinimizedWindowPos(w.X, w.Y) {
		return nil
	}
	if w.Maximized {
		return map[string]string{"window_maximized": "true"}
	}
	if w.Width < minWindowWidth || w.Height < minWindowHeight {
		return nil
	}
	return map[string]string{
		"window_x":         strconv.Itoa(w.X),
		"window_y":         strconv.Itoa(w.Y),
		"window_width":     strconv.Itoa(w.Width),
		"window_height":    strconv.Itoa(w.Height),
		"window_maximized": "false",
	}
}
