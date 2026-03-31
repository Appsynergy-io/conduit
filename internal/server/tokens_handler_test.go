package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func seedJoinToken(t *testing.T, database *db.DB, tenantID, name, tokenType string, revoked int) string {
	t.Helper()
	tokenID := uuid.NewString()
	token := &db.JoinToken{
		ID:        tokenID,
		TenantID:  tenantID,
		Type:      tokenType,
		Name:      name,
		TokenHash: "hash-" + tokenID[:8],
		Revoked:   revoked,
	}
	require.NoError(t, database.CreateJoinToken(context.Background(), token))
	return tokenID
}

func TestListJoinTokens_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedJoinToken(t, database, tenantID, "Token A", "single_use", 0)
	seedJoinToken(t, database, tenantID, "Token B", "persistent", 0)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/tokens", nil)
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

func TestListJoinTokens_FilterByType(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedJoinToken(t, database, tenantID, "Token A", "single_use", 0)
	seedJoinToken(t, database, tenantID, "Token B", "persistent", 0)
	seedJoinToken(t, database, tenantID, "Token C", "persistent", 0)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/tokens?type=persistent", nil)
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

func TestListJoinTokens_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	seedJoinToken(t, database, tenantA, "Token A1", "single_use", 0)
	seedJoinToken(t, database, tenantB, "Token B1", "single_use", 0)
	seedJoinToken(t, database, tenantB, "Token B2", "persistent", 0)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/tokens", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1, "tenant A must only see its own tokens")
}

func TestCreateJoinToken_Success(t *testing.T) {
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

	body := `{"name":"Fleet Token","type":"persistent","labels":{"env":"prod"},"ttlHours":24}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "Fleet Token", resp["name"])
	assert.Equal(t, "persistent", resp["type"])
	assert.NotEmpty(t, resp["token"], "plaintext token must be returned")
	assert.NotNil(t, resp["expiresAt"])
}

func TestCreateJoinToken_SingleUse(t *testing.T) {
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

	body := `{"name":"One Shot","type":"single_use"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(1), resp["maxUses"])
}

func TestCreateJoinToken_InvalidType(t *testing.T) {
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

	body := `{"name":"Bad","type":"multi_use"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateJoinToken_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, []string{"remote-access"})

	body := `{"name":"Test","type":"single_use"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/tokens", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRevokeJoinToken_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tokenID := seedJoinToken(t, database, tenantID, "Revoke Me", "persistent", 0)
	jwtToken, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/tokens/"+tokenID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify revoked
	tok, _ := database.GetJoinTokenByID(ctx, tokenID)
	assert.Equal(t, 1, tok.Revoked)
}

func TestRevokeJoinToken_AlreadyRevoked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tokenID := seedJoinToken(t, database, tenantID, "Already Revoked", "persistent", 1)
	jwtToken, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/tokens/"+tokenID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRevokeJoinToken_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	jwtToken, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/tokens/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRevokeJoinToken_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	tokenID := seedJoinToken(t, database, tenantB, "Token B1", "persistent", 0)
	jwtToken, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/tokens/"+tokenID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "cross-tenant access must be blocked")
}
