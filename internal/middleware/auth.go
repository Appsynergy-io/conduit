package middleware

import (
	"net/http"
	"strings"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
)

// Auth validates the JWT from the Authorization header and injects claims into context.
// Returns 401 for missing/invalid tokens (NIST IA-2, OWASP A07, API2).
func Auth(jwtMgr *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				apierror.Unauthorized(w, r, "Missing authorization header.", nil)
				return
			}

			token, found := strings.CutPrefix(header, "Bearer ")
			if !found || token == "" {
				apierror.Unauthorized(w, r, "Invalid authorization header format.", nil)
				return
			}

			claims, err := jwtMgr.ValidateToken(token)
			if err != nil {
				apierror.Unauthorized(w, r, "Invalid or expired token.", err)
				return
			}

			ctx := WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole checks that the authenticated user has one of the required roles.
// Must be used after Auth middleware (NIST AC-3, AC-6, OWASP A01, API5).
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromCtx(r.Context())
			if claims == nil {
				apierror.Unauthorized(w, r, "Authentication required.", nil)
				return
			}

			for _, role := range claims.Roles {
				if allowed[role] {
					next.ServeHTTP(w, r)
					return
				}
			}

			apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		})
	}
}

// RequireService checks that the authenticated user's JWT includes the required service
// (NIST AC-3, Conduit service registry — jwt.services claim).
func RequireService(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromCtx(r.Context())
			if claims == nil {
				apierror.Unauthorized(w, r, "Authentication required.", nil)
				return
			}

			for _, s := range claims.Services {
				if s == service {
					next.ServeHTTP(w, r)
					return
				}
			}

			apierror.Forbidden(w, r, "Service not enabled for this tenant.", nil)
		})
	}
}
