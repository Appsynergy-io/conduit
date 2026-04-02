package server_test

import (
	"context"
	"encoding/json"
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

	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/server"
)

func TestEventBus_PublishToSubscribers(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	eb := server.NewEventBus(logger)

	assert.Equal(t, 0, eb.ClientCount())
}

func TestEventBus_ThrottleFirstEventPublishes(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond)

	// First event should publish immediately
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "agents", Type: "agent.connected",
		Data: map[string]string{"agentId": "a1", "hostname": "plex"},
	})

	readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
	_, data, err := conn.Read(readCtx)
	readCancel()
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "agent.connected", msg["type"])
}

func TestEventBus_ThrottleSuppressesRapidEvents(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond)

	// Fire 10 rapid connect/disconnect events for same agent
	for i := 0; i < 5; i++ {
		srv.PublishTestEvent("tenant-1", server.Event{
			Channel: "agents", Type: "agent.connected",
			Data: map[string]string{"agentId": "a1"},
		})
		srv.PublishTestEvent("tenant-1", server.Event{
			Channel: "agents", Type: "agent.disconnected",
			Data: map[string]string{"agentId": "a1"},
		})
	}

	// Should receive first event immediately
	readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
	_, data, err := conn.Read(readCtx)
	readCancel()
	require.NoError(t, err)

	var first map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &first))
	assert.Equal(t, "agent.connected", first["type"])

	// The remaining events within the throttle window should be suppressed.
	// After the throttle window (~5s), the final pending event should arrive.
	readCtx2, readCancel2 := context.WithTimeout(ctx, 7*time.Second)
	_, data2, err := conn.Read(readCtx2)
	readCancel2()
	require.NoError(t, err)

	var last map[string]interface{}
	require.NoError(t, json.Unmarshal(data2, &last))
	assert.Equal(t, "agent.disconnected", last["type"],
		"final pending event should be the last disconnected event")
}

func TestEventBus_ThrottleDoesNotAffectDifferentAgents(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond)

	// Agent A connected
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "agents", Type: "agent.connected",
		Data: map[string]string{"agentId": "a1"},
	})
	// Agent B connected — different agent, not throttled
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "agents", Type: "agent.connected",
		Data: map[string]string{"agentId": "a2"},
	})

	// Should receive both events (different agents, independent throttle keys)
	for i := 0; i < 2; i++ {
		readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
		_, _, err := conn.Read(readCtx)
		readCancel()
		require.NoError(t, err, "should receive event %d", i+1)
	}
}

func TestEventBus_ThrottleDoesNotAffectNonAgentEvents(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=shell"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond)

	// Fire 3 rapid shell events — should NOT be throttled
	for i := 0; i < 3; i++ {
		srv.PublishTestEvent("tenant-1", server.Event{
			Channel: "shell", Type: "shell.start",
			Data: map[string]string{"sessionId": "s1"},
		})
	}

	for i := 0; i < 3; i++ {
		readCtx, readCancel := context.WithTimeout(ctx, 1*time.Second)
		_, _, err := conn.Read(readCtx)
		readCancel()
		require.NoError(t, err, "shell event %d should not be throttled", i+1)
	}
}

func TestEventStream_NoToken(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	// Use httptest.Server so we can do WebSocket upgrade
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := websocket.Dial(ctx, url, nil)
	assert.Error(t, err, "should reject connection without token")
}

func TestEventStream_InvalidToken(t *testing.T) {
	srv, _, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?token=invalid.jwt.token"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := websocket.Dial(ctx, url, nil)
	assert.Error(t, err, "should reject invalid token")
}

func TestEventStream_ValidToken(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	// Use httpOnly cookie for authentication (no token in URL query params)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Cookie": []string{middleware.AuthCookieName + "=" + token},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.Close(websocket.StatusNormalClosure, "")
}

func TestEventStream_AuthHeader(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	defer conn.Close(websocket.StatusNormalClosure, "")
}

// ---------------------------------------------------------------------------
// Channel-based subscription tests
// ---------------------------------------------------------------------------

func TestEventStream_ChannelQuery(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	// Subscribe only to "agents" channel
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Give client time to register
	time.Sleep(50 * time.Millisecond)

	// Publish an agents event — should be received
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "agents",
		Type:    "agent.connected",
		Data:    map[string]string{"agentId": "a1"},
	})

	// Publish a shell event — should NOT be received
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "shell",
		Type:    "shell.start",
		Data:    map[string]string{"sessionId": "s1"},
	})

	// Read the first message — should be agents event
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "agents", msg["channel"])
	assert.Equal(t, "agent.connected", msg["type"])
}

func TestEventStream_DynamicSubscribe(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	// Connect with no channels (defaults to all)... actually let's start with just agents
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(50 * time.Millisecond)

	// Dynamically subscribe to "shell" channel
	subMsg, _ := json.Marshal(map[string]interface{}{
		"type":     "subscribe",
		"channels": []string{"shell"},
	})
	writeCtx, writeCancel := context.WithTimeout(ctx, 1*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, subMsg)
	writeCancel()
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Now publish a shell event — should be received
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "shell",
		Type:    "shell.start",
		Data:    map[string]string{"sessionId": "s1"},
	})

	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "shell", msg["channel"])
	assert.Equal(t, "shell.start", msg["type"])
}

func TestEventStream_DynamicUnsubscribe(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	// Connect with agents + shell channels
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents,shell"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(50 * time.Millisecond)

	// Unsubscribe from "shell"
	unsubMsg, _ := json.Marshal(map[string]interface{}{
		"type":     "unsubscribe",
		"channels": []string{"shell"},
	})
	writeCtx, writeCancel := context.WithTimeout(ctx, 1*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, unsubMsg)
	writeCancel()
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Publish an agents event — should be received
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "agents",
		Type:    "agent.connected",
		Data:    map[string]string{"agentId": "a1"},
	})

	// Publish a shell event — should NOT be received
	srv.PublishTestEvent("tenant-1", server.Event{
		Channel: "shell",
		Type:    "shell.start",
		Data:    map[string]string{"sessionId": "s1"},
	})

	// Read — should get agents event only
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "agents", msg["channel"])
}

func TestEventStream_PingPong(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Send ping
	pingMsg, _ := json.Marshal(map[string]string{"type": "ping"})
	writeCtx, writeCancel := context.WithTimeout(ctx, 1*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, pingMsg)
	writeCancel()
	require.NoError(t, err)

	// Read pong
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]string
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "pong", msg["type"])
}

func TestEventStream_TenantIsolation(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	tokenA, _ := jwtMgr.IssueAccessToken("user-a", "tenant-a", "sess-a", []string{"org_admin"}, nil)
	tokenB, _ := jwtMgr.IssueAccessToken("user-b", "tenant-b", "sess-b", []string{"org_admin"}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	baseURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream?channels=agents"

	connA, _, err := websocket.Dial(ctx, baseURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + tokenA}},
	})
	require.NoError(t, err)
	defer connA.Close(websocket.StatusNormalClosure, "")

	connB, _, err := websocket.Dial(ctx, baseURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + tokenB}},
	})
	require.NoError(t, err)
	defer connB.Close(websocket.StatusNormalClosure, "")

	time.Sleep(50 * time.Millisecond)

	// Publish event for tenant-a only
	srv.PublishTestEvent("tenant-a", server.Event{
		Channel: "agents",
		Type:    "agent.connected",
		Data:    map[string]string{"agentId": "a1"},
	})

	// Tenant A should receive it
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	_, data, err := connA.Read(readCtx)
	readCancel()
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "agent.connected", msg["type"])

	// Tenant B should NOT receive it (read should timeout)
	readCtxB, readCancelB := context.WithTimeout(ctx, 300*time.Millisecond)
	_, _, err = connB.Read(readCtxB)
	readCancelB()
	assert.Error(t, err, "tenant B should not receive tenant A's events")
}

func TestEventStream_InvalidMessage(t *testing.T) {
	srv, jwtMgr, _ := newTestServerWithDB(t, "dev")

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	token, err := jwtMgr.IssueAccessToken("user-1", "tenant-1", "sess-1", []string{"org_admin"}, nil)
	require.NoError(t, err)

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Send invalid JSON
	writeCtx, writeCancel := context.WithTimeout(ctx, 1*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, []byte("not json"))
	writeCancel()
	require.NoError(t, err)

	// Should get error response
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err)

	var msg map[string]string
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "error", msg["type"])
	assert.Contains(t, msg["message"], "invalid")
}
