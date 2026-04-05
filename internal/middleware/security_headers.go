package middleware

import (
	"fmt"
	"net/http"
)

// SecurityHeaders adds mandatory security headers to every response
// (NIST SC-8, SC-18, OWASP A05).
// quicPort advertises HTTP/3 via Alt-Svc header. Pass "" to omit.
func SecurityHeaders(next http.Handler) http.Handler {
	return SecurityHeadersWithAltSvc("")(next)
}

// SecurityHeadersWithAltSvc adds security headers and optionally advertises HTTP/3
// availability via the Alt-Svc header when quicPort is non-empty.
func SecurityHeadersWithAltSvc(quicPort string) func(http.Handler) http.Handler {
	var altSvc string
	if quicPort != "" {
		altSvc = fmt.Sprintf(`h3=":%s"; ma=86400`, quicPort)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' wss:; worker-src 'self' blob:; frame-ancestors 'none'")
			if altSvc != "" {
				w.Header().Set("Alt-Svc", altSvc)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoCacheHeaders adds Cache-Control: no-store for sensitive responses
// (NIST SC-28, OWASP ASVS V8).
func NoCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
