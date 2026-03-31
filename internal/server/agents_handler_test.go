package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func seedAgent(t *testing.T, database *db.DB, tenantID, hostname, status string) string {
	t.Helper()
	agentID := uuid.NewString()
	agent := &db.Agent{
		ID:           agentID,
		TenantID:     tenantID,
		Hostname:     hostname,
		AgentKeyHash: "test-hash-" + agentID[:8],
		Status:       status,
	}
	require.NoError(t, database.CreateAgent(context.Background(), agent))
	return agentID
}

func TestListAgents_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgent(t, database, tenantID, "web-01", "online")
	seedAgent(t, database, tenantID, "web-02", "offline")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
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

func TestListAgents_FilterByStatus(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgent(t, database, tenantID, "web-01", "online")
	seedAgent(t, database, tenantID, "web-02", "offline")
	seedAgent(t, database, tenantID, "web-03", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?status=online", nil)
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

func TestListAgents_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	seedAgent(t, database, tenantA, "web-A1", "online")
	seedAgent(t, database, tenantB, "web-B1", "online")
	seedAgent(t, database, tenantB, "web-B2", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1, "tenant A must only see its own agents")
}

func TestListAgents_NoServiceScope(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAgents_Pagination(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	for i := 0; i < 5; i++ {
		seedAgent(t, database, tenantID, "web-"+uuid.NewString()[:4], "online")
	}

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?limit=2", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)

	pg := resp["pagination"].(map[string]interface{})
	assert.Equal(t, true, pg["hasMore"])
	assert.NotNil(t, pg["nextCursor"])
}

func TestGetAgent_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, agentID, resp["id"])
	assert.Equal(t, "web-01", resp["hostname"])
}

func TestGetAgent_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetAgent_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	agentID := seedAgent(t, database, tenantB, "web-B1", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "cross-tenant access must be blocked")
}

func TestDeleteAgent_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "offline")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	agent, _ := database.GetAgentByID(ctx, agentID)
	assert.Nil(t, agent)
}

func TestDeleteAgent_NonAdmin(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "offline")
	token, _ := jwtMgr.IssueAccessToken("user", tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+agentID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDeleteAgent_InvalidID(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("admin", uuid.NewString(), "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
