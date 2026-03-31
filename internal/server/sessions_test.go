package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func seedSession(t *testing.T, database *db.DB, tenantID, userID, sessType string) string {
	t.Helper()
	id := uuid.NewString()
	sess := &db.Session{
		ID:        id,
		TenantID:  tenantID,
		UserID:    userID,
		Type:      sessType,
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, database.CreateSession(context.Background(), sess))
	return id
}

// ---------------------------------------------------------------------------
// List Sessions
// ---------------------------------------------------------------------------

func TestListSessions_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "s@test.com",
		FirstName: "S", LastName: "U", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	seedSession(t, database, tenantID, userID, "web")
	seedSession(t, database, tenantID, userID, "cli")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
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

func TestListSessions_FilterByType(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "f@test.com",
		FirstName: "F", LastName: "U", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	seedSession(t, database, tenantID, userID, "web")
	seedSession(t, database, tenantID, userID, "cli")
	seedSession(t, database, tenantID, userID, "web")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?type=web", nil)
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

// ---------------------------------------------------------------------------
// Revoke Session
// ---------------------------------------------------------------------------

func TestRevokeSession_OwnSession(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "r@test.com",
		FirstName: "R", LastName: "U", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	sessID := seedSession(t, database, tenantID, userID, "web")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+sessID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	s, err := database.GetSessionByID(ctx, sessID)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestRevokeSession_MemberCantRevokeOthers(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	otherUserID := uuid.NewString()
	other := &db.User{ID: otherUserID, TenantID: tenantID, Email: "other@test.com",
		FirstName: "O", LastName: "U", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, other))

	sessID := seedSession(t, database, tenantID, otherUserID, "web")

	// Different member trying to revoke other's session
	token, _ := jwtMgr.IssueAccessToken(uuid.NewString(), tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+sessID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRevokeSession_AdminCanRevokeOthers(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	otherUserID := uuid.NewString()
	other := &db.User{ID: otherUserID, TenantID: tenantID, Email: "target@test.com",
		FirstName: "T", LastName: "U", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, other))

	sessID := seedSession(t, database, tenantID, otherUserID, "web")

	// Admin revoking other's session
	token, _ := jwtMgr.IssueAccessToken(uuid.NewString(), tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+sessID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestRevokeSession_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
