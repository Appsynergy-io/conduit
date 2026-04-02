package server_test

import (
	"bytes"
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

// autoRespondMux is a FrameMux that auto-responds to FILE_WRITE and FILE_STAT frames.
type autoRespondMux struct {
	mu       sync.Mutex
	sent     []*protocol.Frame
	nextID   uint32
	streams  map[uint32]chan *protocol.Frame
	global   chan *protocol.Frame
	done     chan struct{}
	fileSize int64 // reported size for FILE_STAT responses
}

func newAutoRespondMux() *autoRespondMux {
	return &autoRespondMux{
		streams: make(map[uint32]chan *protocol.Frame),
		global:  make(chan *protocol.Frame, 16),
		done:    make(chan struct{}),
	}
}

func (m *autoRespondMux) Global() <-chan *protocol.Frame       { return m.global }
func (m *autoRespondMux) Done() <-chan struct{}                 { return m.done }
func (m *autoRespondMux) ReadLoop(_ context.Context) error      { <-m.done; return nil }
func (m *autoRespondMux) Close() error                          { return nil }
func (m *autoRespondMux) StreamCount() int                      { return len(m.streams) }

func (m *autoRespondMux) NextStreamID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	return m.nextID
}

func (m *autoRespondMux) OpenStream(id uint32) <-chan *protocol.Frame {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *protocol.Frame, 1)
	m.streams[id] = ch
	return ch
}

func (m *autoRespondMux) CloseStream(id uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, id)
}

func (m *autoRespondMux) Send(_ context.Context, f *protocol.Frame) error {
	m.mu.Lock()
	m.sent = append(m.sent, f)
	ch := m.streams[f.StreamID]
	m.mu.Unlock()

	if ch == nil {
		return nil
	}

	// Auto-respond based on frame type
	switch f.Type {
	case protocol.FrameFileWrite:
		var req protocol.FileWriteRequest
		protocol.UnmarshalPayload(f.Payload, &req)
		resp := protocol.FileWriteResponse{
			Path: req.Path,
			Size: int64(len(req.Content)),
		}
		respFrame, _ := protocol.NewFrame(protocol.FrameFileWrite, f.StreamID, resp)
		ch <- respFrame

	case protocol.FrameFileStat:
		m.mu.Lock()
		sz := m.fileSize
		m.mu.Unlock()
		resp := protocol.FileStatResponse{
			Path: "/tmp/test.bin",
			Type: "file",
			Size: sz,
		}
		respFrame, _ := protocol.NewFrame(protocol.FrameFileStat, f.StreamID, resp)
		ch <- respFrame
	}

	return nil
}

func TestInitUpload_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"path":"/tmp/test.bin","totalSize":10485760}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/uploads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotEmpty(t, resp["uploadId"])
	assert.Equal(t, float64(4194304), resp["chunkSize"]) // 4MB
}

func TestInitUpload_AgentOffline(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	seedAgent(t, database, tenantID, "web-01", "offline")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"path":"/tmp/test.bin","totalSize":1024}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+uuid.NewString()+"/uploads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Agent not found or not connected
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusServiceUnavailable)
}

func TestInitUpload_InvalidTotalSize(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	body := `{"path":"/tmp/test.bin","totalSize":-1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/uploads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUploadStatus_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Init upload
	initBody := `{"path":"/tmp/test.bin","totalSize":1048576}`
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/uploads", strings.NewReader(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Authorization", "Bearer "+token)
	initW := httptest.NewRecorder()
	srv.Router().ServeHTTP(initW, initReq)
	require.Equal(t, http.StatusCreated, initW.Code)

	var initResp map[string]interface{}
	require.NoError(t, json.NewDecoder(initW.Body).Decode(&initResp))
	uploadID := initResp["uploadId"].(string)

	// Check status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/uploads/"+uploadID, nil)
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusW := httptest.NewRecorder()
	srv.Router().ServeHTTP(statusW, statusReq)

	assert.Equal(t, http.StatusOK, statusW.Code)

	var statusResp map[string]interface{}
	require.NoError(t, json.NewDecoder(statusW.Body).Decode(&statusResp))
	assert.Equal(t, uploadID, statusResp["uploadId"])
	assert.Equal(t, float64(0), statusResp["offset"])
	assert.Equal(t, float64(1048576), statusResp["totalSize"])
}

func TestUploadStatus_NotFound(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/uploads/"+uuid.NewString(), nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCancelUpload(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newMockMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Init upload
	initBody := `{"path":"/tmp/test.bin","totalSize":1024}`
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/uploads", strings.NewReader(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Authorization", "Bearer "+token)
	initW := httptest.NewRecorder()
	srv.Router().ServeHTTP(initW, initReq)
	require.Equal(t, http.StatusCreated, initW.Code)

	var initResp map[string]interface{}
	json.NewDecoder(initW.Body).Decode(&initResp)
	uploadID := initResp["uploadId"].(string)

	// Cancel
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+agentID+"/uploads/"+uploadID, nil)
	delReq.Header.Set("Content-Type", "application/json")
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	srv.Router().ServeHTTP(delW, delReq)

	assert.Equal(t, http.StatusOK, delW.Code)

	// Verify it's gone
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/uploads/"+uploadID, nil)
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusW := httptest.NewRecorder()
	srv.Router().ServeHTTP(statusW, statusReq)

	assert.Equal(t, http.StatusNotFound, statusW.Code)
}

func TestUploadChunk_Success(t *testing.T) {
	srv, jwtMgr, database := newTestServerWithDB(t, "dev")
	ctx := context.Background()

	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	agentID := seedAgent(t, database, tenantID, "web-01", "online")

	mux := newAutoRespondMux()
	srv.RegisterAgentForTesting(agentID, tenantID, "web-01", mux)
	t.Cleanup(func() { srv.UnregisterAgentForTesting(agentID) })

	token, _ := jwtMgr.IssueAccessToken("admin", tenantID, "sess", []string{"org_admin"}, []string{"remote-access"})

	// Init upload (1KB total)
	initBody := `{"path":"/tmp/test.bin","totalSize":1024}`
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/uploads", strings.NewReader(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Authorization", "Bearer "+token)
	initW := httptest.NewRecorder()
	srv.Router().ServeHTTP(initW, initReq)
	require.Equal(t, http.StatusCreated, initW.Code)

	var initResp map[string]interface{}
	json.NewDecoder(initW.Body).Decode(&initResp)
	uploadID := initResp["uploadId"].(string)

	// Send a 512-byte chunk
	chunk := make([]byte, 512)
	for i := range chunk {
		chunk[i] = byte(i % 256)
	}

	chunkReq := httptest.NewRequest(http.MethodPatch,
		"/api/v1/agents/"+agentID+"/uploads/"+uploadID,
		bytes.NewReader(chunk))
	chunkReq.Header.Set("Content-Type", "application/octet-stream")
	chunkReq.Header.Set("Authorization", "Bearer "+token)
	chunkW := httptest.NewRecorder()
	srv.Router().ServeHTTP(chunkW, chunkReq)

	assert.Equal(t, http.StatusOK, chunkW.Code)

	var chunkResp map[string]interface{}
	require.NoError(t, json.NewDecoder(chunkW.Body).Decode(&chunkResp))
	assert.Equal(t, float64(512), chunkResp["offset"])
}

