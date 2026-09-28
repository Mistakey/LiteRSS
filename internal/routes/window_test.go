package routes

import (
	"net/http"
	"slices"
	"testing"
)

// fakeWindow records what the title bar asked the shell to do.
type fakeWindow struct{ calls []string }

func (w *fakeWindow) Minimise()       { w.calls = append(w.calls, "minimise") }
func (w *fakeWindow) ToggleMaximise() { w.calls = append(w.calls, "maximise") }
func (w *fakeWindow) Close()          { w.calls = append(w.calls, "close") }

func TestWindowButtons(t *testing.T) {
	for _, tc := range []struct{ path, call string }{
		{"/api/window/minimise", "minimise"},
		{"/api/window/maximise", "maximise"},
		{"/api/window/close", "close"},
	} {
		t.Run(tc.call, func(t *testing.T) {
			api := newTestAPI(t)
			if rec := api.send(t, http.MethodPost, tc.path, ""); rec.Code != http.StatusNoContent {
				t.Fatalf("POST %s: %d %s", tc.path, rec.Code, rec.Body)
			}
			if !slices.Equal(api.window.calls, []string{tc.call}) {
				t.Fatalf("POST %s asked the window for %v, want [%s]", tc.path, api.window.calls, tc.call)
			}
		})
	}
}
