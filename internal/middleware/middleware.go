// Package middleware provides HTTP middleware components for the application.
package middleware

import (
	"net/http"
)

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Apply wraps h in the given middlewares. They are applied in reverse order, so
// the first middleware in the list ends up the outermost handler.
func Apply(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
