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

// ---------------------------------------------------------------------------
// List Groups
// ---------------------------------------------------------------------------

func TestListGroups_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	for i := 0; i < 3; i++ {
		g := &db.Group{ID: uuid.NewString(), TenantID: tenantID, Name: "Group " + string(rune('A'+i))}
		require.NoError(t, database.CreateGroup(ctx, g))
	}

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
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

func TestListGroups_Search(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: uuid.NewString(), TenantID: tenantID, Name: "Engineering"}))
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: uuid.NewString(), TenantID: tenantID, Name: "Marketing"}))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups?search=eng", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1)
}

// ---------------------------------------------------------------------------
// Create Group
// ---------------------------------------------------------------------------

func TestCreateGroup_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"name":"New Group","description":"A test group"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "New Group", resp["name"])
	assert.Equal(t, float64(0), resp["memberCount"])
}

func TestCreateGroup_MemberCantCreate(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("user", "tid", "sess", []string{"org_member"}, nil)

	body := `{"name":"Forbidden Group"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateGroup_EmptyName(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// Get Group
// ---------------------------------------------------------------------------

func TestGetGroup_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	groupID := uuid.NewString()
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: groupID, TenantID: tenantID, Name: "Devs"}))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "Devs", resp["name"])
}

func TestGetGroup_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// Update Group
// ---------------------------------------------------------------------------

func TestUpdateGroup_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	groupID := uuid.NewString()
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: groupID, TenantID: tenantID, Name: "Old Name"}))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"name":"New Name"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/groups/"+groupID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "New Name", resp["name"])
}

// ---------------------------------------------------------------------------
// Delete Group
// ---------------------------------------------------------------------------

func TestDeleteGroup_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	groupID := uuid.NewString()
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: groupID, TenantID: tenantID, Name: "ToDelete"}))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify deleted
	g, err := database.GetGroupByID(ctx, groupID)
	require.NoError(t, err)
	assert.Nil(t, g)
}

func TestDeleteGroup_MemberCantDelete(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	groupID := uuid.NewString()
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: groupID, TenantID: tenantID, Name: "Protected"}))

	token, _ := jwtMgr.IssueAccessToken("user", tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestDeleteGroup_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	groupID := uuid.NewString()
	require.NoError(t, database.CreateGroup(ctx, &db.Group{ID: groupID, TenantID: tenantA, Name: "A Group"}))

	// Admin of tenant B
	tenantB := uuid.NewString()
	token, _ := jwtMgr.IssueAccessToken("admin", tenantB, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/"+groupID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "must not leak groups across tenants")
}
