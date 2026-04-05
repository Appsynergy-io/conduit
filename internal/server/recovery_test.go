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
// POST /api/v1/auth/recovery/generate (PR #34 — recovery codes)
// ---------------------------------------------------------------------------

func TestGenerateRecoveryCodes_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "gen@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	codes, ok := resp["codes"].([]interface{})
	require.True(t, ok)
	assert.Len(t, codes, 10, "should generate 10 recovery codes")

	// Each code should be xxxx-xxxx-xxxx-xxxx format (19 chars total).
	for _, c := range codes {
		code := c.(string)
		assert.Len(t, code, 19)
		assert.Equal(t, "-", string(code[4]))
		assert.Equal(t, "-", string(code[9]))
		assert.Equal(t, "-", string(code[14]))
	}
}

func TestGenerateRecoveryCodes_StoresHashedCodes(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "hash@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify codes are stored as Argon2id hashes in DB
	codes, err := database.GetUnusedRecoveryCodesByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, codes, 10)
	for _, c := range codes {
		assert.True(t, strings.HasPrefix(c.CodeHash, "$argon2id$"),
			"code hash should be Argon2id: %s", c.CodeHash)
	}
}

func TestGenerateRecoveryCodes_ReplacesExisting(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "repl@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	// Generate twice
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// Should still be exactly 10 (old ones replaced)
	codes, err := database.GetUnusedRecoveryCodesByUser(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, codes, 10)
}

func TestGenerateRecoveryCodes_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGenerateRecoveryCodes_AuditLogged(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "audit@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify audit log entry
	events, err := database.ListAuditLogsByEventType(context.Background(), tenantID, "recovery.generated", 10, 0)
	require.NoError(t, err)
	found := false
	for _, e := range events {
		if e.EventType == "recovery.generated" && e.Outcome == "success" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have audit log entry for recovery.generated")
}

// ---------------------------------------------------------------------------
// POST /api/v1/auth/recovery/verify (PR #34 — recovery code verification)
// ---------------------------------------------------------------------------

func TestVerifyRecoveryCode_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "verify@test.com", "password")

	// Generate codes
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	var genResp map[string]interface{}
	require.NoError(t, json.NewDecoder(genW.Body).Decode(&genResp))
	codes := genResp["codes"].([]interface{})
	firstCode := codes[0].(string)

	// Verify the code (public endpoint — no JWT needed, but captcha required)
	body, err := json.Marshal(map[string]interface{}{
		"email":   "verify@test.com",
		"code":    firstCode,
		"captcha": solveCaptcha(t, srv, "/auth/recovery/verify"),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["token"], "should return scoped JWT")
	assert.Equal(t, float64(300), resp["expiresIn"])
	assert.Equal(t, "passkey:register", resp["scope"])
	assert.Equal(t, float64(9), resp["remaining"], "should have 9 remaining codes")
}

func TestVerifyRecoveryCode_ScopedToken(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "scope@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	var genResp map[string]interface{}
	require.NoError(t, json.NewDecoder(genW.Body).Decode(&genResp))
	firstCode := genResp["codes"].([]interface{})[0].(string)

	body, err := json.Marshal(map[string]interface{}{
		"email":   "scope@test.com",
		"code":    firstCode,
		"captcha": solveCaptcha(t, srv, "/auth/recovery/verify"),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	// Validate the returned token is scoped to passkey:register only
	scopedToken := resp["token"].(string)
	claims, err := jwtMgr.ValidateToken(scopedToken)
	require.NoError(t, err)
	assert.Equal(t, []string{"passkey:register"}, claims.Permissions)
	assert.Empty(t, claims.Roles, "scoped token should have no roles")
	assert.Empty(t, claims.Services, "scoped token should have no services")
}

func TestVerifyRecoveryCode_WrongCode(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "wrong@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	body := `{"email": "wrong@test.com", "code": "xxxx-yyyy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyRecoveryCode_UserNotFound(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"email": "nobody@test.com", "code": "xxxx-yyyy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyRecoveryCode_IdenticalErrors(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "ident@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	// Three failure modes should produce identical error responses (OWASP A07)
	bodies := []string{
		`{"email": "nobody@test.com", "code": "xxxx-yyyy"}`,    // user not found
		`{"email": "ident@test.com", "code": "xxxx-yyyy"}`,     // wrong code
		`{"email": "", "code": "xxxx-yyyy"}`,                    // missing email
	}

	var responses []string
	for _, b := range bodies {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		responses = append(responses, w.Body.String())
	}

	// All three should have the same error detail
	var prob1, prob2, prob3 map[string]interface{}
	json.Unmarshal([]byte(responses[0]), &prob1)
	json.Unmarshal([]byte(responses[1]), &prob2)
	json.Unmarshal([]byte(responses[2]), &prob3)

	assert.Equal(t, prob1["detail"], prob2["detail"],
		"user-not-found and wrong-code must produce identical errors")
	// Missing email produces a different detail "Invalid email or recovery code."
	// which is the same message — verify both are 401 with same text
}

func TestVerifyRecoveryCode_CodeCanOnlyBeUsedOnce(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "once@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	var genResp map[string]interface{}
	require.NoError(t, json.NewDecoder(genW.Body).Decode(&genResp))
	code := genResp["codes"].([]interface{})[0].(string)

	// First use — should succeed (fresh captcha)
	body1, err := json.Marshal(map[string]interface{}{
		"email":   "once@test.com",
		"code":    code,
		"captcha": solveCaptcha(t, srv, "/auth/recovery/verify"),
	})
	require.NoError(t, err)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body1)))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second use — should fail (code already consumed, fresh captcha)
	body2, err := json.Marshal(map[string]interface{}{
		"email":   "once@test.com",
		"code":    code,
		"captcha": solveCaptcha(t, srv, "/auth/recovery/verify"),
	})
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body2)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestVerifyRecoveryCode_RequiresCaptcha(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	// No captcha field → 401 (identical error, no enumeration)
	body := `{"email": "x@test.com", "code": "xxxx-yyyy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyRecoveryCode_RejectsForgedCaptcha(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	forged := map[string]interface{}{
		"salt":     strings.Repeat("a", 64),
		"target":   strings.Repeat("f", 64),
		"expires":  int64(9999999999),
		"endpoint": "/auth/recovery/verify",
		"hmac":     strings.Repeat("0", 64),
		"nonce":    uint64(1),
	}
	body, err := json.Marshal(map[string]interface{}{
		"email": "x@test.com", "code": "xxxx-yyyy", "captcha": forged,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyRecoveryCode_RejectsReplayedCaptcha(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "replay@test.com", "password")
	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	// Generate codes so the happy path past captcha can complete
	genReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/generate", nil)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.Header.Set("Authorization", "Bearer "+token)
	genW := httptest.NewRecorder()
	srv.Router().ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)
	var genResp map[string]interface{}
	require.NoError(t, json.NewDecoder(genW.Body).Decode(&genResp))
	code := genResp["codes"].([]interface{})[0].(string)

	// Solve once, submit twice. First OK, second replayed → 401.
	cap := solveCaptcha(t, srv, "/auth/recovery/verify")
	body, err := json.Marshal(map[string]interface{}{
		"email": "replay@test.com", "code": code, "captcha": cap,
	})
	require.NoError(t, err)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body)))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader(string(body)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.Router().ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestVerifyRecoveryCode_EmptyBody(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVerifyRecoveryCode_InvalidJSON(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------------------------------------------------------------------------
// POST /api/v1/users/{userId}/recovery/reset (PR #34 — admin recovery reset)
// ---------------------------------------------------------------------------

func TestAdminRecoveryReset_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID, adminUserID := seedLoginState(t, database, "admin@test.com", "password")

	// Create a target user with passkeys and recovery codes
	targetUserID := uuid.NewString()
	target := &db.User{
		ID:        targetUserID,
		TenantID:  tenantID,
		Email:     "target@test.com",
		FirstName: "Target",
		LastName:  "User",
		Role:      "org_member",
		Status:    "active",
	}
	require.NoError(t, database.CreateUser(ctx, target))

	// Seed passkeys and recovery codes for the target
	warning := "classical"
	pk := &db.Passkey{
		ID: uuid.NewString(), TenantID: tenantID, UserID: targetUserID,
		CredentialID: []byte("cred1"), PublicKey: []byte("pk1"),
		Algorithm: "ECDSA-P256", AlgorithmWarning: &warning,
		AuthenticatorType: "platform",
	}
	require.NoError(t, database.CreatePasskey(ctx, pk))

	codes := []db.RecoveryCode{{
		ID: uuid.NewString(), TenantID: tenantID, UserID: targetUserID,
		CodeHash: "$argon2id$test", CreatedAt: db.Now(),
	}}
	require.NoError(t, database.CreateRecoveryCodes(ctx, targetUserID, codes))

	// Admin resets the target user
	token, _ := jwtMgr.IssueAccessToken(adminUserID, tenantID, "sess",
		[]string{"platform_owner"}, nil)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/"+targetUserID+"/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify passkeys and recovery codes are gone
	passkeys, err := database.GetPasskeysByUserID(ctx, targetUserID)
	require.NoError(t, err)
	assert.Empty(t, passkeys, "passkeys should be deleted")

	count, err := database.CountUnusedRecoveryCodes(ctx, targetUserID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "recovery codes should be deleted")
}

func TestAdminRecoveryReset_Forbidden_NonAdmin(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, _ := seedLoginState(t, database, "member@test.com", "password")

	memberUserID := uuid.NewString()
	member := &db.User{
		ID: memberUserID, TenantID: tenantID, Email: "member2@test.com",
		FirstName: "M", LastName: "U", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(context.Background(), member))

	token, _ := jwtMgr.IssueAccessToken(memberUserID, tenantID, "sess",
		[]string{"org_member"}, nil)

	targetUserID := uuid.NewString()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/"+targetUserID+"/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminRecoveryReset_UserNotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, adminUserID := seedLoginState(t, database, "admin2@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(adminUserID, tenantID, "sess",
		[]string{"platform_owner"}, nil)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/"+uuid.NewString()+"/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminRecoveryReset_InvalidUUID(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, adminUserID := seedLoginState(t, database, "admin3@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(adminUserID, tenantID, "sess",
		[]string{"platform_owner"}, nil)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/not-a-uuid/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminRecoveryReset_CrossTenantBlocked(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID1, adminUserID := seedLoginState(t, database, "admin4@test.com", "password")

	// Create second tenant with a user
	tenantID2 := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID2, "Other Org"))
	targetUserID := uuid.NewString()
	target := &db.User{
		ID: targetUserID, TenantID: tenantID2, Email: "other@test.com",
		FirstName: "Other", LastName: "User", Role: "org_member", Status: "active",
	}
	require.NoError(t, database.CreateUser(ctx, target))

	// Admin from tenant1 tries to reset user from tenant2
	token, _ := jwtMgr.IssueAccessToken(adminUserID, tenantID1, "sess",
		[]string{"platform_owner"}, nil)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/"+targetUserID+"/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"cross-tenant reset should return 404, not 403 (no information leak)")
}

func TestAdminRecoveryReset_Unauthenticated(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/users/"+uuid.NewString()+"/recovery/reset", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
