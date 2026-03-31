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

func seedWebhook(t *testing.T, database *db.DB, tenantID, url string, enabled bool) string {
	t.Helper()
	subID := uuid.NewString()
	sub := &db.WebhookSubscription{
		ID:         subID,
		TenantID:   tenantID,
		URL:        url,
		SecretHash: "secret-hash-" + subID[:8],
		Events:     `["auth.login"]`,
		Enabled:    enabled,
	}
	require.NoError(t, database.CreateWebhookSubscription(context.Background(), sub))
	return subID
}

func TestListWebhooks_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedWebhook(t, database, tenantID, "https://example.com/hook1", true)
	seedWebhook(t, database, tenantID, "https://example.com/hook2", true)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
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

func TestListWebhooks_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListWebhooks_TenantIsolation(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantA, "A"))
	require.NoError(t, database.CreateTenant(ctx, tenantB, "B"))

	seedWebhook(t, database, tenantA, "https://example.com/a1", true)
	seedWebhook(t, database, tenantB, "https://example.com/b1", true)
	seedWebhook(t, database, tenantB, "https://example.com/b2", true)

	token, _ := jwtMgr.IssueAccessToken("admin", tenantA, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	data := resp["data"].([]interface{})
	assert.Len(t, data, 1, "tenant A must only see its own webhooks")
}

func TestListWebhooks_Pagination(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	for i := 0; i < 5; i++ {
		seedWebhook(t, database, tenantID, "https://example.com/hook"+uuid.NewString()[:4], true)
	}

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks?limit=2", nil)
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
}

func TestCreateWebhook_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"url":"https://example.com/webhook","events":["auth.login","agent.connected"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "https://example.com/webhook", resp["url"])
	assert.NotEmpty(t, resp["secret"], "signing secret must be returned")
	assert.True(t, resp["enabled"].(bool))
}

func TestCreateWebhook_DevModeLocalhost(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"url":"http://localhost:9090/hook","events":["auth.login"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code, "http://localhost must be allowed in dev mode")
}

func TestCreateWebhook_ProdModeRejectsHTTP(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "prod")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"url":"http://example.com/hook","events":["auth.login"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "HTTP must be rejected in prod mode")
}

func TestCreateWebhook_InvalidEventType(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"url":"https://example.com/hook","events":["invalid.event"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWebhook_NoEvents(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	adminID := uuid.NewString()
	require.NoError(t, database.CreateUser(ctx, &db.User{
		ID: adminID, TenantID: tenantID, Email: "admin@test.com",
		FirstName: "Admin", LastName: "User", Role: "org_admin", Status: "active",
	}))

	token, _ := jwtMgr.IssueAccessToken(adminID, tenantID, "sess", []string{"org_admin"}, nil)

	body := `{"url":"https://example.com/hook","events":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWebhook_NonAdmin(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	token, _ := jwtMgr.IssueAccessToken("user", uuid.NewString(), "sess", []string{"org_member"}, nil)

	body := `{"url":"https://example.com/hook","events":["auth.login"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
