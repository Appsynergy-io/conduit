package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

// mockMux is a minimal FrameMux implementation for testing power commands.
type mockMux struct {
	mu     sync.Mutex
	sent   []*protocol.Frame
	nextID uint32
	global chan *protocol.Frame
	done   chan struct{}
}

func newMockMux() *mockMux {
	return &mockMux{
		global: make(chan *protocol.Frame, 16),
		done:   make(chan struct{}),
	}
}

func (m *mockMux) Global() <-chan *protocol.Frame       { return m.global }
func (m *mockMux) Done() <-chan struct{}                 { return m.done }
func (m *mockMux) OpenStream(_ uint32) <-chan *protocol.Frame { return make(chan *protocol.Frame, 1) }
func (m *mockMux) CloseStream(_ uint32)                  {}
func (m *mockMux) ReadLoop(_ context.Context) error      { <-m.done; return nil }
func (m *mockMux) Close() error                          { return nil }
func (m *mockMux) StreamCount() int                      { return 0 }

func (m *mockMux) NextStreamID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	return m.nextID
}

func (m *mockMux) Send(_ context.Context, f *protocol.Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, f)
	return nil
}

func (m *mockMux) SentFrames() []*protocol.Frame {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*protocol.Frame{}, m.sent...)
}

func TestPowerAction_Reboot(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	// Register a mock connected agent
	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"action":"reboot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, agentID, resp["agentId"])
	assert.Equal(t, "reboot", resp["action"])
	assert.Equal(t, "sent", resp["status"])
}

func TestPowerAction_Poweroff(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"action":"poweroff"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "poweroff", resp["action"])
}

func TestPowerAction_InvalidAction(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"action":"restart"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPowerAction_AgentOffline(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "offline")
	// Do NOT register in agent registry — agent is offline

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"action":"reboot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPowerAction_NonAdmin(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	// viewer role, not admin
	token, _ := jwtMgr.IssueAccessToken("user", tenantID, "sess", []string{"viewer"}, []string{"remote-access"})

	body := `{"action":"reboot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPowerAction_AgentNotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	fakeID := uuid.NewString()
	body := `{"action":"reboot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+fakeID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPowerAction_AuditLog(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"action":"reboot"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/power", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	// Verify audit log was created
	events, err := database.ListAuditLogsByEventType(ctx, tenantID, "agent.power.reboot", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "agent.power.reboot", events[0].EventType)
	assert.Equal(t, "success", events[0].Outcome)
}
