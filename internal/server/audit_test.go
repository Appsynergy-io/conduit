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

func seedAuditLogs(t *testing.T, database *db.DB, tenantID string, count int, eventType, outcome string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		entry := &db.AuditEntry{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			EventType: eventType,
			Outcome:   outcome,
		}
		require.NoError(t, database.InsertAuditLog(ctx, entry))
	}
}

func TestListAuditEvents_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAuditLogs(t, database, tenantID, 3, "auth.login", "success")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3)
}

func TestListAuditEvents_FilterByEventType(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAuditLogs(t, database, tenantID, 2, "auth.login", "success")
	seedAuditLogs(t, database, tenantID, 1, "user.created", "success")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?eventType=auth.login", nil)
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

func TestListAuditEvents_FilterByOutcome(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAuditLogs(t, database, tenantID, 2, "auth.login", "success")
	seedAuditLogs(t, database, tenantID, 3, "auth.login", "failure")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?outcome=failure", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3)
}

func TestListAuditEvents_Pagination(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAuditLogs(t, database, tenantID, 5, "test.event", "success")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?limit=2", nil)
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

func TestListAuditEvents_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	seedAuditLogs(t, database, tenantA, 3, "auth.login", "success")
	seedAuditLogs(t, database, tenantB, 2, "auth.login", "success")

	// Query as tenant A — should only see 3
	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3, "tenant A must only see its own audit logs")
}
