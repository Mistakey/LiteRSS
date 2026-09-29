package routes

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"LiteRSS/internal/settings"
)

// SettingsPanel is what the settings panel does; *settings.Panel implements
// it. Its refusals match settings.ErrInvalid.
type SettingsPanel interface {
	View(ctx context.Context) (settings.View, error)
	Update(ctx context.Context, in map[string]json.RawMessage) (settings.View, error)
	ClearSecret(ctx context.Context, key string) (settings.View, error)
	TestFreshRSS(ctx context.Context, form map[string]string) (settings.Test, error)
	TestModel(ctx context.Context, form map[string]string) (settings.Test, error)
}

// GetSettings answers GET /api/settings with {settings, saved_secrets}: the
// keys the panel edits, typed, with every credential "" and those set listed
// in saved_secrets.
func GetSettings(panel SettingsPanel) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view, err := panel.View(r.Context())
		if err != nil {
			writeSettingsError(w, "load settings", err)
			return
		}
		writeJSON(w, view)
	})
}

// UpdateSettings answers POST /api/settings/update with {key: value, ...},
// the keys to change, by writing them and answering the settings as they now
// are. Anything the panel may not write refuses the whole write with 400
// (spec D10).
func UpdateSettings(panel SettingsPanel) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if !decodeBody(w, r, &body) {
			return
		}
		view, err := panel.Update(r.Context(), body)
		if err != nil {
			writeSettingsError(w, "update settings", err)
			return
		}
		writeJSON(w, view)
	})
}

// ClearSecret answers POST /api/settings/secrets/clear with {key}, a
// credential to delete, by deleting it and answering the settings as they
// now are. An update refuses an empty credential, so clearing is only this.
func ClearSecret(panel SettingsPanel) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Key string `json:"key"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		view, err := panel.ClearSecret(r.Context(), body.Key)
		if err != nil {
			writeSettingsError(w, "clear secret", err)
			return
		}
		writeJSON(w, view)
	})
}

// TestFreshRSS answers POST /api/settings/freshrss/test with {ok, message}
// by logging in with {freshrss_server_url, freshrss_username,
// freshrss_api_password}, each defaulting to the stored value.
func TestFreshRSS(panel SettingsPanel) http.Handler {
	return connectionTest(func(ctx context.Context, form map[string]string) (settings.Test, error) {
		return panel.TestFreshRSS(ctx, form)
	})
}

// TestModel answers POST /api/settings/llm/test with {ok, message} by
// sending the model a one-word request with {llm_endpoint, llm_model,
// llm_api_key}, each defaulting to the stored value.
func TestModel(panel SettingsPanel) http.Handler {
	return connectionTest(func(ctx context.Context, form map[string]string) (settings.Test, error) {
		return panel.TestModel(ctx, form)
	})
}

func connectionTest(test func(context.Context, map[string]string) (settings.Test, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var form map[string]string
		if !decodeBody(w, r, &form) {
			return
		}
		result, err := test(r.Context(), form)
		if err != nil {
			writeSettingsError(w, "connection test", err)
			return
		}
		writeJSON(w, result)
	})
}

// writeSettingsError answers a refusal 400 and anything else 500.
func writeSettingsError(w http.ResponseWriter, what string, err error) {
	if errors.Is(err, settings.ErrInvalid) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("API %s: %v", what, err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
