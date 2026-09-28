package routes

import (
	"net/http"
	"strconv"

	"LiteRSS/internal/enrich"
)

// FetchFullText answers POST /api/articles/{id}/fulltext with the article's
// full text {outcome, content, message}: the cached one or a fresh fetch. A
// failed fetch is still 200, its outcome saying which failure and message
// the Chinese reason to show (spec D11).
func FetchFullText(svc *enrich.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		ft, err := svc.FullText(r.Context(), id)
		if err != nil {
			writeLibraryError(w, "full text", err)
			return
		}
		writeJSON(w, ft)
	})
}

// TranslateTitles answers POST /api/articles/translate-titles with {"ids"},
// at most library.MaxCards, by deciding those titles:
// {titles: [{id, translated_title}], message}. Titles left undecided are
// absent and message says why in Chinese.
func TranslateTitles(svc *enrich.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IDs []int64 `json:"ids"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.IDs == nil {
			http.Error(w, "ids is required", http.StatusBadRequest)
			return
		}
		titles, err := svc.TranslateTitles(r.Context(), body.IDs)
		if err != nil {
			writeLibraryError(w, "translate titles", err)
			return
		}
		writeJSON(w, titles)
	})
}

// Summarize answers POST /api/articles/{id}/summary with {html, note}: the
// rendered Chinese summary, stored or new, and a Chinese note on what it is
// based on; html is empty when no summary was made and note says why
// (spec D11).
func Summarize(svc *enrich.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		s, err := svc.Summarize(r.Context(), id)
		if err != nil {
			writeLibraryError(w, "summary", err)
			return
		}
		writeJSON(w, s)
	})
}

// pathID reads the {id} path value; on failure it answers 400.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id must be an item id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
