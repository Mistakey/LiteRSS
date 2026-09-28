package httputil

import (
	"net/http"
	"testing"
	"time"
)

func mustRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("NewRequest(%q): %v", rawURL, err)
	}
	return req
}

func TestProxyForManualUsesTheConfiguredAddress(t *testing.T) {
	resolve, err := ProxyFor(ProxyModeManual, "http://127.0.0.1:7897")
	if err != nil {
		t.Fatalf("ProxyFor returned error: %v", err)
	}

	got, err := resolve(mustRequest(t, "https://example.com/article"))
	if err != nil {
		t.Fatalf("resolver returned error: %v", err)
	}
	if got == nil {
		t.Fatal("manual mode resolved to no proxy")
	}
	if got.String() != "http://127.0.0.1:7897" {
		t.Fatalf("got proxy %q, want %q", got.String(), "http://127.0.0.1:7897")
	}
}

func TestProxyForDirectIgnoresTheEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")

	resolve, err := ProxyFor(ProxyModeDirect, "http://127.0.0.1:1080")
	if err != nil {
		t.Fatalf("ProxyFor returned error: %v", err)
	}

	got, err := resolve(mustRequest(t, "https://example.com/article"))
	if err != nil {
		t.Fatalf("resolver returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("direct mode resolved to proxy %q, want none", got.String())
	}
}

func TestCreateHTTPClientAlwaysResolvesAProxy(t *testing.T) {
	client := CreateHTTPClient(time.Second)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", client.Transport)
	}
	// A nil Proxy means "never use a proxy" and skips the environment too.
	// Unconfigured clients must still follow the system instead.
	if transport.Proxy == nil {
		t.Fatal("Transport.Proxy is nil, so every request would bypass the proxy")
	}
}

func TestCreateHTTPClientFollowsTheConfiguredMode(t *testing.T) {
	t.Cleanup(func() { _ = ConfigureProxy(ProxyModeSystem, "") })

	if err := ConfigureProxy(ProxyModeManual, "http://127.0.0.1:7897"); err != nil {
		t.Fatalf("ConfigureProxy returned error: %v", err)
	}

	transport := CreateHTTPClient(time.Second).Transport.(*http.Transport)
	got, err := transport.Proxy(mustRequest(t, "https://example.com/article"))
	if err != nil {
		t.Fatalf("resolver returned error: %v", err)
	}
	if got == nil || got.String() != "http://127.0.0.1:7897" {
		t.Fatalf("got proxy %v, want http://127.0.0.1:7897", got)
	}
}

func TestConfigureProxyRejectsManualWithoutAnAddress(t *testing.T) {
	t.Cleanup(func() { _ = ConfigureProxy(ProxyModeSystem, "") })

	if err := ConfigureProxy(ProxyModeManual, ""); err == nil {
		t.Fatal("ConfigureProxy accepted manual mode with no address")
	}
}

func TestConfigureProxyFromSettingsBuildsTheManualAddress(t *testing.T) {
	t.Cleanup(func() { _ = ConfigureProxy(ProxyModeSystem, "") })

	settings := map[string]string{
		"proxy_mode":     ProxyModeManual,
		"proxy_type":     "http",
		"proxy_host":     "127.0.0.1",
		"proxy_port":     "7897",
		"proxy_username": "kelch",
		"proxy_password": "p@ss:word",
	}

	if err := ConfigureProxyFromSettings(settings); err != nil {
		t.Fatalf("ConfigureProxyFromSettings returned error: %v", err)
	}

	transport := CreateHTTPClient(time.Second).Transport.(*http.Transport)
	got, err := transport.Proxy(mustRequest(t, "https://example.com/article"))
	if err != nil {
		t.Fatalf("resolver returned error: %v", err)
	}
	if got == nil || got.Host != "127.0.0.1:7897" || got.User.Username() != "kelch" {
		t.Fatalf("got proxy %v, want kelch at 127.0.0.1:7897", got)
	}
	if pass, _ := got.User.Password(); pass != "p@ss:word" {
		t.Fatalf("proxy password %q, want p@ss:word", pass)
	}
}

func TestConfigureProxyFromSettingsKeepsTheResolverOnError(t *testing.T) {
	t.Cleanup(func() { _ = ConfigureProxy(ProxyModeSystem, "") })
	if err := ConfigureProxy(ProxyModeManual, "http://127.0.0.1:7897"); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureProxyFromSettings(map[string]string{"proxy_mode": ProxyModeManual, "proxy_host": ""}); err == nil {
		t.Fatal("manual mode without a host was accepted")
	}
	transport := CreateHTTPClient(time.Second).Transport.(*http.Transport)
	if got, _ := transport.Proxy(mustRequest(t, "https://example.com/")); got == nil || got.Host != "127.0.0.1:7897" {
		t.Fatalf("resolver changed to %v after a refused configuration", got)
	}
}
