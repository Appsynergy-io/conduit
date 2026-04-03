package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	ws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func smTestLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func smTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func smSeedTenant(t *testing.T, d *db.DB) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, d.CreateTenant(context.Background(), id, "Test Org"))
	d.SetTenantID(id)
	return id
}

func smSeedAgent(t *testing.T, d *db.DB, tenantID string) *db.Agent {
	t.Helper()
	a := &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Hostname:     "test-host",
		AgentKeyHash: "deadbeef",
	}
	require.NoError(t, d.CreateAgent(context.Background(), a))
	return a
}

func smSeedUser(t *testing.T, d *db.DB, tenantID string) *db.User {
	t.Helper()
	u := &db.User{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Email:     "smtest@example.com",
		FirstName: "Test",
		LastName:  "User",
		Role:      "org_admin",
		Status:    "active",
	}
	require.NoError(t, d.CreateUser(context.Background(), u))
	return u
}

// wsTestPair creates a client/server WebSocket pair for testing.
// Returns a connected Mux (server-side) and the test server.
func wsTestPair(t *testing.T) (*protocol.Mux, *httptest.Server) {
	t.Helper()

	muxReady := make(chan *protocol.Mux, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			return
		}
		m := protocol.NewMux(conn, smTestLogger())
		muxReady <- m
		// Keep the handler open until test ends — ReadLoop blocks
		m.ReadLoop(r.Context())
	}))
	t.Cleanup(srv.Close)

	// Dial from client side
	clientConn, _, err := ws.Dial(context.Background(), "ws://"+srv.Listener.Addr().String(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { clientConn.Close(ws.StatusNormalClosure, "") })

	// Start client-side read loop to drain messages
	go func() {
		for {
			_, _, err := clientConn.Read(context.Background())
			if err != nil {
				return
			}
		}
	}()

	mux := <-muxReady
	return mux, srv
}

// ---------------------------------------------------------------------------
// LiveSession unit tests
// ---------------------------------------------------------------------------

func TestLiveSession_IsDetached(t *testing.T) {
	ls := &LiveSession{clients: make(map[string]*SessionClient)}
	assert.True(t, ls.IsDetached(), "should be detached when no clients connected")

	// Add a client to simulate attached state
	ls.mu.Lock()
	ls.clients["test"] = &SessionClient{ID: "test", Role: roleController}
	ls.mu.Unlock()

	assert.False(t, ls.IsDetached(), "should not be detached when clients are connected")
}

func TestLiveSession_DetachedSince_NotDetached(t *testing.T) {
	ls := &LiveSession{}
	// detachedAt is nil → should return 0
	assert.Equal(t, time.Duration(0), ls.DetachedSince())
}

func TestLiveSession_DetachedSince_Detached(t *testing.T) {
	past := time.Now().Add(-5 * time.Minute)
	ls := &LiveSession{
		detachedAt: &past,
	}
	dur := ls.DetachedSince()
	assert.True(t, dur >= 5*time.Minute-time.Second, "should be at least ~5 minutes")
}

// ---------------------------------------------------------------------------
// SessionManager: Get / List
// ---------------------------------------------------------------------------

func TestSessionManager_GetNil(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	assert.Nil(t, sm.Get("nonexistent"))
}

func TestSessionManager_ListEmpty(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	assert.Empty(t, sm.List())
}

func TestSessionManager_GetAndList(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	ls := &LiveSession{
		ID:      "sess-1",
		AgentID: "agent-1",
		UserID:  "user-1",
	}

	sm.mu.Lock()
	sm.sessions["sess-1"] = ls
	sm.mu.Unlock()

	got := sm.Get("sess-1")
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.ID)

	list := sm.List()
	assert.Len(t, list, 1)
	assert.Equal(t, "sess-1", list[0].ID)
}

// ---------------------------------------------------------------------------
// SessionManager: UpdateSession
// ---------------------------------------------------------------------------

func TestSessionManager_UpdateSession(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	agent := smSeedAgent(t, d, tenantID)
	user := smSeedUser(t, d, tenantID)
	ctx := context.Background()

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	// Create DB record that UpdateSession will modify
	sess := &db.ShellSession{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		Status:      "active",
		Recording:   1,
		Pinned:      0,
		IdleTimeout: 3600,
	}
	require.NoError(t, d.CreateShellSession(ctx, sess))

	ls := &LiveSession{
		ID:          sess.ID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		TenantID:    tenantID,
		Pinned:      false,
		IdleTimeout: time.Hour,
	}

	sm.mu.Lock()
	sm.sessions[ls.ID] = ls
	sm.mu.Unlock()

	// Update pin
	pinTrue := true
	err := sm.UpdateSession(ctx, ls, &pinTrue, nil)
	require.NoError(t, err)
	assert.True(t, ls.Pinned)

	// Update idle timeout
	newTimeout := 1800
	err = sm.UpdateSession(ctx, ls, nil, &newTimeout)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, ls.IdleTimeout)

	// Verify DB was updated
	dbSess, err := d.GetShellSessionByID(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, dbSess.Pinned)
	assert.Equal(t, 1800, dbSess.IdleTimeout)
}

// ---------------------------------------------------------------------------
// SessionManager: reapIdleSessions
// ---------------------------------------------------------------------------

func TestSessionManager_ReapIdleSessions_ReapsExpired(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	agent := smSeedAgent(t, d, tenantID)
	user := smSeedUser(t, d, tenantID)
	ctx := context.Background()

	mux, _ := wsTestPair(t)

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	// Create DB record
	sessID := uuid.NewString()
	require.NoError(t, d.CreateShellSession(ctx, &db.ShellSession{
		ID:          sessID,
		TenantID:    tenantID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		Status:      "detached",
		Recording:   0,
		Pinned:      0,
		IdleTimeout: 60, // 1 minute
	}))

	// Detached 2 minutes ago — should be reaped
	detachedAt := time.Now().Add(-2 * time.Minute)
	sessionCancel := make(chan struct{})
	ls := &LiveSession{
		ID:          sessID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		TenantID:    tenantID,
		StreamID:    1,
		Pinned:      false,
		IdleTimeout: time.Minute,
		Ring:        NewRingBuffer(256),
		detachedAt:  &detachedAt,
		connAgent: &ConnectedAgent{
			AgentID: agent.ID,
			Mux:     mux,
		},
		cancel: func() { close(sessionCancel) },
	}

	sm.mu.Lock()
	sm.sessions[sessID] = ls
	sm.mu.Unlock()

	sm.reapIdleSessions(ctx)

	// Verify cancel was called (TerminateSession fires cancel + CloseStream).
	// Map removal happens asynchronously in onSessionExit via agentReadLoop,
	// which is not running in this unit test. We verify the cancel signal instead.
	select {
	case <-sessionCancel:
		// good — cancel was called, session will be reaped
	default:
		t.Fatal("expected session cancel to be called during reap")
	}
}

func TestSessionManager_ReapIdleSessions_SkipsPinned(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	ctx := context.Background()

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	detachedAt := time.Now().Add(-2 * time.Hour)
	ls := &LiveSession{
		ID:          uuid.NewString(),
		AgentID:     "agent-1",
		UserID:      "user-1",
		TenantID:    tenantID,
		Pinned:      true, // pinned — should NOT be reaped
		IdleTimeout: time.Minute,
		detachedAt:  &detachedAt,
	}

	sm.mu.Lock()
	sm.sessions[ls.ID] = ls
	sm.mu.Unlock()

	sm.reapIdleSessions(ctx)

	// Should still be present
	assert.NotNil(t, sm.Get(ls.ID), "pinned session should not be reaped")
}

func TestSessionManager_ReapIdleSessions_SkipsAttached(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	ls := &LiveSession{
		ID:          uuid.NewString(),
		AgentID:     "agent-1",
		UserID:      "user-1",
		TenantID:    "tenant-1",
		Pinned:      false,
		IdleTimeout: time.Minute,
		detachedAt:  nil, // attached — detachedAt is nil
	}

	sm.mu.Lock()
	sm.sessions[ls.ID] = ls
	sm.mu.Unlock()

	sm.reapIdleSessions(context.Background())

	assert.NotNil(t, sm.Get(ls.ID), "attached session should not be reaped")
}

func TestSessionManager_ReapIdleSessions_SkipsNotExpired(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	// Detached only 10 seconds ago with a 1-hour timeout
	detachedAt := time.Now().Add(-10 * time.Second)
	ls := &LiveSession{
		ID:          uuid.NewString(),
		AgentID:     "agent-1",
		UserID:      "user-1",
		TenantID:    "tenant-1",
		Pinned:      false,
		IdleTimeout: time.Hour,
		detachedAt:  &detachedAt,
	}

	sm.mu.Lock()
	sm.sessions[ls.ID] = ls
	sm.mu.Unlock()

	sm.reapIdleSessions(context.Background())

	assert.NotNil(t, sm.Get(ls.ID), "session within idle timeout should not be reaped")
}

// ---------------------------------------------------------------------------
// SessionManager: CleanupAgentSessions
// ---------------------------------------------------------------------------

func TestSessionManager_CleanupAgentSessions(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	agent := smSeedAgent(t, d, tenantID)
	user := smSeedUser(t, d, tenantID)
	ctx := context.Background()

	mux, _ := wsTestPair(t)

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	cancelled := make([]string, 0)
	var mu sync.Mutex

	// Create two sessions for the same agent
	for i := 0; i < 2; i++ {
		sessID := uuid.NewString()
		require.NoError(t, d.CreateShellSession(ctx, &db.ShellSession{
			ID:          sessID,
			TenantID:    tenantID,
			AgentID:     agent.ID,
			UserID:      user.ID,
			Status:      "active",
			Recording:   0,
			Pinned:      0,
			IdleTimeout: 3600,
		}))

		id := sessID // capture for closure
		ls := &LiveSession{
			ID:        sessID,
			AgentID:   agent.ID,
			UserID:    user.ID,
			TenantID:  tenantID,
			StreamID:  uint32(i + 1),
			Ring:      NewRingBuffer(256),
			connAgent: &ConnectedAgent{AgentID: agent.ID, Mux: mux},
			cancel: func() {
				mu.Lock()
				cancelled = append(cancelled, id)
				mu.Unlock()
			},
		}
		sm.mu.Lock()
		sm.sessions[sessID] = ls
		sm.mu.Unlock()
	}

	// Create one session for a different agent — should NOT be cleaned
	otherSessID := uuid.NewString()
	otherLS := &LiveSession{
		ID:       otherSessID,
		AgentID:  "other-agent",
		UserID:   user.ID,
		TenantID: tenantID,
	}
	sm.mu.Lock()
	sm.sessions[otherSessID] = otherLS
	sm.mu.Unlock()

	assert.Len(t, sm.List(), 3)

	sm.CleanupAgentSessions(ctx, agent.ID)

	// Verify both cancels were called (map removal is async via onSessionExit)
	mu.Lock()
	assert.Len(t, cancelled, 2, "both agent sessions should have cancel() called")
	mu.Unlock()

	// The other-agent session should still be in the manager
	assert.NotNil(t, sm.Get(otherSessID), "session for other agent should not be cleaned up")
}

// ---------------------------------------------------------------------------
// SessionManager: CreateSession (integration — requires Mux)
// ---------------------------------------------------------------------------

func TestSessionManager_CreateSession(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	agent := smSeedAgent(t, d, tenantID)
	user := smSeedUser(t, d, tenantID)
	ctx := context.Background()

	mux, _ := wsTestPair(t)

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	connAgent := &ConnectedAgent{
		AgentID:  agent.ID,
		TenantID: tenantID,
		Hostname: agent.Hostname,
		Mux:      mux,
	}

	ls, err := sm.CreateSession(ctx, connAgent, agent, user.ID, tenantID, 80, 24, false, 3600, true)
	require.NoError(t, err)
	require.NotNil(t, ls)

	assert.NotEmpty(t, ls.ID)
	assert.Equal(t, agent.ID, ls.AgentID)
	assert.Equal(t, user.ID, ls.UserID)
	assert.Equal(t, tenantID, ls.TenantID)
	assert.False(t, ls.Pinned)
	assert.Equal(t, time.Hour, ls.IdleTimeout)
	assert.NotNil(t, ls.Ring)
	assert.NotNil(t, ls.Recorder, "recording=true should create recorder")
	assert.True(t, ls.IsDetached(), "starts without browser attached")

	// Should be in the manager
	assert.NotNil(t, sm.Get(ls.ID))
	assert.Len(t, sm.List(), 1)

	// Verify DB record
	dbSess, err := d.GetShellSessionByID(ctx, ls.ID)
	require.NoError(t, err)
	require.NotNil(t, dbSess)
	assert.Equal(t, "active", dbSess.Status)
	assert.Equal(t, 1, dbSess.Recording)
	assert.Equal(t, 0, dbSess.Pinned)
	assert.Equal(t, 3600, dbSess.IdleTimeout)

	// Clean up
	sm.TerminateSession(ctx, ls)
}

func TestSessionManager_CreateSession_NilRecorder(t *testing.T) {
	d := smTestDB(t)
	tenantID := smSeedTenant(t, d)
	agent := smSeedAgent(t, d, tenantID)
	user := smSeedUser(t, d, tenantID)
	ctx := context.Background()

	mux, _ := wsTestPair(t)

	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))
	defer sm.Stop()

	connAgent := &ConnectedAgent{
		AgentID:  agent.ID,
		TenantID: tenantID,
		Hostname: agent.Hostname,
		Mux:      mux,
	}

	ls, err := sm.CreateSession(ctx, connAgent, agent, user.ID, tenantID, 80, 24, true, 1800, false)
	require.NoError(t, err)
	assert.True(t, ls.Pinned)
	assert.Equal(t, 30*time.Minute, ls.IdleTimeout)
	assert.Nil(t, ls.Recorder, "recording=false should not create recorder")

	sm.TerminateSession(ctx, ls)
}

// ---------------------------------------------------------------------------
// SessionManager: Stop
// ---------------------------------------------------------------------------

func TestSessionManager_Stop(t *testing.T) {
	d := smTestDB(t)
	sm := NewSessionManager(d, smTestLogger(), NewEventBus(smTestLogger()))

	// Stop should not panic and can be called multiple times safely
	sm.Stop()
	sm.Stop()
}
