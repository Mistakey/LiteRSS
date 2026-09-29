package routes

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/library"
)

// ArticleSnapshot answers GET /api/articles?view=unread|all&stream=<id>
// with the view's snapshot: all its ordered item IDs and the newest of them
// (spec D8). view defaults to unread, stream to the reading list.
func ArticleSnapshot(lib *library.Library) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		view := library.View{Stream: q.Get("stream"), UnreadOnly: true}
		if view.Stream == "" {
			view.Stream = freshrss.StreamReadingList
		}
		switch q.Get("view") {
		case "", "unread":
		case "all":
			view.UnreadOnly = false
		default:
			http.Error(w, "view must be unread or all", http.StatusBadRequest)
			return
		}
		s, err := lib.Snapshot(r.Context(), view)
		if err != nil {
			writeLibraryError(w, "snapshot", err)
			return
		}
		writeJSON(w, s)
	})
}

// ArticleCards answers GET /api/articles/cards?ids=<id>,<id>,... with the
// cards of those items in that order, skipping items no longer in the
// library; at most library.MaxCards per request.
func ArticleCards(lib *library.Library) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ids []int64
		if raw := r.URL.Query().Get("ids"); raw != "" {
			parts := strings.Split(raw, ",")
			if len(parts) > library.MaxCards {
				http.Error(w, "too many ids", http.StatusBadRequest)
				return
			}
			for _, s := range parts {
				id, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					http.Error(w, "ids must be item ids separated by commas", http.StatusBadRequest)
					return
				}
				ids = append(ids, id)
			}
		}
		cards, err := lib.Cards(r.Context(), ids)
		if err != nil {
			writeLibraryError(w, "cards", err)
			return
		}
		writeJSON(w, cards)
	})
}

// ArticleContent answers GET /api/articles/{id}/content with {content,
// fulltext}: the RSS body and the cached full text, untrusted HTML the
// frontend sanitizes before showing (spec D16).
func ArticleContent(lib *library.Library) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "id must be an item id", http.StatusBadRequest)
			return
		}
		body, err := lib.Content(r.Context(), id)
		if err != nil {
			writeLibraryError(w, "content", err)
			return
		}
		writeJSON(w, body)
	})
}

// UnreadCounts answers GET /api/unread-counts with the sidebar's live counts.
func UnreadCounts(lib *library.Library) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts, err := lib.UnreadCounts(r.Context())
		if err != nil {
			writeLibraryError(w, "unread counts", err)
			return
		}
		writeJSON(w, counts)
	})
}

// Subscriptions answers GET /api/subscriptions with the subscription tree.
func Subscriptions(lib *library.Library) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tree, err := lib.Tree(r.Context())
		if err != nil {
			writeLibraryError(w, "subscriptions", err)
			return
		}
		writeJSON(w, tree)
	})
}

// writeLibraryError maps a library error to its status; anything but bad
// input or a missing article is logged and answered 500 without details.
func writeLibraryError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, library.ErrBadRequest):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, library.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		log.Printf("API %s: %v", what, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
