package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestJWTManager creates a JWTManager with a fresh Ed25519 keypair for testing.
func newTestJWTManager(t *testing.T, accessTTL time.Duration) *auth.JWTManager {
	t.Helper()
	mgr, err := auth.NewJWTManager("test-issuer", accessTTL, 24*time.Hour)
	require.NoError(t, err)
	return mgr
}

// buildAuthServer creates an httptest server with the given middleware chain
// and a simple 200-OK handler. It returns the server and a cleanup function.
func buildAuthServer(middlewares ...func(http.Handler) http.Handler) *httptest.Server {
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Apply in reverse so the first middleware in the slice is outermost.
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return httptest.NewServer(handler)
}

// --- Auth middleware tests ---

func TestAuth_ValidToken(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	token, err := mgr.IssueAccessToken("user-123", "tenant-abc", "sess-1",
		[]string{"org_admin"}, []string{"remote-access"})
	require.NoError(t, err)

	var gotClaims *auth.Claims
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = middleware.ClaimsFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler = middleware.Auth(mgr)(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, gotClaims)
	assert.Equal(t, "user-123", gotClaims.Subject)
	assert.Equal(t, "tenant-abc", gotClaims.TenantID)
	assert.Equal(t, []string{"org_admin"}, gotClaims.Roles)
	assert.Equal(t, []string{"remote-access"}, gotClaims.Services)
}

func TestAuth_MissingHeader(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	handler := middleware.Auth(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestAuth_InvalidFormat(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	handler := middleware.Auth(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestAuth_ExpiredToken(t *testing.T) {
	mgr := newTestJWTManager(t, 1*time.Millisecond)

	token, err := mgr.IssueAccessToken("user-123", "tenant-abc", "sess-1",
		[]string{"org_admin"}, []string{"remote-access"})
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	handler := middleware.Auth(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestAuth_InvalidToken(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	handler := middleware.Auth(mgr)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt-at-all")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

// --- RequireRole tests ---

func TestRequireRole_Allowed(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	token, err := mgr.IssueAccessToken("user-1", "tenant-1", "sess-1",
		[]string{"org_admin"}, []string{"remote-access"})
	require.NoError(t, err)

	srv := buildAuthServer(
		middleware.Auth(mgr),
		middleware.RequireRole("org_admin", "platform_owner"),
	)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/protected", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequireRole_Denied(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	token, err := mgr.IssueAccessToken("user-2", "tenant-1", "sess-2",
		[]string{"org_member"}, []string{"remote-access"})
	require.NoError(t, err)

	srv := buildAuthServer(
		middleware.Auth(mgr),
		middleware.RequireRole("platform_owner"),
	)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/protected", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestRequireRole_NoClaims(t *testing.T) {
	// RequireRole without Auth middleware -- no claims in context.
	srv := buildAuthServer(
		middleware.RequireRole("org_admin"),
	)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/protected", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// --- RequireService tests ---

func TestRequireService_Allowed(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	token, err := mgr.IssueAccessToken("user-3", "tenant-1", "sess-3",
		[]string{"org_admin"}, []string{"remote-access"})
	require.NoError(t, err)

	srv := buildAuthServer(
		middleware.Auth(mgr),
		middleware.RequireService("remote-access"),
	)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequireService_Denied(t *testing.T) {
	mgr := newTestJWTManager(t, 15*time.Minute)

	// Token has no services.
	token, err := mgr.IssueAccessToken("user-4", "tenant-1", "sess-4",
		[]string{"org_admin"}, nil)
	require.NoError(t, err)

	srv := buildAuthServer(
		middleware.Auth(mgr),
		middleware.RequireService("remote-access"),
	)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
