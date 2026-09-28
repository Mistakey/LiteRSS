package routes

import (
	"context"
	"errors"
	"log"
	"net/http"

	"LiteRSS/internal/browser"
	"LiteRSS/internal/update"
)

// Browser opens a link in the system browser; browser.Opener implements
// it, refusing anything but an absolute http(s) URL with
// browser.ErrUnsupportedURL.
type Browser interface {
	Open(raw string) error
}

// Updater checks for a newer release and installs it (spec D20);
// *update.Updater implements it.
type Updater interface {
	Check(ctx context.Context) update.Result
	Start(fromTray bool) update.Status
	Status() update.Status
}

// OpenInBrowser answers POST /api/browser/open with {"url"} by opening it in
// the system browser, 204. The frontend has no other way out of the window
// (spec D4); a link that is not absolute http(s) is refused with 400.
func OpenInBrowser(b Browser) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if err := b.Open(body.URL); err != nil {
			if errors.Is(err, browser.ErrUnsupportedURL) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			log.Printf("API open in browser: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// CheckUpdate answers GET /api/update/check with {current_version,
// latest_version, update_available, release_url, in_app, message}; a failed
// check is still 200, message saying why in Chinese. in_app says 更新 installs
// from the app; otherwise the user goes to release_url. It checks whatever
// update_check_enabled says: the setting only governs the automatic check.
func CheckUpdate(u Updater) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, u.Check(r.Context()))
	})
}

// StartUpdate answers POST /api/update/start by starting the in-app update,
// or leaving the one under way, and 202 with its status.
func StartUpdate(u Updater) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, http.StatusAccepted, u.Start(false))
	})
}

// UpdateStatus answers GET /api/update/status with {state, version, received,
// total, message, release_url}: the panel polls it while an update runs.
func UpdateStatus(u Updater) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, u.Status())
	})
}
