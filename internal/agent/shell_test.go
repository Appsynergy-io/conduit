//go:build linux || darwin

package agent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

func TestStartShell_AndReceiveOutput(t *testing.T) {
	// Create a server that accepts a WebSocket, then we use the agent-side
	// mux directly to test startShell behavior.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{"conduit-cwp-v1"},
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		conn.SetReadLimit(protocol.MaxPayloadSize + protocol.HeaderSize)

		// Read all frames until the connection closes
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			frame, err := protocol.DecodeBytes(data)
			if err != nil {
				continue
			}

			// Echo back SHELL_DATA frames (to exercise the bridge)
			if frame.Type == protocol.FrameShellData {
				// Check for shell output containing our marker
				if strings.Contains(string(frame.Payload), "hello-conduit") {
					// Send exit command
					exitCmd := &protocol.Frame{
						Type:     protocol.FrameShellData,
						StreamID: frame.StreamID,
						Payload:  []byte("exit\n"),
					}
					exitData, _ := protocol.EncodeBytes(exitCmd)
					conn.Write(ctx, websocket.MessageBinary, exitData)
				}
			}
		}
	}))
	defer srv.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg: &Config{
			ServerURL: srv.URL,
			AgentID:   "test-agent",
			AgentKey:  "unused",
			TenantID:  "test-tenant",
		},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect to the mock server
	wsURL := a.buildWSURL()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"conduit-cwp-v1"},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	conn.SetReadLimit(protocol.MaxPayloadSize + protocol.HeaderSize)
	mux := protocol.NewMux(conn, logger)

	// Start read loop
	go mux.ReadLoop(ctx)

	streamID := uint32(1)
	payload := &protocol.ShellStartPayload{
		SessionID: "test-session-001",
		Shell:     "/bin/sh",
		Cols:      80,
		Rows:      24,
	}

	// Start shell on the agent side
	a.startShell(ctx, mux, streamID, payload)

	// Verify shell session was registered
	a.shellMu.Lock()
	_, exists := a.shells[streamID]
	a.shellMu.Unlock()
	assert.True(t, exists, "shell session should be registered")

	// Wait for shell to be ready, then send a command through the stream
	time.Sleep(300 * time.Millisecond)

	// Send echo command through the stream channel
	echoFrame := &protocol.Frame{
		Type:     protocol.FrameShellData,
		StreamID: streamID,
		Payload:  []byte("echo hello-conduit\n"),
	}
	err = mux.Send(ctx, echoFrame)
	require.NoError(t, err)

	// Wait for the echo to complete and the server to send exit
	time.Sleep(2 * time.Second)

	// Send exit directly to the shell's stream
	exitFrame := &protocol.Frame{
		Type:     protocol.FrameShellData,
		StreamID: streamID,
		Payload:  []byte("exit\n"),
	}
	err = mux.Send(ctx, exitFrame)
	require.NoError(t, err)

	// Wait for shell to terminate and clean up
	time.Sleep(2 * time.Second)

	// Verify cleanup
	a.shellMu.Lock()
	count := len(a.shells)
	a.shellMu.Unlock()
	assert.Equal(t, 0, count, "shell session should be cleaned up after exit")
}

func TestCleanupShells(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg:    &Config{ServerURL: "https://example.com", AgentID: "a", AgentKey: "b", TenantID: "c"},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	// Add mock sessions (with nil cmd/ptmx — the Close method handles this safely)
	_, cancel1 := context.WithCancel(context.Background())
	a.shells[1] = &shellSession{streamID: 1, cancel: cancel1}

	_, cancel2 := context.WithCancel(context.Background())
	a.shells[2] = &shellSession{streamID: 2, cancel: cancel2}

	assert.Len(t, a.shells, 2)

	a.cleanupShells()
	assert.Len(t, a.shells, 0)
}

func TestDefaultShell(t *testing.T) {
	shell := defaultShell()
	assert.NotEmpty(t, shell)
	assert.Contains(t, shell, "/")
}

func TestBuildShellEnv(t *testing.T) {
	env := buildShellEnv()
	assert.NotEmpty(t, env)

	hasTerm := false
	for _, e := range env {
		if hasPrefix(e, "TERM=") {
			hasTerm = true
			break
		}
	}
	assert.True(t, hasTerm, "TERM should be set in shell environment")
}

func TestResizeShell_UnknownStream(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := &Agent{
		cfg:    &Config{ServerURL: "https://example.com", AgentID: "a", AgentKey: "b", TenantID: "c"},
		logger: logger,
		shells: make(map[uint32]*shellSession),
	}

	// Should not panic when stream doesn't exist
	a.resizeShell(999, &protocol.ShellResizePayload{Cols: 120, Rows: 40})
}
