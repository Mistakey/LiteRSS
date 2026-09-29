package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"LiteRSS/internal/freshrss/freshrsstest"
	"LiteRSS/internal/settings"
	"LiteRSS/internal/utils/httputil"
)

func (a *testAPI) postJSON(t *testing.T, target, body string, v any) {
	t.Helper()
	rec := a.send(t, http.MethodPost, target, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s: %d %s", target, rec.Code, rec.Body)
	}
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
}

func (a *testAPI) stored(t *testing.T) map[string]string {
	t.Helper()
	values, err := a.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestSettingsHideCredentialsAndInternalKeys(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.Update(context.Background(), map[string]string{
		"freshrss_api_password": "secret", "freshrss_username": "kelch", "window_x": "10",
	}); err != nil {
		t.Fatal(err)
	}

	var got settings.View
	api.getJSON(t, "/api/settings", &got)
	s := got.Settings
	if s["freshrss_username"] != "kelch" || s["freshrss_api_password"] != "" || s["llm_api_key"] != "" {
		t.Fatalf("settings = %v", s)
	}
	if s["close_to_tray"] != true || s["freshrss_auto_sync_interval"] != float64(30) {
		t.Fatalf("typed defaults = %v, %v", s["close_to_tray"], s["freshrss_auto_sync_interval"])
	}
	if _, ok := s["window_x"]; ok {
		t.Fatal("an internal key is shown")
	}
	if !slices.Equal(got.SavedSecrets, []string{"freshrss_api_password"}) {
		t.Fatalf("saved_secrets = %v", got.SavedSecrets)
	}
}

func TestUpdateSettingsWritesTypedValues(t *testing.T) {
	api := newTestAPI(t)
	var got settings.View
	api.postJSON(t, "/api/settings/update",
		`{"close_to_tray": false, "freshrss_auto_sync_interval": 15, "llm_api_key": "k", "llm_model": "m"}`, &got)

	v := api.stored(t)
	if v["close_to_tray"] != "false" || v["freshrss_auto_sync_interval"] != "15" || v["llm_api_key"] != "k" || v["llm_model"] != "m" {
		t.Fatalf("stored = %v", v)
	}
	if got.Settings["close_to_tray"] != false || got.Settings["llm_api_key"] != "" || !slices.Equal(got.SavedSecrets, []string{"llm_api_key"}) {
		t.Fatalf("answer = %+v", got)
	}
	if api.triggered != 1 {
		t.Fatalf("a changed sync interval triggered %d syncs, want 1", api.triggered)
	}
	api.postJSON(t, "/api/settings/update", `{"llm_model": "n"}`, &got)
	if api.triggered != 1 {
		t.Fatal("a model change started a sync")
	}
}

// The reader group (Bionic Reading, spec D22) is written by the article view,
// not the panel, but goes through the same routes.
func TestReaderSettingsReadAndWrite(t *testing.T) {
	api := newTestAPI(t)
	var got settings.View
	api.getJSON(t, "/api/settings", &got)
	if got.Settings["bionic_reading"] != false {
		t.Fatalf("default bionic_reading = %v", got.Settings["bionic_reading"])
	}
	api.postJSON(t, "/api/settings/update", `{"bionic_reading": true}`, &got)
	if api.stored(t)["bionic_reading"] != "true" || got.Settings["bionic_reading"] != true {
		t.Fatalf("stored = %v, answer = %v", api.stored(t)["bionic_reading"], got.Settings["bionic_reading"])
	}
	if api.triggered != 0 {
		t.Fatal("a reader setting started a sync")
	}
}

// TestUpdateSettingsRejectsUnknownKeys refuses the whole write for a key
// outside the panel's list, a value of the wrong type or an invalid proxy
// (spec D10).
func TestUpdateSettingsRejectsUnknownKeys(t *testing.T) {
	api := newTestAPI(t)
	for _, body := range []string{
		`{"llm_model": "m", "ai_model": "gpt"}`,
		`{"llm_model": "m", "window_x": 5}`,
		`{"llm_model": "m", "close_to_tray": "false"}`,
		`{"llm_model": "m", "freshrss_auto_sync_interval": 1.5}`,
		`{"llm_model": "m", "freshrss_auto_sync_interval": 0}`,
		`{"llm_model": 3}`,
		`{"llm_model": "m", "proxy_mode": "manual", "proxy_host": ""}`,
		`{"llm_model": "m", "llm_api_key": ""}`,
		`["llm_model"]`,
	} {
		if rec := api.send(t, http.MethodPost, "/api/settings/update", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
	}
	if v := api.stored(t); v["llm_model"] != "" || v["proxy_mode"] != "system" {
		t.Fatalf("a refused write was stored: %v", v)
	}
}

// TestClearSecret deletes one stored credential: an empty string in an
// update is refused, so keeping and clearing never look alike.
func TestClearSecret(t *testing.T) {
	api := newTestAPI(t)
	if err := api.store.Update(context.Background(), map[string]string{
		"llm_api_key": "k", "freshrss_api_password": "p", "llm_model": "m",
	}); err != nil {
		t.Fatal(err)
	}

	var got settings.View
	api.postJSON(t, "/api/settings/secrets/clear", `{"key": "llm_api_key"}`, &got)
	if v := api.stored(t); v["llm_api_key"] != "" || v["freshrss_api_password"] != "p" || v["llm_model"] != "m" {
		t.Fatalf("stored = %v", v)
	}
	if !slices.Equal(got.SavedSecrets, []string{"freshrss_api_password"}) || got.Settings["llm_api_key"] != "" {
		t.Fatalf("answer = %+v", got)
	}
	if api.triggered != 0 {
		t.Fatal("clearing the model key started a sync")
	}
	api.postJSON(t, "/api/settings/secrets/clear", `{"key": "freshrss_api_password"}`, &got)
	if len(got.SavedSecrets) != 0 || api.triggered != 1 {
		t.Fatalf("answer = %+v, %d syncs", got, api.triggered)
	}

	for _, body := range []string{
		`{"key": "llm_model"}`,
		`{"key": "ai_api_key"}`,
		`{"key": "window_x"}`,
		`{"key": "llm_api_key", "also": 1}`,
		`{}`,
	} {
		if rec := api.send(t, http.MethodPost, "/api/settings/secrets/clear", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
	}
	if v := api.stored(t); v["llm_model"] != "m" {
		t.Fatalf("a refused clear changed %v", v)
	}
}

func TestUpdateSettingsAppliesStartupOnBoot(t *testing.T) {
	api := newTestAPI(t)
	var got settings.View
	api.postJSON(t, "/api/settings/update", `{"startup_on_boot": true}`, &got)
	if !slices.Equal(api.autostart, []bool{true}) || api.stored(t)["startup_on_boot"] != "true" {
		t.Fatalf("enable: system saw %v, stored %q", api.autostart, api.stored(t)["startup_on_boot"])
	}
	api.postJSON(t, "/api/settings/update", `{"llm_model": "m"}`, &got)
	if len(api.autostart) != 1 {
		t.Fatal("an unrelated write touched the system's login items")
	}

	// A system that refuses leaves the setting as it was.
	api.autostartErr = errors.New("access denied")
	if rec := api.send(t, http.MethodPost, "/api/settings/update", `{"startup_on_boot": false}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("refused by the system: %d, want 500", rec.Code)
	}
	if api.stored(t)["startup_on_boot"] != "true" {
		t.Fatal("the setting changed although the system refused")
	}
}

func TestUpdateSettingsInstallsTheProxy(t *testing.T) {
	t.Cleanup(func() { _ = httputil.ConfigureProxy(httputil.ProxyModeSystem, "") })
	api := newTestAPI(t)
	var got settings.View
	api.postJSON(t, "/api/settings/update", `{"proxy_mode": "manual", "proxy_port": "7897"}`, &got)

	transport := httputil.CreateHTTPClient(time.Second).Transport.(*http.Transport)
	proxy, err := transport.Proxy(httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	if err != nil || proxy == nil || proxy.Host != "127.0.0.1:7897" {
		t.Fatalf("proxy = %v, %v; want the stored host with the new port", proxy, err)
	}
}

func TestFreshRSSConnectionTest(t *testing.T) {
	api := newTestAPI(t)
	fake := httptest.NewServer(freshrsstest.New("user", "secret"))
	t.Cleanup(fake.Close)

	var got settings.Test
	api.postJSON(t, "/api/settings/freshrss/test", `{}`, &got)
	if got.OK || got.Message != "请先填写服务器地址、用户名和 API 密码。" {
		t.Fatalf("nothing configured: %+v", got)
	}

	// The stored password is used when the panel leaves it out.
	if err := api.store.Update(context.Background(), map[string]string{"freshrss_api_password": "secret"}); err != nil {
		t.Fatal(err)
	}
	api.postJSON(t, "/api/settings/freshrss/test",
		`{"freshrss_server_url": "`+fake.URL+`", "freshrss_username": "user"}`, &got)
	if !got.OK {
		t.Fatalf("right account: %+v", got)
	}

	api.postJSON(t, "/api/settings/freshrss/test",
		`{"freshrss_server_url": "`+fake.URL+`", "freshrss_username": "user", "freshrss_api_password": "wrong"}`, &got)
	if got.OK || !strings.Contains(got.Message, "API 密码不对") {
		t.Fatalf("wrong password: %+v", got)
	}
	api.postJSON(t, "/api/settings/freshrss/test",
		`{"freshrss_server_url": "ftp://x", "freshrss_username": "u", "freshrss_api_password": "p"}`, &got)
	if got.OK || got.Message != "服务器地址要以 http:// 或 https:// 开头。" {
		t.Fatalf("ftp address: %+v", got)
	}
	if rec := api.send(t, http.MethodPost, "/api/settings/freshrss/test", `{"llm_model": "m"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a field of another test: %d, want 400", rec.Code)
	}
}

func TestModelConnectionTest(t *testing.T) {
	api := newTestAPI(t)
	var keys []string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "OK"}}}})
	}))
	t.Cleanup(model.Close)
	if err := api.store.Update(context.Background(), map[string]string{"llm_api_key": "good"}); err != nil {
		t.Fatal(err)
	}

	var got settings.Test
	api.postJSON(t, "/api/settings/llm/test", `{}`, &got)
	if got.OK || got.Message != "请先填写端点和模型。" {
		t.Fatalf("nothing configured: %+v", got)
	}
	endpoint := `"llm_endpoint": "` + model.URL + `/v1/chat/completions", "llm_model": "m"`
	api.postJSON(t, "/api/settings/llm/test", `{`+endpoint+`}`, &got)
	if !got.OK || got.Message != "连接成功。" {
		t.Fatalf("stored key: %+v", got)
	}
	api.postJSON(t, "/api/settings/llm/test", `{`+endpoint+`, "llm_api_key": "bad"}`, &got)
	if got.OK || got.Message != "密钥不对，或没有使用这个模型的权限。" {
		t.Fatalf("wrong key: %+v", got)
	}
	if !slices.Contains(keys, "Bearer good") || !slices.Contains(keys, "Bearer bad") {
		t.Fatalf("keys sent = %v", keys)
	}
}
