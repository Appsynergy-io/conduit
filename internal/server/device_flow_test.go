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

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
)

func seedUserForDeviceFlow(t *testing.T, database *db.DB, tenantID string) *db.User {
	t.Helper()
	hash, err := auth.HashPassword("testpassword")
	require.NoError(t, err)
	displayName := "Device Tester"
	user := &db.User{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Email:        "device-test@example.com",
		DisplayName:  &displayName,
		Role:         "org_admin",
		PasswordHash: &hash,
	}
	require.NoError(t, database.CreateUser(context.Background(), user))
	return user
}

func TestDeviceFlow_Begin(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	body := `{"clientId":"conduit-cli","profileName":"default"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/begin", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.NotEmpty(t, resp["deviceCode"])
	assert.NotEmpty(t, resp["userCode"])
	assert.NotEmpty(t, resp["verificationUri"])
	assert.Equal(t, float64(900), resp["expiresIn"])
	assert.Equal(t, float64(5), resp["interval"])

	// User code should be in format XXXX-XXXX
	userCode := resp["userCode"].(string)
	assert.Len(t, userCode, 9, "user code should be 9 chars (XXXX-XXXX)")
	assert.Equal(t, '-', rune(userCode[4]), "user code should have dash in position 4")
}

func TestDeviceFlow_PollPending(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	// Begin flow
	beginBody := `{"clientId":"conduit-cli"}`
	beginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/begin", strings.NewReader(beginBody))
	beginReq.Header.Set("Content-Type", "application/json")
	beginW := httptest.NewRecorder()
	srv.Router().ServeHTTP(beginW, beginReq)
	require.Equal(t, http.StatusOK, beginW.Code)

	var beginResp map[string]interface{}
	require.NoError(t, json.NewDecoder(beginW.Body).Decode(&beginResp))
	deviceCode := beginResp["deviceCode"].(string)

	// Poll — should be pending
	pollBody := `{"deviceCode":"` + deviceCode + `"}`
	pollReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/poll", strings.NewReader(pollBody))
	pollReq.Header.Set("Content-Type", "application/json")
	pollW := httptest.NewRecorder()
	srv.Router().ServeHTTP(pollW, pollReq)

	assert.Equal(t, http.StatusBadRequest, pollW.Code)
	var pollResp map[string]interface{}
	require.NoError(t, json.NewDecoder(pollW.Body).Decode(&pollResp))
	assert.Equal(t, "authorization_pending", pollResp["error"])
}

func TestDeviceFlow_AuthorizeAndPoll(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	// Seed remote-access service
	serviceID := uuid.NewString()
	require.NoError(t, database.CreateService(ctx, serviceID, "remote-access", "Remote Access", ""))
	require.NoError(t, database.EnableServiceForTenant(ctx, tenantID, serviceID))

	user := seedUserForDeviceFlow(t, database, tenantID)

	// Begin flow
	beginBody := `{"clientId":"conduit-cli"}`
	beginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/begin", strings.NewReader(beginBody))
	beginReq.Header.Set("Content-Type", "application/json")
	beginW := httptest.NewRecorder()
	srv.Router().ServeHTTP(beginW, beginReq)
	require.Equal(t, http.StatusOK, beginW.Code)

	var beginResp map[string]interface{}
	require.NoError(t, json.NewDecoder(beginW.Body).Decode(&beginResp))
	deviceCode := beginResp["deviceCode"].(string)
	userCode := beginResp["userCode"].(string)

	// Authorize — authenticated user approves the device
	token, _ := jwtMgr.IssueAccessToken(user.ID, tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})
	authBody := `{"userCode":"` + userCode + `"}`
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/authorize", strings.NewReader(authBody))
	authReq.Header.Set("Content-Type", "application/json")
	authReq.Header.Set("Authorization", "Bearer "+token)
	authW := httptest.NewRecorder()
	srv.Router().ServeHTTP(authW, authReq)

	assert.Equal(t, http.StatusOK, authW.Code)

	// Poll again — should now get tokens
	pollBody := `{"deviceCode":"` + deviceCode + `"}`
	pollReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/poll", strings.NewReader(pollBody))
	pollReq.Header.Set("Content-Type", "application/json")
	pollW := httptest.NewRecorder()
	srv.Router().ServeHTTP(pollW, pollReq)

	assert.Equal(t, http.StatusOK, pollW.Code)
	var pollResp map[string]interface{}
	require.NoError(t, json.NewDecoder(pollW.Body).Decode(&pollResp))
	assert.NotEmpty(t, pollResp["accessToken"])
	assert.NotEmpty(t, pollResp["refreshToken"])
	assert.Equal(t, "Bearer", pollResp["tokenType"])

	// Verify user info in response
	respUser := pollResp["user"].(map[string]interface{})
	assert.Equal(t, user.ID, respUser["id"])
	assert.Equal(t, "device-test@example.com", respUser["email"])
}

func TestDeviceFlow_AuthorizeUnauthenticated(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	body := `{"userCode":"ABCD-1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/authorize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, "authorize must require authentication")
}

func TestDeviceFlow_PollInvalidCode(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	body := `{"deviceCode":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/poll", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeviceFlow_AuthorizeInvalidUserCode(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"userCode":"INVALID"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/authorize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
