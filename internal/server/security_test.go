package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/middleware"
)

// ---------------------------------------------------------------------------
// 1. Error Response Sanitization (NIST SI-11, REC-API-23)
// ---------------------------------------------------------------------------

func TestErrorResponse_RFC9457Format(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&problem)
	require.NoError(t, err)

	// RFC 9457 required fields
	assert.Contains(t, problem, "type")
	assert.Contains(t, problem, "title")
	assert.Contains(t, problem, "status")

	// Must NOT contain internal implementation details
	body := w.Body.String()
	for _, forbidden := range []string{"stack", "trace", "panic", "goroutine", "sql", "SELECT", "INSERT"} {
		assert.NotContains(t, strings.ToLower(body), strings.ToLower(forbidden),
			"response body must not contain %q", forbidden)
	}
}

func TestErrorResponse_NoInternalLeakage(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer expired.invalid.token")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	body := w.Body.String()
	for _, forbidden := range []string{"/root/", "/home/", ".go:", "runtime.", "internal/", "github.com/"} {
		assert.NotContains(t, body, forbidden,
			"response body must not contain file path or internal reference %q", forbidden)
	}
}

// ---------------------------------------------------------------------------
// 2. Recover Middleware Sanitization
// ---------------------------------------------------------------------------

func TestRecoverMiddleware_NoLeakInResponse(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Recover)
	r.Use(middleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("sql: connection failed to /var/lib/conduit/data.db")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&problem)
	require.NoError(t, err)

	// Verify RFC 9457 structure
	assert.Contains(t, problem, "type")
	assert.Contains(t, problem, "title")
	assert.Contains(t, problem, "status")
	assert.Equal(t, float64(http.StatusInternalServerError), problem["status"])

	// The panic message must NOT leak to the client
	body := w.Body.String()
	assert.NotContains(t, body, "/var/lib/",
		"panic message must not leak file paths")
	assert.NotContains(t, body, "sql:",
		"panic message must not leak SQL details")
	assert.NotContains(t, body, "connection failed",
		"panic message must not leak internal error text")
}

// ---------------------------------------------------------------------------
// 3. Security Headers Completeness
// ---------------------------------------------------------------------------

func TestSecurityHeaders_AllRequired(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	headers := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains; preload",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
	}

	for name, expected := range headers {
		assert.Equal(t, expected, w.Header().Get(name),
			"header %s must be %q", name, expected)
	}

	csp := w.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "default-src 'self'",
		"CSP must contain default-src 'self'")

	assert.NotEmpty(t, w.Header().Get("X-Request-Id"),
		"X-Request-Id must be present and non-empty")
}

func TestSecurityHeaders_OnErrorResponses(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Security headers must still be present on error responses
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, "camera=(), microphone=(), geolocation=()", w.Header().Get("Permissions-Policy"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "default-src 'self'")
	assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
}

// ---------------------------------------------------------------------------
// 4. Auth Boundary Tests
// ---------------------------------------------------------------------------

func TestAuth_EmptyBearer(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestAuth_MalformedJWT(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestAuth_BearerCaseSensitive(t *testing.T) {
	srv, jwtMgr := newTestServer(t)

	token, err := jwtMgr.IssueAccessToken(
		"user-123", "tenant-456", "session-789",
		[]string{"org_admin"}, []string{"remote-access"},
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "bearer "+token) // lowercase "bearer"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"lowercase 'bearer' prefix must be rejected (case-sensitive)")
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

// ---------------------------------------------------------------------------
// 5. Content-Type Enforcement
// ---------------------------------------------------------------------------

func TestContentType_XMLRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", nil)
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestContentType_FormRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestContentType_EmptyBodyGET(t *testing.T) {
	srv, _ := newTestServer(t)

	// GET without Content-Type must not be rejected as 415
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusUnsupportedMediaType, w.Code,
		"GET requests without Content-Type must not be rejected as 415")
	// /health returns 200
	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// 6. Cache Control on Sensitive Responses
// ---------------------------------------------------------------------------

func TestCacheControl_AuthenticatedEndpoints(t *testing.T) {
	srv, jwtMgr := newTestServer(t)

	token, err := jwtMgr.IssueAccessToken(
		"user-123", "tenant-456", "session-789",
		[]string{"org_admin"}, []string{"remote-access"},
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"),
		"authenticated endpoints must have Cache-Control: no-store")
}

func TestCacheControl_PublicEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEqual(t, "no-store", w.Header().Get("Cache-Control"),
		"public endpoints should NOT have Cache-Control: no-store")
}
