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

// ---------------------------------------------------------------------------
// PATCH /api/v1/agents/{agentId} — Update Agent
// ---------------------------------------------------------------------------

func seedAgentWithLabels(t *testing.T, database *db.DB, tenantID, hostname, status, labels string) string {
	t.Helper()
	agentID := uuid.NewString()
	var labelsPtr *string
	if labels != "" {
		labelsPtr = &labels
	}
	agent := &db.Agent{
		ID:           agentID,
		TenantID:     tenantID,
		Hostname:     hostname,
		AgentKeyHash: "test-hash-" + agentID[:8],
		Status:       status,
		Labels:       labelsPtr,
	}
	require.NoError(t, database.CreateAgent(context.Background(), agent))
	return agentID
}

func TestUpdateAgent_Labels(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"labels":{"env":"production","role":"web"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, agentID, resp["id"])

	labels := resp["labels"].(map[string]interface{})
	assert.Equal(t, "production", labels["env"])
	assert.Equal(t, "web", labels["role"])
}

func TestUpdateAgent_DisplayName(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"displayName":"Production Web Server"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "Production Web Server", resp["displayName"])
}

func TestUpdateAgent_ClearLabels(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgentWithLabels(t, database, tenantID, "web-01", "online", `{"env":"prod"}`)
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"labels":{}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Nil(t, resp["labels"])
}

func TestUpdateAgent_NonAdmin(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("user", tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	body := `{"labels":{"env":"staging"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUpdateAgent_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"labels":{"env":"prod"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateAgent_InvalidLabelKey(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"labels":{"invalid key!":"value"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAgent_TooManyLabels(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Build 51 labels
	labels := make(map[string]string)
	for i := 0; i < 51; i++ {
		labels["key"+uuid.NewString()[:8]] = "val"
	}
	b, _ := json.Marshal(map[string]interface{}{"labels": labels})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAgent_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	agentID := seedAgent(t, database, tenantB, "web-B1", "online")
	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"labels":{"env":"hacked"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agentID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// GET /api/v1/agents?label= — Label Filtering
// ---------------------------------------------------------------------------

func TestListAgents_LabelFilter(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgentWithLabels(t, database, tenantID, "prod-web", "online", `{"env":"production","role":"web"}`)
	seedAgentWithLabels(t, database, tenantID, "staging-api", "online", `{"env":"staging","role":"api"}`)
	seedAgentWithLabels(t, database, tenantID, "prod-api", "online", `{"env":"production","role":"api"}`)
	seedAgent(t, database, tenantID, "no-labels", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Exact match
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?label=env:production", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestListAgents_LabelFilterOR(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgentWithLabels(t, database, tenantID, "prod-web", "online", `{"env":"production","role":"web"}`)
	seedAgentWithLabels(t, database, tenantID, "staging-api", "online", `{"env":"staging","role":"api"}`)
	seedAgentWithLabels(t, database, tenantID, "prod-api", "online", `{"env":"production","role":"api"}`)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// OR: role:web,api
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?label=role:web,api", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3)
}

func TestListAgents_LabelFilterNegate(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgentWithLabels(t, database, tenantID, "prod-web", "online", `{"env":"production"}`)
	seedAgentWithLabels(t, database, tenantID, "staging-api", "online", `{"env":"staging"}`)
	seedAgent(t, database, tenantID, "no-labels", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// NOT: !env:staging
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?label=!env:staging", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2, "should return prod-web + no-labels")
}

func TestListAgents_LabelFilterExistence(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgentWithLabels(t, database, tenantID, "prod-web", "online", `{"env":"production","role":"web"}`)
	seedAgentWithLabels(t, database, tenantID, "staging", "online", `{"env":"staging"}`)
	seedAgent(t, database, tenantID, "no-labels", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Existence: role:
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?label=role:", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1, "only prod-web has the role label")
}

// ---------------------------------------------------------------------------
// GET /api/v1/labels — List Labels
// ---------------------------------------------------------------------------

func TestListLabels_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgentWithLabels(t, database, tenantID, "prod-web", "online", `{"env":"production","role":"web"}`)
	seedAgentWithLabels(t, database, tenantID, "staging-api", "online", `{"env":"staging","role":"api"}`)
	seedAgentWithLabels(t, database, tenantID, "prod-api", "online", `{"env":"production","role":"api"}`)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/labels", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	data := resp["data"].([]interface{})
	assert.Len(t, data, 2) // env, role

	// Sorted by key
	first := data[0].(map[string]interface{})
	assert.Equal(t, "env", first["key"])
	assert.Equal(t, float64(3), first["count"])

	second := data[1].(map[string]interface{})
	assert.Equal(t, "role", second["key"])
	assert.Equal(t, float64(3), second["count"])
}

func TestListLabels_Empty(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/labels", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Empty(t, data)
}

func TestListLabels_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	seedAgentWithLabels(t, database, tenantA, "a-server", "online", `{"env":"production"}`)
	seedAgentWithLabels(t, database, tenantB, "b-server", "online", `{"region":"eu-west"}`)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/labels", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1) // only env from tenant A
	first := data[0].(map[string]interface{})
	assert.Equal(t, "env", first["key"])
}
