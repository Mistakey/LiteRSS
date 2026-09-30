package desktopapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateLoopbackAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "IPv4 loopback", address: "127.0.0.1:1234"},
		{name: "IPv6 loopback", address: "[::1]:1234"},
		{name: "ephemeral loopback port", address: "127.0.0.1:0"},
		{name: "all interfaces", address: "0.0.0.0:1234", wantErr: true},
		{name: "non-loopback", address: "192.0.2.10:1234", wantErr: true},
		{name: "missing port", address: "127.0.0.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLoopbackAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateLoopbackAddress(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
			}
		})
	}
}

func TestProtectLocalAPI(t *testing.T) {
	const self = "http://127.0.0.1:1235"
	handler := protectLocalAPI(self, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// Chrome sends Origin on same-origin POST and on same-origin GETs in CORS
	// mode (<script type="module" crossorigin>, <link crossorigin>), but not on
	// a plain same-origin fetch GET (Epic literss-rb0 dev-isolation.md:
	// git show 4056ac16:docs/specs/literss-rb0/dev-isolation.md).
	tests := []struct {
		name           string
		method         string
		host           string
		origin         string
		secFetchSite   string
		wantStatusCode int
	}{
		{name: "agent request without Origin", host: "127.0.0.1:1235", wantStatusCode: http.StatusNoContent},
		{name: "agent request via localhost Host", host: "localhost:1235", wantStatusCode: http.StatusNoContent},
		{name: "same-origin fetch GET", host: "127.0.0.1:1235", secFetchSite: "same-origin", wantStatusCode: http.StatusNoContent},
		{name: "same-origin POST", method: http.MethodPost, host: "127.0.0.1:1235", origin: self, secFetchSite: "same-origin", wantStatusCode: http.StatusNoContent},
		{name: "same-origin crossorigin asset GET", host: "127.0.0.1:1235", origin: self, secFetchSite: "same-origin", wantStatusCode: http.StatusNoContent},
		{name: "address-bar navigation", host: "127.0.0.1:1235", secFetchSite: "none", wantStatusCode: http.StatusNoContent},
		{name: "foreign host", host: "attacker.example", wantStatusCode: http.StatusForbidden},
		{name: "foreign Host with own Origin", method: http.MethodPost, host: "attacker.example:1235", origin: self, wantStatusCode: http.StatusForbidden},
		{name: "cross-site origin", method: http.MethodPost, host: "127.0.0.1:1235", origin: "https://attacker.example", secFetchSite: "cross-site", wantStatusCode: http.StatusForbidden},
		{name: "cross-site without Origin", host: "127.0.0.1:1235", secFetchSite: "cross-site", wantStatusCode: http.StatusForbidden},
		{name: "same-site other loopback port", method: http.MethodPost, host: "127.0.0.1:1235", origin: "http://127.0.0.1:5173", secFetchSite: "same-site", wantStatusCode: http.StatusForbidden},
		{name: "same-site without Origin", host: "127.0.0.1:1235", secFetchSite: "same-site", wantStatusCode: http.StatusForbidden},
		{name: "own origin but same-site", method: http.MethodPost, host: "127.0.0.1:1235", origin: self, secFetchSite: "same-site", wantStatusCode: http.StatusForbidden},
		{name: "localhost alias origin", method: http.MethodPost, host: "localhost:1235", origin: "http://localhost:1235", secFetchSite: "same-origin", wantStatusCode: http.StatusForbidden},
		{name: "localhost alias asset GET", host: "localhost:1235", origin: "http://localhost:1235", secFetchSite: "same-origin", wantStatusCode: http.StatusForbidden},
		{name: "https scheme of own host", method: http.MethodPost, host: "127.0.0.1:1235", origin: "https://127.0.0.1:1235", wantStatusCode: http.StatusForbidden},
		{name: "opaque origin", method: http.MethodPost, host: "127.0.0.1:1235", origin: "null", wantStatusCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "http://127.0.0.1:1235/api/version", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatusCode {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestServerLifecycle(t *testing.T) {
	server, err := Start("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	response, err := http.Get("http://" + server.Address() + "/api/version")
	if err != nil {
		t.Fatalf("GET desktop API: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := <-server.Errors(); err != nil {
		t.Fatalf("Serve() error after shutdown = %v", err)
	}
}

func TestServerHasNoWriteTimeout(t *testing.T) {
	server, err := Start("127.0.0.1:0", http.NotFoundHandler())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Shutdown(context.Background())
	if server.httpServer.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %v would cut the 25 s long poll", server.httpServer.WriteTimeout)
	}
}

func TestServerAcceptsItsOwnOrigin(t *testing.T) {
	server, err := Start("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	for origin, want := range map[string]int{
		server.Origin():      http.StatusNoContent,
		"http://127.0.0.1:1": http.StatusForbidden,
		"http://localhost:1": http.StatusForbidden,
	} {
		req, err := http.NewRequest(http.MethodPost, server.Origin()+"/api/version", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Origin", origin)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST with Origin %q: %v", origin, err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("Origin %q: status = %d, want %d", origin, response.StatusCode, want)
		}
	}
}

func TestServerOrigin(t *testing.T) {
	for address, want := range map[string]string{
		"127.0.0.1:1235": "http://127.0.0.1:1235",
		"[::1]:1235":     "http://[::1]:1235",
	} {
		if got := originOf(address); got != want {
			t.Errorf("originOf(%q) = %q, want %q", address, got, want)
		}
	}
}
