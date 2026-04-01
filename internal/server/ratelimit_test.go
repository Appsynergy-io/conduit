package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Rate limiting on public auth endpoints (OWASP A07, API2, API4)
// ---------------------------------------------------------------------------

func TestRateLimit_AuthEndpoints_EnforcedPerIP(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "rate@test.com", "password")

	// The auth rate limiter allows burst of 20 requests.
	// Send 20 requests — all should succeed (400 for bad creds is fine, not 429).
	for i := 0; i < 20; i++ {
		body := `{"email":"rate@test.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.20.30.40:12345"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.NotEqual(t, http.StatusTooManyRequests, w.Code,
			"request %d should not be rate limited (within burst)", i+1)
	}

	// Request 21 should be rate limited
	body := `{"email":"rate@test.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.20.30.40:12345"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code, "should be rate limited after burst exhausted")
	assert.Contains(t, w.Header().Get("Content-Type"), "application/problem+json")
}

func TestRateLimit_AuthEndpoints_DifferentIPsIndependent(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "rateip@test.com", "password")

	// Exhaust IP A's burst (20 requests)
	for i := 0; i < 20; i++ {
		body := `{"email":"rateip@test.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "1.1.1.1:9999"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// IP A is now limited
	body := `{"email":"rateip@test.com","password":"wrong"}`
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
		strings.NewReader(body))
	reqA.Header.Set("Content-Type", "application/json")
	reqA.RemoteAddr = "1.1.1.1:9999"
	wA := httptest.NewRecorder()
	srv.Router().ServeHTTP(wA, reqA)
	assert.Equal(t, http.StatusTooManyRequests, wA.Code, "IP A should be rate limited")

	// IP B should still work
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
		strings.NewReader(body))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.RemoteAddr = "2.2.2.2:9999"
	wB := httptest.NewRecorder()
	srv.Router().ServeHTTP(wB, reqB)
	assert.NotEqual(t, http.StatusTooManyRequests, wB.Code, "IP B should not be rate limited")
}

func TestRateLimit_WebAuthnLoginBegin_Enforced(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "wa@test.com", "password")

	// Exhaust burst on webauthn login endpoint
	for i := 0; i < 20; i++ {
		body := `{"email":"wa@test.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/begin",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "3.3.3.3:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// Should be rate limited
	body := `{"email":"wa@test.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/begin",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "3.3.3.3:1234"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_RecoveryVerify_Enforced(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "rec@test.com", "password")

	// Exhaust burst on recovery verify endpoint
	for i := 0; i < 20; i++ {
		body := `{"email":"rec@test.com","code":"1234-5678"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "4.4.4.4:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// Should be rate limited
	body := `{"email":"rec@test.com","code":"1234-5678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "4.4.4.4:1234"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_SetupEndpoints_StricterLimit(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	// Setup limiter has burst of 10. Exhaust it.
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "5.5.5.5:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// Should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "5.5.5.5:1234"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_AuthenticatedEndpoints_NotRateLimited(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	tenantID, userID := seedLoginState(t, database, "nolimit@test.com", "password")

	token, _ := jwtMgr.IssueAccessToken(userID, tenantID, "sess", []string{"org_member"}, nil)

	// Authenticated endpoints should not be affected by auth rate limiter
	for i := 0; i < 25; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "6.6.6.6:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.NotEqual(t, http.StatusTooManyRequests, w.Code,
			"authenticated endpoint should not be rate limited by auth limiter (request %d)", i+1)
	}
}

func TestRateLimit_SharedBudget_AcrossAuthEndpoints(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	seedLoginState(t, database, "shared@test.com", "password")

	// Use 10 requests on password login
	for i := 0; i < 10; i++ {
		body := `{"email":"shared@test.com","password":"wrong"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "7.7.7.7:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// Use 10 more on recovery verify (same IP, same limiter)
	for i := 0; i < 10; i++ {
		body := `{"email":"shared@test.com","code":"0000-0000"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery/verify",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "7.7.7.7:1234"
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
	}

	// 20 total used. Next request on any auth endpoint should be blocked.
	body := `{"email":"shared@test.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/login",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "7.7.7.7:1234"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code,
		"budget is shared across auth endpoints for the same IP")
}
