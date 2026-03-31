package protocol

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMuxPair creates a connected client/server mux pair for testing.
func setupMuxPair(t *testing.T) (client *Mux, server *Mux, cleanup func()) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Create a test HTTP server that upgrades to WebSocket
	serverConn := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("server accept: %v", err)
		}
		serverConn <- conn
	}))

	// Client dials
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, "ws"+ts.URL[4:], nil)
	require.NoError(t, err)

	srvConn := <-serverConn

	clientMux := NewMux(clientConn, logger)
	serverMux := NewMux(srvConn, logger)

	return clientMux, serverMux, func() {
		clientMux.Close()
		serverMux.Close()
		ts.Close()
	}
}

func TestMux_SendReceiveGlobal(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()

	// Start read loops
	go client.ReadLoop(ctx)
	go server.ReadLoop(ctx)

	// Client sends a HELLO frame to server
	hello := &Frame{
		Type:     FrameHello,
		StreamID: 0,
		Payload:  []byte(`{"agentId":"a1","hostname":"web-01"}`),
	}
	require.NoError(t, client.Send(ctx, hello))

	// Server receives on global channel
	select {
	case f := <-server.Global():
		assert.Equal(t, FrameHello, f.Type)
		assert.Equal(t, uint32(0), f.StreamID)
		assert.Contains(t, string(f.Payload), "web-01")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame")
	}
}

func TestMux_StreamRouting(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()

	// Register a stream on the server side
	streamCh := server.OpenStream(42)

	go client.ReadLoop(ctx)
	go server.ReadLoop(ctx)

	// Client sends a frame with StreamID 42
	f := &Frame{
		Type:     FrameShellData,
		StreamID: 42,
		Payload:  []byte("hello from stream 42"),
	}
	require.NoError(t, client.Send(ctx, f))

	// Server's stream 42 channel receives it
	select {
	case received := <-streamCh:
		assert.Equal(t, FrameShellData, received.Type)
		assert.Equal(t, uint32(42), received.StreamID)
		assert.Equal(t, []byte("hello from stream 42"), received.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stream frame")
	}
}

func TestMux_UnregisteredStreamGoesToGlobal(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()

	go client.ReadLoop(ctx)
	go server.ReadLoop(ctx)

	// Client sends a frame with StreamID 99 (not registered)
	f := &Frame{
		Type:     FrameShellData,
		StreamID: 99,
		Payload:  []byte("orphan frame"),
	}
	require.NoError(t, client.Send(ctx, f))

	// Server's global channel receives it
	select {
	case received := <-server.Global():
		assert.Equal(t, uint32(99), received.StreamID)
		assert.Equal(t, []byte("orphan frame"), received.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for global frame")
	}
}

func TestMux_CloseStream(t *testing.T) {
	_, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ch := server.OpenStream(10)
	assert.Equal(t, 1, server.StreamCount())

	server.CloseStream(10)
	assert.Equal(t, 0, server.StreamCount())

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)
}

func TestMux_NextStreamID(t *testing.T) {
	_, server, cleanup := setupMuxPair(t)
	defer cleanup()

	id1 := server.NextStreamID()
	id2 := server.NextStreamID()
	id3 := server.NextStreamID()

	assert.Equal(t, uint32(1), id1)
	assert.Equal(t, uint32(2), id2)
	assert.Equal(t, uint32(3), id3)
}

func TestMux_Bidirectional(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()

	// Both sides register stream 1
	clientCh := client.OpenStream(1)
	serverCh := server.OpenStream(1)

	go client.ReadLoop(ctx)
	go server.ReadLoop(ctx)

	// Client → Server
	require.NoError(t, client.Send(ctx, &Frame{Type: FrameShellData, StreamID: 1, Payload: []byte("from client")}))

	select {
	case f := <-serverCh:
		assert.Equal(t, []byte("from client"), f.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Server → Client
	require.NoError(t, server.Send(ctx, &Frame{Type: FrameShellData, StreamID: 1, Payload: []byte("from server")}))

	select {
	case f := <-clientCh:
		assert.Equal(t, []byte("from server"), f.Payload)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestMux_DoneClosedOnDisconnect(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()
	go server.ReadLoop(ctx)

	// Close client connection
	client.Close()

	// Server's Done channel should close
	select {
	case <-server.Done():
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Done")
	}
}

func TestMux_PingPong(t *testing.T) {
	client, server, cleanup := setupMuxPair(t)
	defer cleanup()

	ctx := context.Background()
	go client.ReadLoop(ctx)
	go server.ReadLoop(ctx)

	// Client sends PING
	require.NoError(t, client.Send(ctx, &Frame{Type: FramePing}))

	// Server receives on global (PING with StreamID 0)
	select {
	case f := <-server.Global():
		assert.Equal(t, FramePing, f.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Server sends PONG
	require.NoError(t, server.Send(ctx, &Frame{Type: FramePong}))

	select {
	case f := <-client.Global():
		assert.Equal(t, FramePong, f.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
