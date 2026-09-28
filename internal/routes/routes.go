// Package routes holds the /api the frontend calls (spec D13). Table is the
// one list of routes: main mounts it and routes_test checks its method rule,
// so a route exists exactly when it is in the table.
package routes

import (
	"encoding/json"
	"log"
	"net/http"

	"LiteRSS/internal/enrich"
	"LiteRSS/internal/library"
	"LiteRSS/internal/version"
)

// Route is one entry of the route table.
type Route struct {
	Method string
	Path   string
	// Mutates marks a route that changes state. Such a route takes POST,
	// PUT or DELETE, never GET, so a GET to it is answered 405 (spec D13).
	Mutates bool
	Handler http.Handler
}

// Pattern is the route's http.ServeMux pattern.
func (r Route) Pattern() string {
	return r.Method + " " + r.Path
}

// Deps is what the routes serve from.
type Deps struct {
	Sync    SyncStatus
	SyncNow func()
	Library *library.Library
	Intents Intents
	Enrich  *enrich.Service

	Settings SettingsPanel
	Browser  Browser
	Updates  Updater
	Window   Window
}

// Table lists every /api route.
func Table(d Deps) []Route {
	return []Route{
		{Method: http.MethodGet, Path: "/api/version", Handler: Version()},
		{Method: http.MethodGet, Path: "/api/update/check", Handler: CheckUpdate(d.Updates)},
		{Method: http.MethodPost, Path: "/api/update/start", Mutates: true, Handler: StartUpdate(d.Updates)},
		{Method: http.MethodGet, Path: "/api/update/status", Handler: UpdateStatus(d.Updates)},
		{Method: http.MethodPost, Path: "/api/browser/open", Mutates: true, Handler: OpenInBrowser(d.Browser)},

		{Method: http.MethodPost, Path: "/api/window/minimise", Mutates: true, Handler: MinimiseWindow(d.Window)},
		{Method: http.MethodPost, Path: "/api/window/maximise", Mutates: true, Handler: ToggleMaximiseWindow(d.Window)},
		{Method: http.MethodPost, Path: "/api/window/close", Mutates: true, Handler: CloseWindow(d.Window)},

		{Method: http.MethodGet, Path: "/api/settings", Handler: GetSettings(d.Settings)},
		{Method: http.MethodPost, Path: "/api/settings/update", Mutates: true, Handler: UpdateSettings(d.Settings)},
		{Method: http.MethodPost, Path: "/api/settings/secrets/clear", Mutates: true, Handler: ClearSecret(d.Settings)},
		{Method: http.MethodPost, Path: "/api/settings/freshrss/test", Mutates: true, Handler: TestFreshRSS(d.Settings)},
		{Method: http.MethodPost, Path: "/api/settings/llm/test", Mutates: true, Handler: TestModel(d.Settings)},

		{Method: http.MethodGet, Path: "/api/sync/state", Handler: SyncState(d.Sync, LongPollTimeout)},
		{Method: http.MethodPost, Path: "/api/sync/run", Mutates: true, Handler: SyncRun(d.SyncNow)},

		{Method: http.MethodGet, Path: "/api/articles", Handler: ArticleSnapshot(d.Library)},
		{Method: http.MethodGet, Path: "/api/articles/cards", Handler: ArticleCards(d.Library)},
		{Method: http.MethodGet, Path: "/api/articles/{id}/content", Handler: ArticleContent(d.Library)},
		{Method: http.MethodGet, Path: "/api/unread-counts", Handler: UnreadCounts(d.Library)},
		{Method: http.MethodGet, Path: "/api/subscriptions", Handler: Subscriptions(d.Library)},

		{Method: http.MethodPost, Path: "/api/articles/{id}/read", Mutates: true, Handler: SetArticleRead(d.Intents)},
		{Method: http.MethodPost, Path: "/api/articles/read", Mutates: true, Handler: MarkArticlesRead(d.Intents)},
		{Method: http.MethodPost, Path: "/api/streams/read", Mutates: true, Handler: MarkStreamRead(d.Intents)},
		{Method: http.MethodPost, Path: "/api/undo", Mutates: true, Handler: Undo(d.Intents)},

		{Method: http.MethodPost, Path: "/api/articles/{id}/fulltext", Mutates: true, Handler: FetchFullText(d.Enrich)},
		{Method: http.MethodPost, Path: "/api/articles/translate-titles", Mutates: true, Handler: TranslateTitles(d.Enrich)},
		{Method: http.MethodPost, Path: "/api/articles/{id}/summary", Mutates: true, Handler: Summarize(d.Enrich)},
	}
}

// Handler serves the route table.
func Handler(d Deps) http.Handler {
	return Mount(Table(d))
}

// Mount serves the given routes.
func Mount(table []Route) *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range table {
		mux.Handle(r.Pattern(), r.Handler)
	}
	return mux
}

// Version answers GET /api/version.
func Version() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]string{"version": version.Version})
	})
}

// writeJSON sends v as the response. API answers are live state, never
// cached.
func writeJSON(w http.ResponseWriter, v any) {
	writeJSONStatus(w, http.StatusOK, v)
}

// writeJSONStatus is writeJSON with another status code.
func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("API: write response: %v", err)
	}
}
