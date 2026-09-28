// Package desktopapi exposes the desktop application's API on a loopback-only
// HTTP listener. Development builds also serve the frontend on it as the
// browser forensics channel (spec D5); protectLocalAPI guards both.
package desktopapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Server owns the desktop API listener and its lifecycle.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
	errors     chan error
}

// Start starts an HTTP server on a loopback address. It returns after the
// listener has been created, so address conflicts are reported synchronously.
func Start(address string, handler http.Handler) (*Server, error) {
	if handler == nil {
		return nil, errors.New("desktop API handler is required")
	}
	if err := validateLoopbackAddress(address); err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for desktop API on %s: %w", address, err)
	}

	resolved := listener.Addr().String()
	// No WriteTimeout: the sync status long poll holds its response for about
	// 25 seconds, and a shorter write deadline would cut it (pitfall 31).
	httpServer := &http.Server{
		Addr:              resolved,
		Handler:           protectLocalAPI(originOf(resolved), handler),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	server := &Server{
		httpServer: httpServer,
		listener:   listener,
		errors:     make(chan error, 1),
	}

	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.errors <- err
		close(server.errors)
	}()

	return server, nil
}

// Address returns the listener's resolved address.
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

// Origin is the only browser origin the listener accepts, e.g.
// http://127.0.0.1:1235. Pages opened through an alias such as localhost are
// refused, so the forensics entry point is always this exact origin.
func (s *Server) Origin() string {
	return originOf(s.Address())
}

// Errors reports the terminal serving error. A graceful shutdown reports nil.
func (s *Server) Errors() <-chan error {
	return s.errors
}

// Shutdown stops accepting desktop API requests and waits for those in
// progress. It does not cancel them: whoever serves a long poll wakes it
// first (pitfall 31).
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func validateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid desktop API address %q: %w", address, err)
	}
	if port == "" {
		return fmt.Errorf("invalid desktop API address %q: port is required", address)
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("desktop API address %q must use a loopback IP", address)
	}
	return nil
}

func originOf(address string) string {
	return "http://" + address
}

// protectLocalAPI admits a request only when its Host is loopback and it is
// not a browser request from another origin: Origin must be absent or exactly
// self, and Sec-Fetch-Site must not be same-site or cross-site. Chrome sends
// Origin on same-origin POSTs and on crossorigin asset GETs, so rejecting every
// Origin would block the frontend's own CSS and JS (ADR 0012).
func protectLocalAPI(self string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Sec-Fetch-Site")

		if !isLoopbackHost(r.Host) {
			http.Error(w, "desktop API requires a loopback Host header", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != self {
			http.Error(w, "cross-origin browser requests are not allowed", http.StatusForbidden)
			return
		}
		switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
		case "same-site", "cross-site":
			http.Error(w, "cross-origin browser requests are not allowed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostPort string) bool {
	host := hostPort
	if parsedHost, _, err := net.SplitHostPort(hostPort); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
