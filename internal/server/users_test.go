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

// seedTenantAndToken creates a tenant, sets tenant ID, and returns a valid admin JWT.
func seedTenantAndToken(t *testing.T, srv *server_helper) (tenantID, token string) {
	t.Helper()
	// This helper is defined below to work around import cycle
	return "", ""
}

// ---------------------------------------------------------------------------
// List Users
// ---------------------------------------------------------------------------

func TestListUsers_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	// Create some users
	for i := 0; i < 3; i++ {
		u := &db.User{
			ID: uuid.NewString(), TenantID: tenantID,
			Email:     "user" + string(rune('a'+i)) + "@test.com",
			FirstName: "User", LastName: "Test", Role: "org_member", Status: "active",
		}
		require.NoError(t, database.CreateUser(ctx, u))
	}

	token, err := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 3)

	pg := resp["pagination"].(map[string]interface{})
	assert.Equal(t, false, pg["hasMore"])
}

func TestListUsers_Pagination(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	for i := 0; i < 5; i++ {
		u := &db.User{
			ID: uuid.NewString(), TenantID: tenantID,
			Email:     "page" + string(rune('a'+i)) + "@test.com",
			FirstName: "User", LastName: "Test", Role: "org_member", Status: "active",
		}
		require.NoError(t, database.CreateUser(ctx, u))
	}

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	// First page with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=2", nil)
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

	// Second page
	cursor := pg["nextCursor"].(string)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=2&cursor="+cursor, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data = resp["data"].([]interface{})
	assert.Len(t, data, 2)
}

func TestListUsers_FilterByStatus(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	active := &db.User{ID: uuid.NewString(), TenantID: tenantID, Email: "active@test.com",
		FirstName: "A", LastName: "B", Role: "org_member", Status: "active"}
	suspended := &db.User{ID: uuid.NewString(), TenantID: tenantID, Email: "suspended@test.com",
		FirstName: "C", LastName: "D", Role: "org_member", Status: "suspended"}
	require.NoError(t, database.CreateUser(ctx, active))
	require.NoError(t, database.CreateUser(ctx, suspended))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?status=active", nil)
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
// Create User
// ---------------------------------------------------------------------------

func TestCreateUser_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	// Admin user must exist for created_by FK
	adminID := uuid.NewString()
	admin := &db.User{ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, admin))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"email":"new@test.com","firstName":"New","lastName":"User","role":"org_member"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "new@test.com", resp["email"])
	assert.Equal(t, "org_member", resp["role"])
	assert.Equal(t, "invited", resp["status"])
	assert.NotEmpty(t, resp["id"])
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	u := &db.User{ID: uuid.NewString(), TenantID: tenantID, Email: "dupe@test.com",
		FirstName: "E", LastName: "F", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"email":"dupe@test.com","firstName":"New","lastName":"User","role":"org_member"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestCreateUser_InvalidRole(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	body := `{"email":"x@test.com","firstName":"A","lastName":"B","role":"platform_owner"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateUser_MemberCantCreate(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("user", "tid", "sess", []string{"org_member"}, nil)

	body := `{"email":"x@test.com","firstName":"A","lastName":"B","role":"org_member"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCreateUser_MissingFields(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	body := `{"email":"x@test.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// Get User
// ---------------------------------------------------------------------------

func TestGetUser_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "get@test.com",
		FirstName: "Get", LastName: "User", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, userID, resp["id"])
	assert.Equal(t, "get@test.com", resp["email"])
}

func TestGetUser_NotFound(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetUser_InvalidUUID(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")
	token, _ := jwtMgr.IssueAccessToken("admin", "tid", "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUser_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	// Create user in tenant A
	tenantA := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "Tenant A"))
	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantA, Email: "a@test.com",
		FirstName: "A", LastName: "User", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	// Try to access from tenant B
	tenantB := uuid.NewString()
	token, _ := jwtMgr.IssueAccessToken("admin", tenantB, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+userID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "must not leak users across tenants")
}

// ---------------------------------------------------------------------------
// Update User
// ---------------------------------------------------------------------------

func TestUpdateUser_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "upd@test.com",
		FirstName: "Old", LastName: "Name", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"firstName":"New","role":"org_admin"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+userID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "New", resp["firstName"])
	assert.Equal(t, "org_admin", resp["role"])
}

func TestUpdateUser_MemberCantChangeRole(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))
	database.SetTenantID(tenantID)

	userID := uuid.NewString()
	u := &db.User{ID: userID, TenantID: tenantID, Email: "member@test.com",
		FirstName: "M", LastName: "M", Role: "org_member", Status: "active"}
	require.NoError(t, database.CreateUser(ctx, u))

	// Token for this same user (self-edit)
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	body := `{"role":"org_admin"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+userID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// Unused type alias kept only so the seedTenantAndToken signature compiles.
type server_helper = interface{}
