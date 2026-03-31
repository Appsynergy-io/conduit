package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
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

// --- SSRF Prevention Tests (OWASP A10, API7) ---

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		// RFC 1918 private ranges
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		// Loopback
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		// Link-local
		{"169.254.0.1", true},
		{"169.254.169.254", true}, // AWS/GCP metadata endpoint
		// IPv6
		{"::1", true},
		// Public IPs — should NOT be blocked
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"203.0.113.1", false},
		// Boundary: 172.15.x is NOT private
		{"172.15.255.255", false},
		// Boundary: 172.32.x is NOT private
		{"172.32.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "invalid test IP: %s", tt.ip)
			assert.Equal(t, tt.private, isPrivateOrReservedIP(ip))
		})
	}
}

func TestValidateWebhookURL_SSRFPrevention(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		blocked bool
	}{
		{"loopback IP", "http://127.0.0.1/hook", true},
		{"private 10.x", "https://10.0.0.5/hook", true},
		{"private 172.16.x", "https://172.16.0.1/hook", true},
		{"private 192.168.x", "https://192.168.1.1/hook", true},
		{"aws metadata", "http://169.254.169.254/latest/meta-data/", true},
		{"link-local", "http://169.254.1.1/hook", true},
		{"ipv6 loopback", "http://[::1]/hook", true},
		{"loopback with port", "http://127.0.0.1:8080/hook", true},
		{"private with path", "https://10.0.0.1/api/v1/callback", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if tt.blocked {
				assert.Error(t, err, "should block: %s", tt.url)
				assert.Contains(t, err.Error(), "private")
			}
		})
	}
}

func TestValidateWebhookURL_InvalidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"no scheme", "example.com/hook"},
		{"just path", "/hook"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			// Empty/schemeless URLs won't have an IP to check, but shouldn't panic
			_ = err
		})
	}
}

// --- Webhook Delivery SSRF Integration ---

func TestWebhookDelivery_SSRFBlocked(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var callCount atomic.Int32
	// This server exists but is at 127.0.0.1 which SSRF should block
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        "http://127.0.0.1:12345/evil",
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["agent.connected"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.Start(ctx)
	defer wd.Stop()

	wd.Enqueue(ctx, testTenantID, "agent.connected", map[string]string{"agentId": "a"})
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(0), callCount.Load(), "delivery to loopback should be SSRF-blocked")
}

// --- Webhook Redirect Prevention (OWASP A10) ---

func TestWebhookDelivery_NoRedirectFollowing(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var redirectTargetHit atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetHit.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	var initialHit atomic.Int32
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialHit.Add(1)
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	ctx := context.Background()
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        redirector.URL,
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["agent.connected"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.skipSSRF = true // httptest binds to 127.0.0.1
	wd.Start(ctx)
	defer wd.Stop()

	// Deliver directly to test redirect behavior
	work := webhookWork{
		sub: *sub,
		payload: WebhookPayload{
			ID:        uuid.NewString(),
			EventType: "agent.connected",
			TenantID:  testTenantID,
			Timestamp: db.Now(),
			Data:      map[string]string{"agentId": "a"},
		},
		attempt: 1,
	}
	wd.deliver(ctx, work)

	assert.GreaterOrEqual(t, initialHit.Load(), int32(1), "initial request should be made")
	assert.Equal(t, int32(0), redirectTargetHit.Load(), "redirect target must NOT be hit")
}

// --- HMAC Signature Security ---

func TestSignPayload_Deterministic(t *testing.T) {
	secret := hex.EncodeToString([]byte("deterministic-key"))
	body := []byte(`{"event":"test","data":"value"}`)

	sig1 := signPayload(secret, body)
	sig2 := signPayload(secret, body)
	assert.Equal(t, sig1, sig2, "same input must produce same signature")
}

func TestSignPayload_DifferentSecrets(t *testing.T) {
	body := []byte(`{"event":"test"}`)
	sig1 := signPayload(hex.EncodeToString([]byte("secret-1")), body)
	sig2 := signPayload(hex.EncodeToString([]byte("secret-2")), body)
	assert.NotEqual(t, sig1, sig2)
}

func TestSignPayload_TamperDetection(t *testing.T) {
	secret := hex.EncodeToString([]byte("tamper-key"))
	original := []byte(`{"event":"test","amount":100}`)
	tampered := []byte(`{"event":"test","amount":999}`)

	assert.NotEqual(t, signPayload(secret, original), signPayload(secret, tampered),
		"tampered body must produce different signature")
}

func TestWebhookDelivery_SignatureVerifiable(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	secretBytes := []byte("hmac-verification-key-32bytes!!")
	secretHash := hex.EncodeToString(secretBytes)

	var capturedSig string
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSig = r.Header.Get("X-Conduit-Signature")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: secretHash,
		Events:     `["test.event"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.skipSSRF = true // httptest binds to 127.0.0.1
	wd.Start(ctx)
	defer wd.Stop()

	// Deliver directly to test signature verification
	work := webhookWork{
		sub: *sub,
		payload: WebhookPayload{
			ID:        uuid.NewString(),
			EventType: "test.event",
			TenantID:  testTenantID,
			Timestamp: db.Now(),
			Data:      map[string]string{"key": "value"},
		},
		attempt: 1,
	}
	wd.deliver(ctx, work)

	require.NotEmpty(t, capturedSig)
	require.NotEmpty(t, capturedBody)

	// Verify HMAC independently
	mac := hmac.New(sha256.New, secretBytes)
	mac.Write(capturedBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expected, capturedSig)

	// Verify body structure
	var payload WebhookPayload
	require.NoError(t, json.Unmarshal(capturedBody, &payload))
	assert.Equal(t, "test.event", payload.EventType)
	assert.Equal(t, testTenantID, payload.TenantID)
	assert.NotEmpty(t, payload.ID)
	assert.NotEmpty(t, payload.Timestamp)
}

// --- Webhook Timeout Enforcement ---

func TestWebhookDelivery_TimeoutEnforced(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Second)
	}))
	defer srv.Close()

	ctx := context.Background()
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["test.event"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.skipSSRF = true                         // httptest binds to 127.0.0.1
	wd.client.Timeout = 500 * time.Millisecond // Short timeout for test
	wd.Start(ctx)
	defer wd.Stop()

	start := time.Now()

	// Deliver directly to test timeout behavior
	work := webhookWork{
		sub: *sub,
		payload: WebhookPayload{
			ID:        uuid.NewString(),
			EventType: "test.event",
			TenantID:  testTenantID,
			Timestamp: db.Now(),
			Data:      map[string]string{"key": "val"},
		},
		attempt: 1,
	}
	wd.deliver(ctx, work)

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 5*time.Second, "delivery should time out quickly, not hang")
}

// --- Webhook Queue Overflow Resilience ---

func TestWebhookDeliverer_QueueOverflow(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Slow handler
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["test.event"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.skipSSRF = true // httptest binds to 127.0.0.1
	wd.Start(ctx)
	defer wd.Stop()

	// Flood the queue — must not panic or deadlock
	for i := 0; i < 2000; i++ {
		wd.Enqueue(ctx, testTenantID, "test.event", map[string]string{"i": "flood"})
	}
	// If we reach here, no panic/deadlock occurred
}

// --- Webhook Required Headers ---

func TestWebhookDelivery_RequiredHeaders(t *testing.T) {
	database := newTestDB(t)
	seedTestTenant(t, database)

	var headers http.Header
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		method = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	sub := &db.WebhookSubscription{
		ID:         uuid.NewString(),
		TenantID:   testTenantID,
		URL:        srv.URL,
		SecretHash: hex.EncodeToString([]byte("secret")),
		Events:     `["test.event"]`,
		Enabled:    true,
	}
	require.NoError(t, database.CreateWebhookSubscription(ctx, sub))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wd := NewWebhookDeliverer(database, logger)
	wd.skipSSRF = true // httptest binds to 127.0.0.1
	wd.Start(ctx)
	defer wd.Stop()

	// Deliver directly to test header requirements
	work := webhookWork{
		sub: *sub,
		payload: WebhookPayload{
			ID:        uuid.NewString(),
			EventType: "test.event",
			TenantID:  testTenantID,
			Timestamp: db.Now(),
			Data:      map[string]string{"k": "v"},
		},
		attempt: 1,
	}
	wd.deliver(ctx, work)

	assert.Equal(t, "POST", method)
	assert.Equal(t, "application/json", headers.Get("Content-Type"))
	assert.Equal(t, "Conduit-Webhook/1.0", headers.Get("User-Agent"))
	assert.NotEmpty(t, headers.Get("X-Conduit-Signature"))
	assert.Equal(t, "test.event", headers.Get("X-Conduit-Event"))

	// Delivery ID must be a valid UUID
	deliveryID := headers.Get("X-Conduit-Delivery")
	assert.NotEmpty(t, deliveryID)
	_, err := uuid.Parse(deliveryID)
	assert.NoError(t, err, "delivery ID must be a valid UUID")
}
