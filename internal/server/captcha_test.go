package server_test

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/captcha"
	"github.com/appsynergy-io/conduit/internal/server"
)

// solveCaptcha fetches a challenge for the given endpoint and solves it.
// Returns the Solution map (as decoded JSON) for embedding in request bodies.
// Uses the default server difficulty (~262k iterations, ~50ms in Go).
func solveCaptcha(t *testing.T, srv *server.Server, endpoint string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/captcha/challenge?endpoint="+endpoint, nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "challenge request failed: %s", w.Body.String())

	var c captcha.Challenge
	require.NoError(t, json.NewDecoder(w.Body).Decode(&c))

	salt, err := hex.DecodeString(c.Salt)
	require.NoError(t, err)
	target, err := hex.DecodeString(c.Target)
	require.NoError(t, err)

	for nonce := uint64(0); nonce < 1<<30; nonce++ {
		h := sha256.New()
		h.Write(salt)
		var nb [8]byte
		binary.BigEndian.PutUint64(nb[:], nonce)
		h.Write(nb[:])
		sum := h.Sum(nil)
		less := false
		for i := 0; i < 32; i++ {
			if sum[i] < target[i] {
				less = true
				break
			}
			if sum[i] > target[i] {
				break
			}
		}
		if less {
			return map[string]interface{}{
				"salt": c.Salt, "target": c.Target, "expires": c.Expires,
				"endpoint": c.Endpoint, "hmac": c.HMAC, "nonce": nonce,
			}
		}
	}
	t.Fatalf("captcha solve exhausted")
	return nil
}

// ---------------------------------------------------------------------------
// GET /api/v1/auth/captcha/challenge
// ---------------------------------------------------------------------------

func TestCaptchaChallenge_Success(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/captcha/challenge?endpoint=/auth/recovery/verify", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var c captcha.Challenge
	require.NoError(t, json.NewDecoder(w.Body).Decode(&c))
	assert.Len(t, c.Salt, 64)
	assert.Len(t, c.Target, 64)
	assert.Len(t, c.HMAC, 64)
	assert.Equal(t, "/auth/recovery/verify", c.Endpoint)
	assert.Greater(t, c.Expires, int64(0))
}

func TestCaptchaChallenge_MissingEndpointParam(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/captcha/challenge", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCaptchaChallenge_RejectsArbitraryEndpoint(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")
	// Endpoint is valid format but not in the captcha-gated allowlist.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/captcha/challenge?endpoint=/auth/password/login", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCaptchaChallenge_RejectsMalformedEndpoint(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/captcha/challenge?endpoint=../../etc/passwd", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCaptchaChallenge_IssuesFreshEachCall(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")
	salts := make(map[string]bool)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/auth/captcha/challenge?endpoint=/auth/recovery/verify", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var c captcha.Challenge
		require.NoError(t, json.NewDecoder(w.Body).Decode(&c))
		assert.False(t, salts[c.Salt], "salt repeated")
		salts[c.Salt] = true
	}
}
