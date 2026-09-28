// Package httputil provides HTTP client utilities: proxy-aware clients and the
// proxy mode that decides where their requests are routed.
package httputil

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// InsecureSkipTLSVerifyEnv enables TLS certificate verification bypass for
// private LAN services such as self-signed Ollama endpoints. It is intentionally
// opt-in and environment-scoped.
const InsecureSkipTLSVerifyEnv = "LITERSS_INSECURE_SKIP_TLS_VERIFY"

// BuildProxyURL constructs a proxy URL from the manual proxy settings, ""
// when the host or port is missing. Credentials are escaped, so a password
// may hold '@' or ':'.
func BuildProxyURL(proxyType, proxyHost, proxyPort, username, password string) string {
	if proxyHost == "" || proxyPort == "" {
		return ""
	}
	u := url.URL{Scheme: proxyType, Host: net.JoinHostPort(proxyHost, proxyPort)}
	switch {
	case username != "" && password != "":
		u.User = url.UserPassword(username, password)
	case username != "":
		u.User = url.User(username)
	}
	return u.String()
}

// CreateHTTPClient creates an HTTP client whose requests follow the proxy mode
// installed by ConfigureProxy.
//
// This is the shared client: the AI client, Baidu translation and the update
// checker all ride on it. Changing its transport changes their outbound
// behaviour too, so a caller with different needs gets its own constructor
// rather than a new flag here.
func CreateHTTPClient(timeout time.Duration) *http.Client {
	return newClient(timeout, false)
}

// CreateWebScrapingClient creates the client used to fetch article pages from
// the open web. It differs from CreateHTTPClient in one respect: it attempts
// HTTP/2, which is what stops some sites (old.reddit.com among them) from
// answering with 403. Proxy mode and TLS policy are identical.
func CreateWebScrapingClient(timeout time.Duration) *http.Client {
	return newClient(timeout, true)
}

func newClient(timeout time.Duration, forceHTTP2 bool) *http.Client {
	transport := &http.Transport{
		Proxy: resolveProxy,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecureSkipTLSVerifyEnabled(),
		},
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   forceHTTP2,
		WriteBufferSize:     32 * 1024,
		ReadBufferSize:      32 * 1024,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

func insecureSkipTLSVerifyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(InsecureSkipTLSVerifyEnv))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
