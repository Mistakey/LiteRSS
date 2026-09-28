package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"LiteRSS/internal/ai"
	"LiteRSS/internal/config"
	"LiteRSS/internal/freshrss"
	"LiteRSS/internal/summary"
	"LiteRSS/internal/utils/httputil"
)

// ErrInvalid marks a request the panel refuses: a key it does not edit, a
// value of the wrong type or range, a proxy that cannot be installed.
// *UnknownKeysError and *InvalidValueError match it too.
var ErrInvalid = errors.New("invalid settings")

// connectionTestTimeout bounds a connection test.
const connectionTestTimeout = 20 * time.Second

// Panel is what the settings panel does through the API (spec D10): read and
// write the keys it edits, typed as the schema says, and test the FreshRSS
// account and the model before saving them.
type Panel struct {
	store    *Store
	outbound *http.Client
	syncNow  func()

	// Autostart puts startup_on_boot into effect on the system; nil leaves
	// the system alone.
	Autostart func(enabled bool) error
}

// NewPanel returns the panel over store. outbound is the shared outbound
// client (pitfall 8), for the model test; syncNow asks for a sync cycle.
func NewPanel(store *Store, outbound *http.Client, syncNow func()) *Panel {
	return &Panel{store: store, outbound: outbound, syncNow: syncNow}
}

// View is the panel's settings. Settings holds every key the panel edits,
// typed as the schema says; a credential is always "" there and SavedSecrets
// lists the credentials that are set, so a stored credential never leaves
// the backend.
type View struct {
	Settings     map[string]any `json:"settings"`
	SavedSecrets []string       `json:"saved_secrets"`
}

// View returns the settings as they now are.
func (p *Panel) View(ctx context.Context) (View, error) {
	values, err := p.store.Load(ctx)
	if err != nil {
		return View{}, err
	}
	return viewOf(values), nil
}

// Update writes the given keys, each a JSON string, boolean or integer as
// its schema type says, and returns the settings as they then are. Anything
// the panel may not write refuses the whole update with an ErrInvalid error.
// A new proxy applies at once; a changed FreshRSS account or sync interval
// starts a sync. startup_on_boot is put into effect before it is stored: a
// system that refuses it leaves the setting unchanged, and a failed write
// puts the system back.
func (p *Panel) Update(ctx context.Context, in map[string]json.RawMessage) (View, error) {
	update := make(map[string]string, len(in))
	for key, raw := range in {
		v, err := fromJSON(key, raw)
		if err != nil {
			return View{}, err
		}
		update[key] = v
	}
	if raw, ok := update["freshrss_auto_sync_interval"]; ok {
		if n, _ := strconv.Atoi(raw); n < 1 {
			return View{}, fmt.Errorf("%w: freshrss_auto_sync_interval must be at least 1", ErrInvalid)
		}
	}

	values, err := p.store.Load(ctx)
	if err != nil {
		return View{}, err
	}
	wasOn := values["startup_on_boot"] == "true"
	for key, v := range update {
		values[key] = v
	}
	proxyChanged := writesPrefix(update, "proxy_")
	if proxyChanged {
		if _, err := httputil.ProxyForSettings(values); err != nil {
			return View{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	on, autostart := update["startup_on_boot"]
	autostart = autostart && p.Autostart != nil
	if autostart {
		if err := p.Autostart(on == "true"); err != nil {
			return View{}, err
		}
	}
	if err := p.store.Update(ctx, update); err != nil {
		if autostart {
			if undoErr := p.Autostart(wasOn); undoErr != nil {
				log.Printf("settings: undo startup_on_boot: %v", undoErr)
			}
		}
		return View{}, err
	}
	if proxyChanged {
		if err := httputil.ConfigureProxyFromSettings(values); err != nil {
			log.Printf("settings: install proxy: %v", err)
		}
	}
	if writesPrefix(update, "freshrss_") {
		p.syncNow()
	}
	return viewOf(values), nil
}

// fromJSON checks that key is one the panel edits and raw is a JSON value of
// its type, returning the value as stored.
func fromJSON(key string, raw json.RawMessage) (string, error) {
	def, ok := config.Lookup(key)
	if !ok || def.Internal() {
		return "", &UnknownKeysError{Keys: []string{key}}
	}
	var v string
	var err error
	switch def.Type {
	case config.TypeBool:
		var b bool
		err = json.Unmarshal(raw, &b)
		v = strconv.FormatBool(b)
	case config.TypeInt:
		var n int
		err = json.Unmarshal(raw, &n)
		v = strconv.Itoa(n)
	default:
		err = json.Unmarshal(raw, &v)
	}
	if err != nil {
		return "", &InvalidValueError{Key: key, Type: def.Type}
	}
	return v, nil
}

// viewOf turns loaded settings into the panel's view.
func viewOf(values map[string]string) View {
	view := View{Settings: map[string]any{}, SavedSecrets: []string{}}
	for _, key := range config.SettingsKeys() {
		def, _ := config.Lookup(key)
		v := values[key]
		switch {
		case def.Internal():
		case def.Encrypted:
			view.Settings[key] = ""
			if v != "" {
				view.SavedSecrets = append(view.SavedSecrets, key)
			}
		case def.Type == config.TypeBool:
			view.Settings[key] = v == "true"
		case def.Type == config.TypeInt:
			n, _ := strconv.Atoi(v)
			view.Settings[key] = n
		default:
			view.Settings[key] = v
		}
	}
	return view
}

// writesPrefix reports whether update writes a key with prefix.
func writesPrefix(update map[string]string, prefix string) bool {
	for key := range update {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// Test is a connection test's result: whether it worked and a Chinese
// message to show either way.
type Test struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

var testPassed = Test{OK: true, Message: "连接成功。"}

// TestFreshRSS logs in with the form's freshrss_server_url,
// freshrss_username and freshrss_api_password, each defaulting to the
// stored value.
func (p *Panel) TestFreshRSS(ctx context.Context, form map[string]string) (Test, error) {
	v, err := p.formValues(ctx, form, "freshrss_server_url", "freshrss_username", "freshrss_api_password")
	if err != nil {
		return Test{}, err
	}
	server, user, pass := v["freshrss_server_url"], v["freshrss_username"], v["freshrss_api_password"]
	if server == "" || user == "" || pass == "" {
		return Test{Message: "请先填写服务器地址、用户名和 API 密码。"}, nil
	}
	if u, err := url.Parse(server); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Test{Message: "服务器地址要以 http:// 或 https:// 开头。"}, nil
	}
	return runTest(ctx, "FreshRSS", freshrss.NewClient(server, user, pass).Login, freshrssMessage), nil
}

// TestModel sends the model a one-word request with the form's
// llm_endpoint, llm_model and llm_api_key, each defaulting to the stored
// value.
func (p *Panel) TestModel(ctx context.Context, form map[string]string) (Test, error) {
	v, err := p.formValues(ctx, form, "llm_endpoint", "llm_model", "llm_api_key")
	if err != nil {
		return Test{}, err
	}
	model := summary.Model{Endpoint: v["llm_endpoint"], Name: v["llm_model"], APIKey: v["llm_api_key"], HTTP: p.outbound}
	if model.Endpoint == "" || model.Name == "" {
		return Test{Message: "请先填写端点和模型。"}, nil
	}
	return runTest(ctx, "model", model.Check, modelMessage), nil
}

// formValues fills the keys a test form leaves out from the stored settings:
// a stored credential is never sent to the panel, so the panel leaves it
// out. A form key outside keys is ErrInvalid.
func (p *Panel) formValues(ctx context.Context, form map[string]string, keys ...string) (map[string]string, error) {
	values := map[string]string{}
	for key, v := range form {
		values[key] = v
	}
	stored, err := p.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		v, ok := values[key]
		if !ok {
			v = stored[key]
		}
		values[key] = strings.TrimSpace(v)
	}
	if len(values) != len(keys) {
		return nil, fmt.Errorf("%w: a test takes only %s", ErrInvalid, strings.Join(keys, ", "))
	}
	return values, nil
}

// runTest runs check within connectionTestTimeout; a failure is logged and
// told through message.
func runTest(ctx context.Context, what string, check func(context.Context) error, message func(error) string) Test {
	ctx, cancel := context.WithTimeout(ctx, connectionTestTimeout)
	defer cancel()
	if err := check(ctx); err != nil {
		log.Printf("settings: %s connection test: %v", what, err)
		return Test{Message: message(err)}
	}
	return testPassed
}

// freshrssMessage is the Chinese reason for a failed FreshRSS login.
func freshrssMessage(err error) string {
	var apiErr *freshrss.APIError
	var urlErr *url.Error
	switch {
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden):
		return "用户名或 API 密码不对。注意要填 FreshRSS 的 API 密码，不是登录密码。"
	case errors.As(err, &apiErr):
		return fmt.Sprintf("服务器返回 %d，请检查地址是否指向 FreshRSS，以及是否开启了 API 访问。", apiErr.StatusCode)
	case errors.Is(err, context.DeadlineExceeded):
		return "连接 FreshRSS 超时，请检查服务器地址与网络。"
	case errors.As(err, &urlErr):
		return "连不上 FreshRSS 服务器，请检查服务器地址与网络。"
	default:
		return "这个地址不像 FreshRSS 的 API，请检查服务器地址。"
	}
}

// modelMessage is the Chinese reason for a failed model request.
func modelMessage(err error) string {
	switch {
	case errors.Is(err, ai.ErrUnauthorized):
		return "密钥不对，或没有使用这个模型的权限。"
	case errors.Is(err, ai.ErrModelNotFound):
		return "找不到这个模型，请检查模型名与端点地址。"
	case errors.Is(err, context.DeadlineExceeded):
		return "模型长时间没有回应，请检查端点地址与网络。"
	default:
		return "连不上模型，或它没有正常回应，请检查端点地址、模型名与网络。"
	}
}
