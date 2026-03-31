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

// seedLoginState creates tenant + user with password + enabled service for login tests.
func seedLoginState(t *testing.T, database *db.DB, email, password string) (tenantID, userID string) {
	t.Helper()
	ctx := context.Background()

	tenantID = uuid.NewString()
	err := database.CreateTenant(ctx, tenantID, "Test Org")
	require.NoError(t, err)
	database.SetTenantID(tenantID)

	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	userID = uuid.NewString()
	user := &db.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        email,
		FirstName:    "Test",
		LastName:     "User",
		Role:         "platform_owner",
		Status:       "active",
		PasswordHash: &hash,
	}
	err = database.CreateUser(ctx, user)
	require.NoError(t, err)

	serviceID := uuid.NewString()
	err = database.CreateService(ctx, serviceID, "remote-access", "Remote Access", "Secure remote access.")
	require.NoError(t, err)
	err = database.EnableServiceForTenant(ctx, tenantID, serviceID)
	require.NoError(t, err)

	return tenantID, userID
}

func TestPasswordLogin_Success(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "admin@test.com", "test-password-123")

	body := `{"email": "admin@test.com", "password": "test-password-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp["accessToken"], "must return access token")
	assert.NotEmpty(t, resp["refreshToken"], "must return refresh token")
	assert.Equal(t, "Bearer", resp["tokenType"])
	assert.Equal(t, float64(900), resp["expiresIn"])
}

func TestPasswordLogin_WrongPassword(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "admin@test.com", "correct-password")

	body := `{"email": "admin@test.com", "password": "wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var problem map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&problem)
	require.NoError(t, err)
	// Identical error message — no enumeration (OWASP A07)
	assert.Equal(t, "Invalid email or password.", problem["detail"])
}

func TestPasswordLogin_UserNotFound(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "admin@test.com", "password")

	body := `{"email": "nonexistent@test.com", "password": "password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var problem map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&problem)
	require.NoError(t, err)
	// Same error as wrong password — no user enumeration
	assert.Equal(t, "Invalid email or password.", problem["detail"])
}

func TestPasswordLogin_IdenticalErrors(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "admin@test.com", "password")

	// Capture responses for wrong password vs user not found
	bodies := []string{
		`{"email": "admin@test.com", "password": "wrong"}`,
		`{"email": "nobody@test.com", "password": "anything"}`,
	}

	var responses []string
	for _, b := range bodies {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		responses = append(responses, w.Body.String())
	}

	// Error responses must be identical to prevent enumeration (NIST AC-7, OWASP A07)
	assert.Equal(t, responses[0], responses[1],
		"wrong password and user-not-found must produce identical error responses")
}

func TestPasswordLogin_ProductionMode(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "production")
	seedLoginState(t, database, "admin@test.com", "password")

	body := `{"email": "admin@test.com", "password": "password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Password auth disabled in production — returns 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPasswordLogin_MissingEmail(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"password": "password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPasswordLogin_MissingPassword(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"email": "admin@test.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPasswordLogin_EmptyBody(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPasswordLogin_InvalidJSON(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPasswordLogin_CreatesSession(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	_, userID := seedLoginState(t, database, "admin@test.com", "password")

	body := `{"email": "admin@test.com", "password": "password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify session was created
	sessions, err := database.ListSessionsByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "web", sessions[0].Type)
}

func TestPasswordLogin_NoPasswordHash(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	err := database.CreateTenant(ctx, tenantID, "Test Org")
	require.NoError(t, err)
	database.SetTenantID(tenantID)

	// User without password hash (passkey-only user)
	user := &db.User{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Email:     "passkey@test.com",
		FirstName: "Passkey",
		LastName:  "User",
		Role:      "org_member",
		Status:    "active",
	}
	err = database.CreateUser(ctx, user)
	require.NoError(t, err)

	body := `{"email": "passkey@test.com", "password": "any-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Same error as wrong password — no enumeration
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var problem map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&problem)
	require.NoError(t, err)
	assert.Equal(t, "Invalid email or password.", problem["detail"])
}
