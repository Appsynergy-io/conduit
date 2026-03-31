package middleware

import (
	"log/slog"
	"net/http"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// Recover catches panics in handlers and returns a 500 RFC 9457 response
// (OWASP ASVS V7 — last-resort error handler).
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered in handler",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", RequestIDFromCtx(r.Context()),
				)
				apierror.Internal(w, r, nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
