package webui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"LiteRSS/internal/desktopapi"
)

// wantCSP is spec D16 spelled out, so a drift in either place fails here.
var wantCSP = map[string]string{
	"script-src": "'self'",
	"style-src":  "'self'",
	"img-src":    "'self' http: https: data:",
	"font-src":   "'self' data:",
	"frame-src":  "'none'",
	"object-src": "'none'",
}

var dist = fstest.MapFS{
	"index.html":       {Data: []byte("<!doctype html><title>LiteRSS</title>")},
	"assets/index.js":  {Data: []byte("console.log('ok')")},
	"assets/index.css": {Data: []byte("body{}")},
}

var api = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, "api "+r.URL.Path)
})

func assertCSP(t *testing.T, header http.Header) {
	t.Helper()
	got := map[string]string{}
	for _, directive := range strings.Split(header.Get("Content-Security-Policy"), ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(directive), " ")
		if name != "" {
			got[name] = value
		}
	}
	if len(got) != len(wantCSP) {
		t.Errorf("CSP directives = %v, want %v", got, wantCSP)
	}
	for name, want := range wantCSP {
		if got[name] != want {
			t.Errorf("CSP %s = %q, want %q", name, got[name], want)
		}
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestSiteServesFrontendWithCSP(t *testing.T) {
	site := Site(api, dist)
	for _, path := range []string{"/", "/assets/index.js", "/assets/index.css"} {
		t.Run(path, func(t *testing.T) {
			response := get(t, site, path)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			assertCSP(t, response.Header())
		})
	}
}

func TestSiteRoutesAPI(t *testing.T) {
	response := get(t, Site(api, dist), "/api/version")
	if body := response.Body.String(); body != "api /api/version" {
		t.Fatalf("body = %q, want the API handler", body)
	}
}

func TestWailsMiddlewareMount(t *testing.T) {
	wailsRuntime := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "wails runtime")
	})
	h := WailsMiddleware(Site(api, dist))(wailsRuntime)

	if body := get(t, h, "/wails/runtime").Body.String(); body != "wails runtime" {
		t.Errorf("/wails body = %q, want the Wails runtime", body)
	}
	if body := get(t, h, "/api/version").Body.String(); body != "api /api/version" {
		t.Errorf("/api body = %q, want the API handler", body)
	}
	response := get(t, h, "/")
	if response.Code != http.StatusOK {
		t.Fatalf("/ status = %d, want 200", response.Code)
	}
	assertCSP(t, response.Header())
}

// The browser forensics channel: the same Site behind desktopapi's guard, hit
// the way Chrome loads a Vite build (crossorigin module script with Origin).
func TestBrowserChannelMount(t *testing.T) {
	server, err := desktopapi.Start("127.0.0.1:0", Site(api, dist))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	for _, path := range []string{"/", "/assets/index.js"} {
		req, err := http.NewRequest(http.MethodGet, server.Origin()+path, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Origin", server.Origin())
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		assertCSP(t, response.Header)
	}
}
