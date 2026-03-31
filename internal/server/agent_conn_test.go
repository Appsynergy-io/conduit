package server_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
	"github.com/appsynergy-io/conduit/internal/server"
)

// seedAgentWithKey creates an agent in the DB and returns the agentID and raw key bytes.
func seedAgentWithKey(t *testing.T, database *db.DB, tenantID, hostname string) (string, []byte) {
	t.Helper()
	agentID := uuid.NewString()

	// Generate a raw key
	rawKey := make([]byte, 32)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	// Hash with SHA-256 for storage (same as register handler)
	keyHash := sha256.Sum256(rawKey)
	agentKeyHash := hex.EncodeToString(keyHash[:])

	agent := &db.Agent{
		ID:           agentID,
		TenantID:     tenantID,
		Hostname:     hostname,
		AgentKeyHash: agentKeyHash,
		Status:       "offline",
	}
	require.NoError(t, database.CreateAgent(context.Background(), agent))
	return agentID, rawKey
}

// doAgentHandshake performs the CWP HELLO+AUTH handshake over a WebSocket.
// Returns the client-side mux and a cancel function. The caller must cancel
// when done to release resources; the connection stays alive until then.
func doAgentHandshake(t *testing.T, conn *websocket.Conn, agentID, hostname string, rawKey []byte) (*protocol.Mux, context.CancelFunc) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mux := protocol.NewMux(conn, logger)

	ctx, cancel := context.WithCancel(context.Background())

	go mux.ReadLoop(ctx)

	sendCtx, sendCancel := context.WithTimeout(ctx, 5*time.Second)
	defer sendCancel()

	// Send HELLO
	helloPayload := protocol.HelloPayload{
		AgentID:  agentID,
		Hostname: hostname,
		OS:       "linux",
		Arch:     "amd64",
		Version:  "0.1.0",
	}
	helloFrame, err := protocol.NewFrame(protocol.FrameHello, 0, helloPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(sendCtx, helloFrame))

	// Compute HMAC for AUTH: HMAC-SHA256(key=SHA256(rawKey), msg=helloPayload||nonce)
	nonce := uuid.NewString()
	keyHash := sha256.Sum256(rawKey)
	mac := hmac.New(sha256.New, keyHash[:])
	mac.Write(helloFrame.Payload)
	mac.Write([]byte(nonce))
	sig := hex.EncodeToString(mac.Sum(nil))

	authPayload := protocol.AuthPayload{
		AgentID:   agentID,
		Signature: sig,
		Nonce:     nonce,
	}
	authFrame, err := protocol.NewFrame(protocol.FrameAuth, 0, authPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(sendCtx, authFrame))

	// Wait for AUTH_OK or AUTH_REJECT
	select {
	case f := <-mux.Global():
		assert.Equal(t, protocol.FrameAuthOK, f.Type, "expected AUTH_OK, got %s", f.Type.String())
	case <-sendCtx.Done():
		cancel()
		t.Fatal("timeout waiting for AUTH response")
	}

	return mux, cancel
}

func TestAgentConnect_Success(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID, rawKey := seedAgentWithKey(t, database, tenantID, "web-01")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/agent/v1/connect"
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)

	_, cancelMux := doAgentHandshake(t, conn, agentID, "web-01", rawKey)
	defer cancelMux()

	// Poll DB until agent is online (server handler runs async)
	var agent *db.Agent
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		agent, err = database.GetAgentByID(ctx, agentID)
		require.NoError(t, err)
		if agent.Status == "online" {
			break
		}
	}

	assert.Equal(t, "online", agent.Status)
	require.NotNil(t, agent.Transport)
	assert.Equal(t, "websocket", *agent.Transport)
}

func TestAgentConnect_InvalidAgent(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/agent/v1/connect"
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mux := protocol.NewMux(conn, logger)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	go mux.ReadLoop(readCtx)

	// Send HELLO with unknown agent
	helloPayload := protocol.HelloPayload{
		AgentID:  uuid.NewString(),
		Hostname: "unknown",
	}
	helloFrame, err := protocol.NewFrame(protocol.FrameHello, 0, helloPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(readCtx, helloFrame))

	// Send AUTH with bogus signature
	authPayload := protocol.AuthPayload{
		AgentID:   helloPayload.AgentID,
		Signature: "deadbeef",
		Nonce:     "nonce",
	}
	authFrame, err := protocol.NewFrame(protocol.FrameAuth, 0, authPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(readCtx, authFrame))

	// Should get AUTH_REJECT
	select {
	case f := <-mux.Global():
		assert.Equal(t, protocol.FrameAuthReject, f.Type)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for AUTH_REJECT")
	}
}

func TestAgentConnect_WrongKey(t *testing.T) {
	srv, _, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID, _ := seedAgentWithKey(t, database, tenantID, "web-01")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	wsURL := "ws" + ts.URL[4:] + "/agent/v1/connect"
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()

	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mux := protocol.NewMux(conn, logger)

	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()
	go mux.ReadLoop(readCtx)

	// Send HELLO
	helloPayload := protocol.HelloPayload{AgentID: agentID, Hostname: "web-01"}
	helloFrame, err := protocol.NewFrame(protocol.FrameHello, 0, helloPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(readCtx, helloFrame))

	// Send AUTH with wrong key
	wrongKey := make([]byte, 32)
	wrongKeyHash := sha256.Sum256(wrongKey)
	mac := hmac.New(sha256.New, wrongKeyHash[:])
	mac.Write(helloFrame.Payload)
	mac.Write([]byte("nonce"))
	sig := hex.EncodeToString(mac.Sum(nil))

	authPayload := protocol.AuthPayload{
		AgentID:   agentID,
		Signature: sig,
		Nonce:     "nonce",
	}
	authFrame, err := protocol.NewFrame(protocol.FrameAuth, 0, authPayload)
	require.NoError(t, err)
	require.NoError(t, mux.Send(readCtx, authFrame))

	select {
	case f := <-mux.Global():
		assert.Equal(t, protocol.FrameAuthReject, f.Type)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for AUTH_REJECT")
	}
}

func TestVerifyAgentAuth(t *testing.T) {
	// Generate a raw key
	rawKey := make([]byte, 32)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	// Store SHA-256 hash
	keyHash := sha256.Sum256(rawKey)
	storedHash := hex.EncodeToString(keyHash[:])

	// Compute signature using the same protocol as the agent would
	helloPayload := []byte(`{"agentId":"abc","hostname":"web-01"}`)
	nonce := "test-nonce-123"

	mac := hmac.New(sha256.New, keyHash[:])
	mac.Write(helloPayload)
	mac.Write([]byte(nonce))
	sig := hex.EncodeToString(mac.Sum(nil))

	// Verify should pass
	assert.True(t, server.VerifyAgentAuthForTesting(storedHash, helloPayload, nonce, sig))

	// Wrong signature should fail
	assert.False(t, server.VerifyAgentAuthForTesting(storedHash, helloPayload, nonce, "deadbeef"))

	// Wrong nonce should fail
	assert.False(t, server.VerifyAgentAuthForTesting(storedHash, helloPayload, "wrong-nonce", sig))
}
