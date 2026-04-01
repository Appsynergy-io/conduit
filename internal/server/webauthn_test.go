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

// seedPasskeyForUser creates a passkey in the DB for testing list/delete endpoints.
func seedPasskeyForUser(t *testing.T, database *db.DB, tenantID, userID string) string {
	t.Helper()
	warning := "classical algorithm used: hardware authenticator selected non-PQC algorithm"
	pk := &db.Passkey{
		ID:                uuid.NewString(),
		TenantID:          tenantID,
		UserID:            userID,
		CredentialID:      []byte("cred-" + uuid.NewString()[:8]),
		PublicKey:         []byte("pk-" + uuid.NewString()[:8]),
		Algorithm:         "ECDSA-P256",
		AlgorithmWarning:  &warning,
		AuthenticatorType: "platform",
		SignCount:         0,
	}
	require.NoError(t, database.CreatePasskey(context.Background(), pk))
	return pk.ID
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/webauthn/credentials (PR #34 — list passkeys)
// ---------------------------------------------------------------------------

func TestListPasskeys_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "list@test.com", "password")

	seedPasskeyForUser(t, database, tenantID, userID)
	seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil)
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

	// Verify response shape
	item := items[0].(map[string]interface{})
	assert.NotEmpty(t, item["id"])
	assert.NotEmpty(t, item["algorithm"])
	assert.NotEmpty(t, item["authenticatorType"])
	assert.NotEmpty(t, item["createdAt"])
}

func TestListPasskeys_Empty(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "empty@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(0), resp["total"])
}

func TestListPasskeys_OnlyOwnPasskeys(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "own@test.com", "password")

	// Create another user with passkeys
	otherUserID := uuid.NewString()
	other := &db.User{
		ID: otherUserID, TenantID: tenantID, Email: "other@test.com",
		FirstName: "O", LastName: "U", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(context.Background(), other))

	seedPasskeyForUser(t, database, tenantID, userID)
	seedPasskeyForUser(t, database, tenantID, otherUserID)
	seedPasskeyForUser(t, database, tenantID, otherUserID)

	// User should only see their own passkey
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(1), resp["total"],
		"user should only see their own passkeys, not others'")
}

func TestListPasskeys_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListPasskeys_DoesNotExposeCredentialID(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "noexp@test.com", "password")
	seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/webauthn/credentials", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	items := resp["items"].([]interface{})
	item := items[0].(map[string]interface{})
	// Should not expose raw credential ID or public key (security)
	assert.Nil(t, item["credentialId"])
	assert.Nil(t, item["publicKey"])
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/auth/webauthn/credentials/{credentialId} (PR #34 — delete passkey)
// ---------------------------------------------------------------------------

func TestDeletePasskey_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "del@test.com", "password")

	pkID := seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+pkID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify passkey is gone
	passkeys, err := database.GetPasskeysByUserID(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, passkeys)
}

func TestDeletePasskey_BOLA_OtherUserPasskey(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "bola@test.com", "password")

	// Create another user's passkey
	otherUserID := uuid.NewString()
	other := &db.User{
		ID: otherUserID, TenantID: tenantID, Email: "victim@test.com",
		FirstName: "V", LastName: "U", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(context.Background(), other))
	otherPkID := seedPasskeyForUser(t, database, tenantID, otherUserID)

	// Try to delete another user's passkey
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+otherPkID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"deleting another user's passkey must return 404, not 403 (BOLA prevention)")

	// Verify the passkey still exists
	passkeys, err := database.GetPasskeysByUserID(context.Background(), otherUserID)
	require.NoError(t, err)
	assert.Len(t, passkeys, 1, "victim's passkey must not be deleted")
}

func TestDeletePasskey_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "nf@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeletePasskey_LastPasskey_ProductionMode(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "production")
	tenantID, userID := seedLoginState(t, database, "last@test.com", "password")
	pkID := seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+pkID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"should not allow deleting the last passkey in production mode")

	var problem map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&problem))
	assert.Contains(t, problem["detail"], "only passkey")
}

func TestDeletePasskey_LastPasskey_DevMode_Allowed(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "lastdev@test.com", "password")
	pkID := seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+pkID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code,
		"dev mode should allow deleting the last passkey")
}

func TestDeletePasskey_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeletePasskey_AuditLogged(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "auditdel@test.com", "password")
	pkID := seedPasskeyForUser(t, database, tenantID, userID)

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/auth/webauthn/credentials/"+pkID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	// Verify audit log entry
	events, err := database.ListAuditLogsByEventType(context.Background(), tenantID, "auth.passkey.deleted", 10, 0)
	require.NoError(t, err)
	found := false
	for _, e := range events {
		if e.EventType == "auth.passkey.deleted" && e.Outcome == "success" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have audit log entry for auth.passkey.deleted")
}

// ---------------------------------------------------------------------------
// WebAuthn register/login begin — WebAuthn not configured (PR #34)
// ---------------------------------------------------------------------------

func TestWebAuthnRegisterBegin_NotConfigured(t *testing.T) {
	// Default test server has domain "localhost" and port ":0" which may or
	// may not initialize WebAuthn. If it's nil the handler returns 503.
	// This test verifies the handler gracefully handles the nil case.
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "noreg@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/register/begin", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// May be 503 (not configured) or 200 (configured on localhost)
	// Both are acceptable — we just verify no panic/500
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
}

func TestWebAuthnLoginBegin_EmptyEmail(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"email": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/begin",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should be 401 (no user enumeration) or 503 (not configured)
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusServiceUnavailable}, w.Code)
}

func TestWebAuthnLoginBegin_UserNotFound_NoEnumeration(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "exists@test.com", "password")

	// Both existing and non-existing users should get the same error
	bodies := []string{
		`{"email": "exists@test.com"}`,
		`{"email": "nobody@test.com"}`,
	}

	var statusCodes []int
	for _, b := range bodies {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/begin",
			strings.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		statusCodes = append(statusCodes, w.Code)
	}

	// Both should return the same status (401 or 503)
	assert.Equal(t, statusCodes[0], statusCodes[1],
		"existing and non-existing users must get identical responses (no enumeration)")
}
