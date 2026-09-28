package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"LiteRSS/internal/syncer"
)

// Intents is the part of the sync service the read actions write through;
// *syncer.Service implements it. Every action only records the user's
// intent, which the pusher sends to FreshRSS (spec D6).
type Intents interface {
	SetRead(ctx context.Context, itemID int64, read bool) error
	MarkItemsRead(ctx context.Context, ids []int64) (syncer.Batch, error)
	MarkStreamRead(ctx context.Context, stream string, ts int64) (syncer.Batch, error)
	Undo(ctx context.Context, token string) error
}

// maxReadBody bounds a read action's request body; a whole view's ids fit.
const maxReadBody = 4 << 20

// SetArticleRead answers POST /api/articles/{id}/read with {"read": bool}
// by marking the article and its URL group read or unread, 204. A single
// article is not undone through a token (spec D7).
func SetArticleRead(intents Intents) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "id must be an item id", http.StatusBadRequest)
			return
		}
		var body struct {
			Read *bool `json:"read"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.Read == nil {
			http.Error(w, "read is required", http.StatusBadRequest)
			return
		}
		if err := intents.SetRead(r.Context(), id, *body.Read); err != nil {
			writeIntentError(w, "set read", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// MarkArticlesRead answers POST /api/articles/read with {"ids": [...]}, the
// range of the view's snapshot for "this and above / below" (spec D7), by
// marking those shown unread read; it returns the batch's undo token.
func MarkArticlesRead(intents Intents) http.Handler {
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
		b, err := intents.MarkItemsRead(r.Context(), body.IDs)
		if err != nil {
			writeIntentError(w, "mark items read", err)
			return
		}
		writeBatch(w, b)
	})
}

// MarkStreamRead answers POST /api/streams/read with {"stream", "ts"}: a
// feed, a label with its feeds, or the reading list is marked read up to ts,
// the snapshot's newest for the current view; without ts, the stream's
// newest local item is taken (spec D7). It returns the batch's undo token.
func MarkStreamRead(intents Intents) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream string `json:"stream"`
			TS     int64  `json:"ts"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if !syncer.ValidStream(body.Stream) {
			http.Error(w, "stream must be a feed, a label or the reading list", http.StatusBadRequest)
			return
		}
		if body.TS < 0 {
			http.Error(w, "ts must be an item id", http.StatusBadRequest)
			return
		}
		b, err := intents.MarkStreamRead(r.Context(), body.Stream, body.TS)
		if err != nil {
			writeIntentError(w, "mark stream read", err)
			return
		}
		writeBatch(w, b)
	})
}

// Undo answers POST /api/undo with {"token"} by reverting that batch, 204;
// a token unknown, used or past syncer.UndoTTL is answered 410.
func Undo(intents Intents) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		if body.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		if err := intents.Undo(r.Context(), body.Token); err != nil {
			if errors.Is(err, syncer.ErrUndoExpired) {
				http.Error(w, "undo expired", http.StatusGone)
				return
			}
			writeIntentError(w, "undo", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// decodeBody reads a JSON body into v, rejecting unknown fields; on failure
// it answers 400 and returns false.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReadBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		http.Error(w, fmt.Sprintf("bad request body: %v", err), http.StatusBadRequest)
		return false
	}
	return true
}

func writeBatch(w http.ResponseWriter, b syncer.Batch) {
	writeJSON(w, map[string]any{"token": b.Token, "count": b.Count})
}

// writeIntentError logs an intent write that failed and answers 500
// without details.
func writeIntentError(w http.ResponseWriter, what string, err error) {
	log.Printf("API %s: %v", what, err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
