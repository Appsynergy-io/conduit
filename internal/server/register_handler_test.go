package server_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

// createJoinTokenWithSecret creates a join token in the DB and returns the plaintext token.
func createJoinTokenWithSecret(t *testing.T, database *db.DB, tenantID, name, tokenType string, maxUses *int, expiresAt *string) (string, string) {
	t.Helper()

	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	plainToken := base64.RawURLEncoding.EncodeToString(secret)

	mac := hmac.New(sha256.New, []byte("conduit-join-token"))
	mac.Write(secret)
	tokenHash := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	tokenID := uuid.NewString()
	jt := &db.JoinToken{
		ID:        tokenID,
		TenantID:  tenantID,
		Type:      tokenType,
		Name:      name,
		TokenHash: tokenHash,
		MaxUses:   maxUses,
		ExpiresAt: expiresAt,
		Revoked:   0,
	}
	require.NoError(t, database.CreateJoinToken(context.Background(), jt))
	return tokenID, plainToken
}

func TestAgentRegister_Success(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	_, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Test Token", "persistent", nil, nil)

	body := `{"token":"` + plainToken + `","hostname":"web-01","os":"linux","arch":"amd64","version":"0.1.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["agentId"])
	assert.NotEmpty(t, resp["agentKey"])
	assert.Equal(t, tenantID, resp["tenantId"])

	// Verify agent was created in DB
	agent, err := database.GetAgentByID(ctx, resp["agentId"].(string))
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Equal(t, "web-01", agent.Hostname)
	assert.Equal(t, "offline", agent.Status)
}

func TestAgentRegister_InvalidToken(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	body := `{"token":"completely-invalid-token","hostname":"web-01","os":"linux","arch":"amd64"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentRegister_RevokedToken(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	tokenID, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Revoked", "persistent", nil, nil)
	require.NoError(t, database.RevokeJoinToken(ctx, tokenID))

	body := `{"token":"` + plainToken + `","hostname":"web-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentRegister_ExpiredToken(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	expired := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	_, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Expired", "persistent", nil, &expired)

	body := `{"token":"` + plainToken + `","hostname":"web-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentRegister_MaxUsesExhausted(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	maxUses := 1
	tokenID, plainToken := createJoinTokenWithSecret(t, database, tenantID, "One Use", "single_use", &maxUses, nil)

	// Manually set used_count to max
	require.NoError(t, database.IncrementTokenUsage(ctx, tokenID))

	body := `{"token":"` + plainToken + `","hostname":"web-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentRegister_SingleUseTokenRevoked(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	maxUses := 1
	tokenID, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Single Use", "single_use", &maxUses, nil)

	body := `{"token":"` + plainToken + `","hostname":"web-01","os":"linux","arch":"amd64"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify token was revoked
	tok, err := database.GetJoinTokenByID(ctx, tokenID)
	require.NoError(t, err)
	assert.Equal(t, 1, tok.Revoked)
}

func TestAgentRegister_InheritsLabels(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	// Create token with labels
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	require.NoError(t, err)
	plainToken := base64.RawURLEncoding.EncodeToString(secret)

	mac := hmac.New(sha256.New, []byte("conduit-join-token"))
	mac.Write(secret)
	tokenHash := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	labels := `{"env":"production","role":"web"}`
	jt := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "persistent",
		Name:      "Labeled",
		Labels:    &labels,
		TokenHash: tokenHash,
	}
	require.NoError(t, database.CreateJoinToken(ctx, jt))

	body := `{"token":"` + plainToken + `","hostname":"web-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	agent, err := database.GetAgentByID(ctx, resp["agentId"].(string))
	require.NoError(t, err)
	require.NotNil(t, agent.Labels)
	assert.Contains(t, *agent.Labels, "production")
	assert.Contains(t, *agent.Labels, "web")
}

func TestAgentRegister_MissingHostname(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	_, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Test", "persistent", nil, nil)

	body := `{"token":"` + plainToken + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentRegister_MissingToken(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	body := `{"hostname":"web-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentRegister_PersistentTokenReusable(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	_, plainToken := createJoinTokenWithSecret(t, database, tenantID, "Fleet", "persistent", nil, nil)

	// Register two agents with the same persistent token
	for _, hostname := range []string{"web-01", "web-02"} {
		body := `{"token":"` + plainToken + `","hostname":"` + hostname + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	}

	// Verify both agents exist
	agents, err := database.ListAgents(ctx, tenantID)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}
