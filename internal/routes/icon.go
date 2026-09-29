package routes

import (
	"errors"
	"log"
	"net/http"

	"LiteRSS/internal/feedicon"
)

// FeedIcon answers GET /api/feeds/icon?id=<stream id> with the feed's icon
// from FreshRSS's favicon cache (spec D15), or 204 when there is none to
// show, for whatever reason: the image fails to decode and the frontend shows
// the letter badge. Not a 404, which the browser would log as a failed load
// for every feed without an icon on every start (docs/TESTING.md wants a
// quiet console). The frontend adds the feed's iconUrl as v, so an icon may
// be cached for a day and a changed iconUrl is a new address.
func FeedIcon(icons *feedicon.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		icon, err := icons.Icon(r.Context(), id)
		switch {
		case errors.Is(err, feedicon.ErrNoIcon), errors.Is(err, feedicon.ErrUnavailable):
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		case err != nil:
			if r.Context().Err() == nil {
				log.Printf("API feed icon: %v", err)
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", icon.ContentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		_, _ = w.Write(icon.Data)
	})
}
