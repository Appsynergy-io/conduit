package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func seedCITokenDB(t *testing.T, database *db.DB, tenantID, userID, name string) string {
	t.Helper()
	tokenID := uuid.NewString()
	token := &db.CIToken{
		ID:        tokenID,
		TenantID:  tenantID,
		CreatedBy: userID,
		Name:      name,
		TokenHash: "hash-ci-" + tokenID[:8],
		Scopes:    `["agents:read"]`,
	}
	require.NoError(t, database.CreateCIToken(context.Background(), token))
	return tokenID
}

// ---------------------------------------------------------------------------
// List CI Tokens
// ---------------------------------------------------------------------------

func TestListCITokens_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	seedCITokenDB(t, database, tenantID, adminID, "Token A")
	seedCITokenDB(t, database, tenantID, adminID, "Token B")

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ci-tokens", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestListCITokens_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	userA := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: userA, TenantID: tenantA, Email: "a@test.com",
		FirstName: "A", LastName: "User", Role: "org_admin", Status: "active",
	}))
	userB := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: userB, TenantID: tenantB, Email: "b@test.com",
		FirstName: "B", LastName: "User", Role: "org_admin", Status: "active",
	}))

	seedCITokenDB(t, database, tenantA, userA, "Token A")
	seedCITokenDB(t, database, tenantB, userB, "Token B1")
	seedCITokenDB(t, database, tenantB, userB, "Token B2")

	token, _ := jwtMgr.IssueAccessToken(userA, tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ci-tokens", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1, "tenant A must only see its own CI tokens")
}

func TestListCITokens_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/ci-tokens", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// Create CI Token
// ---------------------------------------------------------------------------

func TestCreateCIToken_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"name":"GitHub Actions","scopes":["agents:read","shell:execute"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "GitHub Actions", resp["name"])
	assert.NotEmpty(t, resp["token"], "plaintext token must be returned once")
	assert.NotEmpty(t, resp["id"])
	assert.NotNil(t, resp["scopes"])
	scopes := resp["scopes"].([]interface{})
	assert.Len(t, scopes, 2)

	// Token value starts with cdci_ prefix
	tokenVal := resp["token"].(string)
	assert.True(t, strings.HasPrefix(tokenVal, "cdci_"), "token must have cdci_ prefix")
}

func TestCreateCIToken_WithExpiry(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	expiry := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"name":"Expiring Token","scopes":["agents:read"],"expiresAt":"` + expiry + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotNil(t, resp["expiresAt"])
}

func TestCreateCIToken_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, []string{"remote-access"})

	body := `{"name":"Test","scopes":["agents:read"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateCIToken_EmptyName(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"name":"","scopes":["agents:read"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCIToken_InvalidScope(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"name":"Bad Scope","scopes":["agents:read","admin:nuke"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCIToken_EmptyScopes(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"name":"No Scopes","scopes":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCIToken_PastExpiry(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	pastExpiry := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"name":"Past","scopes":["agents:read"],"expiresAt":"` + pastExpiry + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCIToken_UnknownField(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"name":"Test","scopes":["agents:read"],"extraField":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ci-tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// Revoke CI Token
// ---------------------------------------------------------------------------

func TestRevokeCIToken_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	tokenID := seedCITokenDB(t, database, tenantID, adminID, "Revoke Me")
	jwtToken, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/ci-tokens/"+tokenID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	got, err := database.GetCITokenByID(ctx, tokenID)
	require.NoError(t, err)
	assert.Nil(t, got, "token must be deleted after revocation")
}

func TestRevokeCIToken_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	jwtToken, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/ci-tokens/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRevokeCIToken_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	jwtToken, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/ci-tokens/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRevokeCIToken_InvalidUUID(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	jwtToken, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/ci-tokens/not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRevokeCIToken_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	userB := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: userB, TenantID: tenantB, Email: "b@test.com",
		FirstName: "B", LastName: "User", Role: "org_admin", Status: "active",
	}))

	tokenID := seedCITokenDB(t, database, tenantB, userB, "Token B")
	jwtToken, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/ci-tokens/"+tokenID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "cross-tenant revocation must be blocked")
}
