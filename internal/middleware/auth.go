package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
)

// AuthCookieName is the httpOnly cookie used for browser-based authentication.
// Uses __Host- prefix for strict cookie security (OWASP V3, NIST SC-23).
const AuthCookieName = "__Host-conduit_token"

// RefreshCookieName is the httpOnly cookie carrying the longer-lived refresh
// token. The server reads it in POST /auth/refresh to issue new access + refresh
// cookies without requiring a full re-authentication (NIST IA-11, OWASP V3).
const RefreshCookieName = "__Host-conduit_refresh"

// CITokenPrefix is the prefix for CI/automation tokens to distinguish from JWTs.
const CITokenPrefix = "cdci_"

// CITokenValidator looks up a CI token by its plaintext value and returns
// synthetic claims if valid. Implemented by the server package.
type CITokenValidator interface {
	ValidateCIToken(ctx context.Context, token string) (*auth.Claims, error)
}

// ExtractToken retrieves a bearer token from the Authorization header first,
// then falls back to the httpOnly auth cookie. Returns empty string if neither
// is present (NIST IA-2, OWASP A07).
func ExtractToken(r *http.Request) string {
	// Check Authorization header first
	if header := r.Header.Get("Authorization"); header != "" {
		if token, found := strings.CutPrefix(header, "Bearer "); found && token != "" {
			return token
		}
	}

	// Fall back to httpOnly cookie
	if cookie, err := r.Cookie(AuthCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// Auth validates the JWT from the Authorization header or httpOnly cookie
// and injects claims into context. Also supports CI tokens (cdci_ prefix).
// Returns 401 for missing/invalid tokens (NIST IA-2, OWASP A07, API2).
func Auth(jwtMgr *auth.JWTManager, ciValidator ...CITokenValidator) func(http.Handler) http.Handler {
	var ciVal CITokenValidator
	if len(ciValidator) > 0 {
		ciVal = ciValidator[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractToken(r)
			if token == "" {
				apierror.Unauthorized(w, r, "Missing authentication credentials.", nil)
				return
			}

			var claims *auth.Claims
			var err error

			if strings.HasPrefix(token, CITokenPrefix) && ciVal != nil {
				// CI token authentication (NIST IA-5)
				claims, err = ciVal.ValidateCIToken(r.Context(), token)
			} else {
				// JWT authentication
				claims, err = jwtMgr.ValidateToken(token)
			}

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
