package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

func TestBuildWSURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		want      string
	}{
		{
			name:      "https to wss",
			serverURL: "https://conduit.example.com",
			want:      "wss://conduit.example.com/agent/v1/connect",
		},
		{
			name:      "http to ws",
			serverURL: "http://localhost:8443",
			want:      "ws://localhost:8443/agent/v1/connect",
		},
		{
			name:      "already wss",
			serverURL: "wss://conduit.example.com",
			want:      "wss://conduit.example.com/agent/v1/connect",
		},
		{
			name:      "bare hostname gets wss",
			serverURL: "conduit.example.com",
			want:      "wss://conduit.example.com/agent/v1/connect",
		},
		{
			name:      "trailing slash stripped",
			serverURL: "https://conduit.example.com/",
			want:      "wss://conduit.example.com/agent/v1/connect",
		},
		{
			name:      "with port",
			serverURL: "https://conduit.example.com:8443",
			want:      "wss://conduit.example.com:8443/agent/v1/connect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Agent{cfg: &Config{ServerURL: tt.serverURL}}
			assert.Equal(t, tt.want, a.buildWSURL())
		})
	}
}

func TestVersion(t *testing.T) {
	v := Version()
	assert.NotEmpty(t, v)
}

// TestHandshake_Success tests the full HELLO + AUTH handshake against a mock server.
func TestHandshake_Success(t *testing.T) {
	agentID := "test-agent-001"
	rawKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" // 64 hex chars = 32 bytes
	rawKeyBytes, err := hex.DecodeString(rawKey)
	require.NoError(t, err)
	keyHash := sha256.Sum256(rawKeyBytes)
	keyHashHex := hex.EncodeToString(keyHash[:])

	// Mock server that validates the handshake
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"conduit-cwp-v1"},
		})
		if err != nil {
			t.Errorf("websocket accept failed: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()

		// Read HELLO frame
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("reading HELLO: %v", err)
			return
		}
		helloFrame, err := protocol.DecodeBytes(data)
		if err != nil {
			t.Errorf("decoding HELLO: %v", err)
			return
		}
		assert.Equal(t, protocol.FrameHello, helloFrame.Type)

		var hello protocol.HelloPayload
		err = protocol.UnmarshalPayload(helloFrame.Payload, &hello)
		require.NoError(t, err)
		assert.Equal(t, agentID, hello.AgentID)

		// Read AUTH frame
		_, data, err = conn.Read(ctx)
		if err != nil {
			t.Errorf("reading AUTH: %v", err)
			return
		}
		authFrame, err := protocol.DecodeBytes(data)
		if err != nil {
			t.Errorf("decoding AUTH: %v", err)
			return
		}
		assert.Equal(t, protocol.FrameAuth, authFrame.Type)

		var auth protocol.AuthPayload
		err = protocol.UnmarshalPayload(authFrame.Payload, &auth)
		require.NoError(t, err)
		assert.Equal(t, agentID, auth.AgentID)

		// Verify HMAC signature
		storedHashBytes, _ := hex.DecodeString(keyHashHex)
		mac := hmac.New(sha256.New, storedHashBytes)
		mac.Write(helloFrame.Payload)
		mac.Write([]byte(auth.Nonce))
		expected := mac.Sum(nil)

		sigBytes, _ := hex.DecodeString(auth.Signature)
		assert.True(t, hmac.Equal(sigBytes, expected), "HMAC signature mismatch")

		// Send AUTH_OK
		okFrame := &protocol.Frame{Type: protocol.FrameAuthOK}
		okData, _ := protocol.EncodeBytes(okFrame)
		conn.Write(ctx, websocket.MessageBinary, okData)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg: &Config{
			ServerURL: srv.URL,
			AgentID:   agentID,
			AgentKey:  rawKey,
			TenantID:  "test-tenant",
		},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	ctx := context.Background()
	mux, _, err := a.connect(ctx)
	require.NoError(t, err)
	defer mux.Close()
}

// TestHandshake_AuthReject tests that the agent properly handles AUTH_REJECT.
func TestHandshake_AuthReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"conduit-cwp-v1"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()

		// Read HELLO
		conn.Read(ctx)
		// Read AUTH
		conn.Read(ctx)

		// Send AUTH_REJECT
		rejectFrame := &protocol.Frame{
			Type:    protocol.FrameAuthReject,
			Payload: []byte("bad credentials"),
		}
		data, _ := protocol.EncodeBytes(rejectFrame)
		conn.Write(ctx, websocket.MessageBinary, data)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg: &Config{
			ServerURL: srv.URL,
			AgentID:   "test-agent",
			AgentKey:  "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			TenantID:  "test-tenant",
		},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	ctx := context.Background()
	_, _, err := a.connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth rejected")
}
