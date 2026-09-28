package httputil

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/mattn/go-ieproxy"
)

// systemProxyTTL bounds how stale the cached system proxy configuration may be.
// go-ieproxy reads it once per process, but a reader stays open for days while
// the user switches a tool like Clash on and off, so re-read it on a timer.
const systemProxyTTL = 30 * time.Second

var (
	systemProxyMu sync.Mutex
	systemProxyFn ProxyResolver
	systemProxyAt time.Time
)

// SystemProxy returns the resolver that follows the operating system's proxy
// settings: the WinHTTP/registry configuration on Windows, CFNetwork on macOS,
// and the HTTP_PROXY family of environment variables elsewhere. Go's own
// net/http only ever reads the environment variables, which is why this goes
// through go-ieproxy.
func SystemProxy() ProxyResolver {
	return func(req *http.Request) (*url.URL, error) {
		return currentSystemProxy()(req)
	}
}

func currentSystemProxy() ProxyResolver {
	systemProxyMu.Lock()
	defer systemProxyMu.Unlock()

	if systemProxyFn == nil || time.Since(systemProxyAt) > systemProxyTTL {
		ieproxy.ReloadConf()
		systemProxyFn = ieproxy.GetProxyFunc()
		systemProxyAt = time.Now()
	}
	return systemProxyFn
}
