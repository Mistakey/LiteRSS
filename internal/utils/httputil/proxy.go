package httputil

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
)

// Proxy modes. They mirror the proxy_mode setting and decide where an outbound
// request looks for its proxy.
const (
	// ProxyModeSystem follows the operating system's proxy settings.
	ProxyModeSystem = "system"
	// ProxyModeDirect bypasses every proxy, including the environment variables.
	ProxyModeDirect = "direct"
	// ProxyModeManual uses the address configured in the app's own settings.
	ProxyModeManual = "manual"
)

// ProxyResolver reports which proxy to use for a request. Its shape matches
// http.Transport.Proxy: a nil URL with a nil error means "connect directly".
type ProxyResolver func(*http.Request) (*url.URL, error)

// ProxyFor builds the resolver a proxy mode describes. manualURL is only read in
// ProxyModeManual; an unrecognised mode is treated as ProxyModeSystem, so a
// missing or corrupt setting still follows the system rather than silently
// disabling the proxy.
func ProxyFor(mode, manualURL string) (ProxyResolver, error) {
	switch mode {
	case ProxyModeDirect:
		return noProxy, nil
	case ProxyModeManual:
		if manualURL == "" {
			return nil, fmt.Errorf("proxy mode %q needs a host and port", ProxyModeManual)
		}
		parsed, err := url.Parse(manualURL)
		if err != nil {
			// *url.Error quotes the address, password included; keep only why.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		return http.ProxyURL(parsed), nil
	default:
		return SystemProxy(), nil
	}
}

func noProxy(*http.Request) (*url.URL, error) { return nil, nil }

// installedResolver is the app-wide proxy behaviour every client built by
// CreateHTTPClient follows. Proxy mode is one setting for the whole
// application, and the clients that need it are constructed in places that have
// no access to the settings store, so it is installed once at startup and
// re-installed whenever the settings change.
var installedResolver atomic.Pointer[ProxyResolver]

// ConfigureProxy installs the app-wide proxy behaviour. Clients created before
// the call pick up the new behaviour too: they read the resolver per request.
func ConfigureProxy(mode, manualURL string) error {
	resolve, err := ProxyFor(mode, manualURL)
	if err != nil {
		return err
	}
	installedResolver.Store(&resolve)
	return nil
}

// resolveProxy defers to whatever ConfigureProxy last installed, falling back to
// the system settings while the application is still starting up.
func resolveProxy(req *http.Request) (*url.URL, error) {
	if resolve := installedResolver.Load(); resolve != nil {
		return (*resolve)(req)
	}
	return SystemProxy()(req)
}

// ProxyForSettings builds the resolver the proxy_* settings describe; values
// are the plain settings, as settings.Store.Load returns them. An error means
// the settings cannot be installed, so a save can refuse them first.
func ProxyForSettings(values map[string]string) (ProxyResolver, error) {
	mode := values["proxy_mode"]
	manualURL := ""
	if mode == ProxyModeManual {
		manualURL = BuildProxyURL(values["proxy_type"], values["proxy_host"], values["proxy_port"],
			values["proxy_username"], values["proxy_password"])
	}
	return ProxyFor(mode, manualURL)
}

// ConfigureProxyFromSettings installs the proxy behaviour the settings
// describe. Call it at startup and again whenever the proxy settings are
// saved.
func ConfigureProxyFromSettings(values map[string]string) error {
	resolve, err := ProxyForSettings(values)
	if err != nil {
		return err
	}
	installedResolver.Store(&resolve)
	return nil
}
