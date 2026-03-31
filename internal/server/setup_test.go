package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/server"
	"github.com/appsynergy-io/conduit/internal/shared"
)

// newTestServerWithDB creates a test server and returns the DB for direct seeding.
func newTestServerWithDB(t *testing.T, mode string) (*server.Server, *auth.JWTManager, *db.DB) {
	t.Helper()
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	jwtMgr, err := auth.NewJWTManager("test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	cfg := &shared.Config{
		Server: shared.ServerConfig{Mode: mode, HTTPAddr: ":0"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := server.New(cfg, database, jwtMgr, nil, logger, nil)
	return srv, jwtMgr, database
}

// ---------------------------------------------------------------------------
// Setup Status
// ---------------------------------------------------------------------------

func TestSetupStatus_Fresh(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, true, body["setupRequired"])
	assert.Equal(t, "domain_config", body["currentStep"])
}

func TestSetupStatus_InProgress(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	err = database.UpdateSetupStep(ctx, "passkey_registration", `["domain_config","admin_account"]`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, true, body["setupRequired"])
	assert.Equal(t, "passkey_registration", body["currentStep"])
}

func TestSetupStatus_Complete(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	err = database.CompleteSetup(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// After completion, setup endpoints return 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// Setup Configure
// ---------------------------------------------------------------------------

func TestSetupConfigure_Success(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	// Pre-create setup state (as InitSetup would)
	testToken := "test-setup-token-" + uuid.NewString()
	err := database.CreateSetupState(ctx, "hash-of-token")
	require.NoError(t, err)
	server.SetSetupTokenForTesting(testToken)
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{
		"setupToken": "` + testToken + `",
		"domain": "test.example.com",
		"adminEmail": "admin@test.com",
		"organizationName": "Test Org",
		"firstName": "Admin",
		"lastName": "User"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "provisioning", resp["status"])
	assert.Contains(t, resp, "redirectUrl")

	// Verify setup state advanced
	state, err := database.GetSetupState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "passkey_registration", state.CurrentStep)
}

func TestSetupConfigure_InvalidToken(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting("real-token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{
		"setupToken": "wrong-token",
		"domain": "test.example.com",
		"adminEmail": "admin@test.com",
		"organizationName": "Test Org",
		"firstName": "Admin",
		"lastName": "User"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
}

func TestSetupConfigure_MissingFields(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting("token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	// Missing organizationName
	body := `{
		"setupToken": "token",
		"domain": "test.example.com",
		"adminEmail": "admin@test.com",
		"firstName": "Admin",
		"lastName": "User"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetupConfigure_AlreadyComplete(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	err = database.CompleteSetup(ctx)
	require.NoError(t, err)
	server.SetSetupTokenForTesting("token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{
		"setupToken": "token",
		"domain": "test.example.com",
		"adminEmail": "admin@test.com",
		"organizationName": "Test Org",
		"firstName": "Admin",
		"lastName": "User"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// Setup Passkey (dev mode stub)
// ---------------------------------------------------------------------------

func TestSetupPasskey_DevMode(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	testToken := "passkey-test-token"
	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting(testToken)
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{"setupToken": "` + testToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/passkey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "setup_complete", resp["status"])

	// Verify setup is marked complete in DB
	complete, err := database.IsSetupComplete(ctx)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestSetupPasskey_InvalidToken(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting("real-token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{"setupToken": "wrong-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/passkey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSetupPasskey_AlreadyComplete(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	err = database.CompleteSetup(ctx)
	require.NoError(t, err)
	server.SetSetupTokenForTesting("token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{"setupToken": "token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/passkey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSetupPasskey_ProductionMode(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "production")
	ctx := context.Background()

	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting("token")
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{"setupToken": "token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/passkey", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Production mode: passkey not implemented yet → 501
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

// ---------------------------------------------------------------------------
// Setup Configure — dev mode creates password hash
// ---------------------------------------------------------------------------

func TestSetupConfigure_DevModeCreatesPasswordHash(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	testToken := "dev-setup-token-" + uuid.NewString()
	err := database.CreateSetupState(ctx, "hash")
	require.NoError(t, err)
	server.SetSetupTokenForTesting(testToken)
	t.Cleanup(func() { server.SetSetupTokenForTesting("") })

	body := `{
		"setupToken": "` + testToken + `",
		"domain": "test.example.com",
		"adminEmail": "devadmin@test.com",
		"organizationName": "Dev Org",
		"firstName": "Dev",
		"lastName": "Admin"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup/configure", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	// Verify the user has a password hash (dev mode hashes setup token as password)
	user, err := database.GetUserByEmail(ctx, "devadmin@test.com")
	require.NoError(t, err)
	require.NotNil(t, user)
	require.NotNil(t, user.PasswordHash, "dev mode must set password hash")

	// Verify the token works as the password
	match, err := auth.VerifyPassword(testToken, *user.PasswordHash)
	require.NoError(t, err)
	assert.True(t, match, "setup token must work as password in dev mode")
}
