package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
)

// Recovery returns a middleware that turns a panic in a handler into a 500
// instead of taking the whole process down, and logs it with its stack.
func Recovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("PANIC recovered: %v\n%s", err, debug.Stack())

					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("Internal Server Error"))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
