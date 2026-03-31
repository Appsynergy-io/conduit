package middleware

import (
	"net/http"

	"github.com/google/uuid"
)

// RequestID generates a unique request ID and adds it to context + response headers
// (NIST AU-3 — request correlation).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		ctx := WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
