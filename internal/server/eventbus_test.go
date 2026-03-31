package server_test

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

	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/server"
)

func TestEventBus_PublishToSubscribers(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	eb := server.NewEventBus(logger)

	assert.Equal(t, 0, eb.ClientCount())
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
