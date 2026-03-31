package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.New(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func TestWebhookDelivery_Success(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var receivedBody []byte
	var receivedSignature string
	var receivedEvent string
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		receivedSignature = r.Header.Get("X-Conduit-Signature")
		receivedEvent = r.Header.Get("X-Conduit-Event")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()

	// Create a webhook subscription
	secretBytes := []byte("test-webhook-secret-key-12345678")
	secretHash := hex.EncodeToString(secretBytes)

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: secretHash,
		Events:     `["agent.connected"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.Start(ctx)
	defer wd.Stop()

	// Enqueue an event
	wd.Enqueue(ctx, testTenantID, "agent.connected", map[string]string{
		"agentId": "agent-123",
	})

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(1), callCount.Load())
	assert.Equal(t, "agent.connected", receivedEvent)

	// Verify the body is valid JSON
	var payload WebhookPayload
	require.NoError(t, json.Unmarshal(receivedBody, &payload))
	assert.Equal(t, "agent.connected", payload.EventType)
	assert.Equal(t, testTenantID, payload.TenantID)

	// Verify HMAC signature
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(receivedBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expected, receivedSignature)
}

func TestWebhookDelivery_Retry(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count <= 2 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["agent.connected"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	// Override the client timeout for faster test
	wd.client.Timeout = 2 * time.Second
	wd.Start(ctx)
	defer wd.Stop()

	wd.Enqueue(ctx, testTenantID, "agent.connected", map[string]string{"agentId": "a"})

	// Wait for retries (with fast retry in test)
	time.Sleep(15 * time.Second)

	// Should have been called at least 2 times (first attempt + retries)
	assert.GreaterOrEqual(t, callCount.Load(), int32(2))
}

func TestWebhookDelivery_NoSubscribers(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.Start(ctx)
	defer wd.Stop()

	// Should not panic with no subscribers
	wd.Enqueue(ctx, testTenantID, "agent.connected", map[string]string{"agentId": "a"})
	time.Sleep(100 * time.Millisecond)
}

func TestWebhookDelivery_DisabledSubscription(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()

	// Disabled subscription
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: "abc",
		Events:     `["agent.connected"]`,
		Enabled:    false,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.Start(ctx)
	defer wd.Stop()

	wd.Enqueue(ctx, testTenantID, "agent.connected", map[string]string{"agentId": "a"})
	time.Sleep(200 * time.Millisecond)

	// Should not be called since subscription is disabled
	assert.Equal(t, int32(0), callCount.Load())
}

func TestSignPayload(t *testing.T) {
	secretHash := hex.EncodeToString([]byte("test-secret"))
	body := []byte(`{"event":"test"}`)

	sig := signPayload(secretHash, body)
	assert.True(t, len(sig) > 10)
	assert.Contains(t, sig, "sha256=")

	// Verify the signature
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expected, sig)
}

// seedTestTenant creates the test tenant so FK constraints pass.
func seedTestTenant(t *testing.T, database *db.DB) {
	t.Helper()
	ctx := context.Background()
	database.SetTenantID(testTenantID)
	err := database.CreateTenant(ctx, testTenantID, "Test Tenant")
	require.NoError(t, err)
}

const testTenantID = "test-tenant-webhook"
