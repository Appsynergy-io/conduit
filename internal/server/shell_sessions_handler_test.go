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

// seedShellSession creates a shell session for testing.
func seedShellSession(t *testing.T, database *db.DB, tenantID, agentID, userID string) *db.ShellSession {
	t.Helper()
	s := &db.ShellSession{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		AgentID:     agentID,
		UserID:      userID,
		Status:      "active",
		Shell:       strPtr("/bin/bash"),
		Cols:        intPtr(80),
		Rows:        intPtr(24),
		Recording:   1,
		Pinned:      0,
		IdleTimeout: 3600,
	}
	require.NoError(t, database.CreateShellSession(context.Background(), s))
	return s
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// seedShellTestTenant creates a tenant with the remote-access service enabled.
func seedShellTestTenant(t *testing.T, database *db.DB) string {
	t.Helper()
	ctx := context.Background()
	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	serviceID := uuid.NewString()
	require.NoError(t, database.CreateService(ctx, serviceID, "remote-access", "Remote Access", "desc"))
	require.NoError(t, database.EnableServiceForTenant(ctx, tenantID, serviceID))

	return tenantID
}

// ---------------------------------------------------------------------------
// List All Shell Sessions — GET /api/v1/shell/sessions
// ---------------------------------------------------------------------------

func TestListAllShellSessions_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "ss@test.com",
		FirstName: "S", LastName: "U", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	seedShellSession(t, database, tenantID, agent.ID, userID)
	seedShellSession(t, database, tenantID, agent.ID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shell/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestListAllShellSessions_MemberSeesOwnOnly(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	user1 := &db.User{ID: uuid.NewString(), TenantID: tenantID, Email: "u1@test.com",
		FirstName: "U", LastName: "1", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, user1))

	user2 := &db.User{ID: uuid.NewString(), TenantID: tenantID, Email: "u2@test.com",
		FirstName: "U", LastName: "2", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, user2))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	seedShellSession(t, database, tenantID, agent.ID, user1.ID)
	seedShellSession(t, database, tenantID, agent.ID, user2.ID)
	seedShellSession(t, database, tenantID, agent.ID, user2.ID)

	// User1 should only see their own session
	token, _ := jwtMgr.IssueAccessToken(user1.ID, tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shell/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1)
}

func TestListAllShellSessions_FilterByStatus(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "sf@test.com",
		FirstName: "S", LastName: "F", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	s1 := seedShellSession(t, database, tenantID, agent.ID, userID)
	seedShellSession(t, database, tenantID, agent.ID, userID)

	// Detach one
	require.NoError(t, database.DetachShellSession(ctx, s1.ID))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Filter by detached status
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shell/sessions?status=detached", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1)
}

func TestListAllShellSessions_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shell/sessions", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// List Agent Shell Sessions — GET /api/v1/agents/{agentId}/shell/sessions
// ---------------------------------------------------------------------------

func TestListAgentShellSessions_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "as@test.com",
		FirstName: "A", LastName: "S", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	agent1 := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "agent-1",
		AgentKeyHash: "hash-1", Status: "online",
	}
	agent2 := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "agent-2",
		AgentKeyHash: "hash-2", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent1))
	require.NoError(t, database.CreateAgent(ctx, agent2))

	seedShellSession(t, database, tenantID, agent1.ID, userID)
	seedShellSession(t, database, tenantID, agent1.ID, userID)
	seedShellSession(t, database, tenantID, agent2.ID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agent1.ID+"/shell/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

// ---------------------------------------------------------------------------
// Get Shell Session — GET /api/v1/agents/{agentId}/shell/sessions/{sessionId}
// ---------------------------------------------------------------------------

func TestGetShellSession_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "gs@test.com",
		FirstName: "G", LastName: "S", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	s := seedShellSession(t, database, tenantID, agent.ID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agent.ID+"/shell/sessions/"+s.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var got db.ShellSession
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, "active", got.Status)
}

func TestGetShellSession_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "nf@test.com",
		FirstName: "N", LastName: "F", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetShellSession_OwnershipEnforced(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	ownerID := uuid.NewString()
	owner := &db.User{ID: ownerID, TenantID: tenantID, Email: "owner@test.com",
		FirstName: "O", LastName: "W", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, owner))

	otherID := uuid.NewString()
	other := &db.User{ID: otherID, TenantID: tenantID, Email: "other@test.com",
		FirstName: "O", LastName: "T", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, other))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	s := seedShellSession(t, database, tenantID, agent.ID, ownerID)

	// Other member cannot see owner's session
	token, _ := jwtMgr.IssueAccessToken(otherID, tenantID, "sess", []string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agent.ID+"/shell/sessions/"+s.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetShellSession_AdminCanSeeOthers(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	ownerID := uuid.NewString()
	owner := &db.User{ID: ownerID, TenantID: tenantID, Email: "sowner@test.com",
		FirstName: "O", LastName: "W", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, owner))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	s := seedShellSession(t, database, tenantID, agent.ID, ownerID)

	// Admin can see any session
	adminID := uuid.NewString()
	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agent.ID+"/shell/sessions/"+s.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// Update Shell Session — PATCH /api/v1/agents/{agentId}/shell/sessions/{sessionId}
// (These test the handler validation; the SessionManager interaction requires
// a live session in memory which needs WebSocket/agent infrastructure.
// We test the validation and auth paths here.)
// ---------------------------------------------------------------------------

func TestUpdateShellSession_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "up@test.com",
		FirstName: "U", LastName: "P", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"pinned": true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Session not in SessionManager → 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateShellSession_InvalidBody(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "ub@test.com",
		FirstName: "U", LastName: "B", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateShellSession_InvalidIdleTimeout(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "ut@test.com",
		FirstName: "U", LastName: "T", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	tests := []struct {
		name string
		body string
	}{
		{"too low", `{"idleTimeout": 10}`},
		{"too high", `{"idleTimeout": 100000}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// Terminate Shell Session — DELETE /api/v1/agents/{agentId}/shell/sessions/{sessionId}
// ---------------------------------------------------------------------------

func TestTerminateShellSession_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "td@test.com",
		FirstName: "T", LastName: "D", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Not in SessionManager and not in DB → 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTerminateShellSession_AlreadyClosed(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := seedShellTestTenant(t, database)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "tc@test.com",
		FirstName: "T", LastName: "C", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	agent := &db.Agent{
		ID: uuid.NewString(), TenantID: tenantID, Hostname: "test-agent",
		AgentKeyHash: "hash-test", Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	s := seedShellSession(t, database, tenantID, agent.ID, userID)
	require.NoError(t, database.CloseShellSession(ctx, s.ID))

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+agent.ID+"/shell/sessions/"+s.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Already closed sessions return 204 (idempotent)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestTerminateShellSession_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+uuid.NewString()+"/shell/sessions/"+uuid.NewString(), nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
