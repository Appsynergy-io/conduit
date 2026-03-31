package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/server"
	"github.com/appsynergy-io/conduit/internal/shared"
)

func newTestServer(t *testing.T) (*server.Server, *auth.JWTManager) {
	t.Helper()
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	jwtMgr, err := auth.NewJWTManager("test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	cfg := &shared.Config{
		Server: shared.ServerConfig{Mode: "dev", HTTPAddr: ":0"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := server.New(cfg, database, jwtMgr, nil, logger)
	return srv, jwtMgr
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestHealthEndpoint_SecurityHeaders(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, w.Header().Get("Strict-Transport-Security"), "max-age=")
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
}

func TestAuthenticatedEndpoint_NoAuth(t *testing.T) {
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
	assert.Equal(t, float64(http.StatusUnauthorized), problem["status"])
}

func TestAuthenticatedEndpoint_ValidAuth(t *testing.T) {
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

	// Users endpoint is now implemented — returns 200 with empty list
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthenticatedEndpoint_InvalidToken(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer garbage.token.value")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestServiceScoped_NoService(t *testing.T) {
	srv, jwtMgr := newTestServer(t)

	token, err := jwtMgr.IssueAccessToken(
		"user-123", "tenant-456", "session-789",
		[]string{"org_admin"}, []string{},
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestServiceScoped_WithService(t *testing.T) {
	srv, jwtMgr := newTestServer(t)

	token, err := jwtMgr.IssueAccessToken(
		"user-123", "tenant-456", "session-789",
		[]string{"org_admin"}, []string{"remote-access"},
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Agents endpoint now returns 200 with empty list
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireJSON_POST(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestNotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
