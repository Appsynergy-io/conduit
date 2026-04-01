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

// seedRecordingState seeds all entities needed for recordings tests and returns IDs.
func seedRecordingState(t *testing.T, database *db.DB) (tenantID, userID, agentID string) {
	t.Helper()
	ctx := context.Background()

	tenantID = uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test Org"))
	database.SetTenantID(tenantID)

	userID = uuid.NewString()
	user := &db.User{
		ID: userID, TenantID: tenantID, Email: "rec@test.com",
		FirstName: "Rec", LastName: "User", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(ctx, user))

	agentID = uuid.NewString()
	agent := &db.Agent{
		ID: agentID, TenantID: tenantID, Hostname: "rec-agent",
		AgentKeyHash: "hash-" + agentID[:8], Status: "online",
	}
	require.NoError(t, database.CreateAgent(ctx, agent))

	serviceID := uuid.NewString()
	require.NoError(t, database.CreateService(ctx, serviceID, "remote-access", "Remote Access", "desc"))
	require.NoError(t, database.EnableServiceForTenant(ctx, tenantID, serviceID))

	return tenantID, userID, agentID
}

func seedRecording(t *testing.T, database *db.DB, tenantID, agentID, userID string) string {
	t.Helper()
	ctx := context.Background()

	sessionID := uuid.NewString()
	session := &db.ShellSession{
		ID: sessionID, TenantID: tenantID, AgentID: agentID, UserID: userID,
		Status: "closed", Recording: 1,
	}
	require.NoError(t, database.CreateShellSession(ctx, session))

	recID := uuid.NewString()
	rec := &db.ShellRecording{
		ID: recID, TenantID: tenantID, SessionID: sessionID,
		AgentID: agentID, UserID: userID,
		AgentHostname: "rec-agent", UserEmail: "rec@test.com",
		Duration: 60, SizeBytes: 2048, Format: "asciicast-v2",
		Data: []byte(`{"version":2,"width":80,"height":24,"timestamp":1234567890}`),
	}
	require.NoError(t, database.CreateShellRecording(ctx, rec))
	return recID
}

// ---------------------------------------------------------------------------
// GET /api/v1/recordings (PR #34 — shell recordings list)
// ---------------------------------------------------------------------------

func TestListRecordings_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, agentID := seedRecordingState(t, database)

	seedRecording(t, database, tenantID, agentID, userID)
	seedRecording(t, database, tenantID, agentID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(2), resp["total"])
	items := resp["items"].([]interface{})
	assert.Len(t, items, 2)
}

func TestListRecordings_FilterByAgent(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, agentID := seedRecordingState(t, database)

	// Create another agent
	agent2ID := uuid.NewString()
	agent2 := &db.Agent{
		ID: agent2ID, TenantID: tenantID, Hostname: "other-agent",
		AgentKeyHash: "hash-other", Status: "online",
	}
	require.NoError(t, database.CreateAgent(context.Background(), agent2))

	seedRecording(t, database, tenantID, agentID, userID)
	seedRecording(t, database, tenantID, agentID, userID)
	seedRecording(t, database, tenantID, agent2ID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings?agent_id="+agentID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(2), resp["total"])
}

func TestListRecordings_Pagination(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, agentID := seedRecordingState(t, database)

	for i := 0; i < 5; i++ {
		seedRecording(t, database, tenantID, agentID, userID)
	}

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings?limit=2&offset=0", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(5), resp["total"])
	assert.Len(t, resp["items"].([]interface{}), 2)
}

func TestListRecordings_InvalidAgentID(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, _ := seedRecordingState(t, database)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings?agent_id=not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListRecordings_InvalidLimit(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, _ := seedRecordingState(t, database)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings?limit=999", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListRecordings_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListRecordings_MissingService(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, _ := seedRecordingState(t, database)

	// Token without remote-access service
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ---------------------------------------------------------------------------
// GET /api/v1/recordings/{recordingId} (PR #34 — get recording with data)
// ---------------------------------------------------------------------------

func TestGetRecording_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, agentID := seedRecordingState(t, database)
	recID := seedRecording(t, database, tenantID, agentID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/"+recID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, recID, resp["id"])
	assert.NotEmpty(t, resp["data"], "should include base64-encoded recording data")
	assert.Equal(t, "asciicast-v2", resp["format"])
}

func TestGetRecording_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, _ := seedRecordingState(t, database)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetRecording_InvalidID(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID, _ := seedRecordingState(t, database)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetRecording_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID1, userID1, agentID1 := seedRecordingState(t, database)
	recID := seedRecording(t, database, tenantID1, agentID1, userID1)

	// Create second tenant
	tenantID2 := uuid.NewString()
	require.NoError(t, database.CreateTenant(context.Background(), tenantID2, "Other Org"))
	user2ID := uuid.NewString()
	user2 := &db.User{
		ID: user2ID, TenantID: tenantID2, Email: "other@test.com",
		FirstName: "O", LastName: "U", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(context.Background(), user2))

	// Token for tenant2 user
	token, _ := jwtMgr.IssueAccessToken(user2ID, tenantID2, "sess",
		[]string{"org_member"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/"+recID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"recording from tenant1 must not be visible to tenant2")
}
