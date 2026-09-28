// Package webui serves the built frontend. It is the one place the
// Content-Security-Policy header is set, so the Wails asset channel and the
// desktopapi browser forensics channel always send the same policy (spec D16).
package webui

import (
	"io/fs"
	"net/http"
	"strings"
)

// ContentSecurityPolicy is sent with every frontend response. Article HTML is
// untrusted: scripts and styles come only from the app's own origin, images may
// come from anywhere, and frames and plugins are refused outright.
const ContentSecurityPolicy = "script-src 'self'; style-src 'self'; " +
	"img-src 'self' http: https: data:; font-src 'self' data:; " +
	"frame-src 'none'; object-src 'none'"

// Static serves files from dist with the CSP header.
func Static(dist fs.FS) http.Handler {
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", ContentSecurityPolicy)
		files.ServeHTTP(w, r)
	})
}

// Site routes /api/ to api and everything else to Static(dist). Both channels
// mount it: the Wails asset server and, in development builds, desktopapi.
func Site(api http.Handler, dist fs.FS) http.Handler {
	static := Static(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// WailsMiddleware leaves /wails to the Wails runtime and hands every other
// request to site.
func WailsMiddleware(site http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/wails") {
				next.ServeHTTP(w, r)
				return
			}
			site.ServeHTTP(w, r)
		})
	}
}
