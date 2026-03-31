package middleware

import (
	"net/http"
	"strings"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// RequireJSON rejects POST/PUT/PATCH requests that don't have Content-Type: application/json
// (OWASP API8, NIST SI-10).
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			if ct == "" || !strings.HasPrefix(ct, "application/json") {
				apierror.UnsupportedMediaType(w, r, "Content-Type must be application/json.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// MaxBody limits the request body size (OWASP API4 — unrestricted resource consumption).
func MaxBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
