package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

const (
	// defaultChunkSize is the recommended chunk size for uploads (4MB).
	defaultChunkSize = 4 << 20

	// maxUploadSize is the maximum total upload size (1GB).
	maxUploadSize = 1 << 30

	// uploadSessionTTL is how long an idle upload session lives before expiry.
	uploadSessionTTL = 1 * time.Hour
)

// uploadSession tracks the state of a resumable upload.
type uploadSession struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agentId"`
	TenantID  string    `json:"tenantId"`
	Path      string    `json:"path"`
	TotalSize int64     `json:"totalSize"`
	Offset    int64     `json:"offset"`
	Mode      string    `json:"mode"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Checksum  string    `json:"checksum,omitempty"` // final SHA-256
	mu        sync.Mutex
}

// uploadStore manages active upload sessions.
type uploadStore struct {
	sessions sync.Map // uploadID → *uploadSession
}

func newUploadStore() *uploadStore {
	return &uploadStore{}
}

func (us *uploadStore) Get(id string) *uploadSession {
	v, ok := us.sessions.Load(id)
	if !ok {
		return nil
	}
	return v.(*uploadSession)
}

func (us *uploadStore) Put(s *uploadSession) {
	us.sessions.Store(s.ID, s)
}

func (us *uploadStore) Delete(id string) {
	us.sessions.Delete(id)
}

// CleanExpired removes expired upload sessions.
func (us *uploadStore) CleanExpired() {
	now := time.Now()
	us.sessions.Range(func(key, value interface{}) bool {
		s := value.(*uploadSession)
		if now.After(s.ExpiresAt) {
			us.sessions.Delete(key)
		}
		return true
	})
}

// initUploadRequest is the request body for POST /api/v1/agents/{agentId}/uploads.
type initUploadRequest struct {
	Path      string `json:"path"`
	TotalSize int64  `json:"totalSize"`
	Mode      string `json:"mode,omitempty"`
}

// initUploadResponse is the response for creating an upload session.
type initUploadResponse struct {
	UploadID  string `json:"uploadId"`
	ChunkSize int64  `json:"chunkSize"`
	ExpiresAt string `json:"expiresAt"`
}

// uploadStatusResponse is the response for checking upload progress.
type uploadStatusResponse struct {
	UploadID  string `json:"uploadId"`
	Path      string `json:"path"`
	TotalSize int64  `json:"totalSize"`
	Offset    int64  `json:"offset"`
	ExpiresAt string `json:"expiresAt"`
}

// uploadCompleteResponse is the response after final assembly.
type uploadCompleteResponse struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// handleInitUpload creates a resumable upload session.
// POST /api/v1/agents/{agentId}/uploads
func (s *Server) handleInitUpload(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	_, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}
	_ = connAgent // verified agent is connected

	var req initUploadRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", nil)
		return
	}

	if req.Path == "" {
		apierror.BadRequest(w, r, "Path is required.", nil)
		return
	}
	if req.TotalSize <= 0 {
		apierror.BadRequest(w, r, "totalSize must be positive.", nil)
		return
	}
	if req.TotalSize > maxUploadSize {
		apierror.BadRequest(w, r, fmt.Sprintf("File exceeds maximum upload size (%d bytes).", maxUploadSize), nil)
		return
	}

	uploadID := uuid.NewString()
	sess := &uploadSession{
		ID:        uploadID,
		AgentID:   agentID,
		TenantID:  claims.TenantID,
		Path:      sanitizeFilePath(req.Path),
		TotalSize: req.TotalSize,
		Offset:    0,
		Mode:      req.Mode,
		CreatedBy: claims.Subject,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(uploadSessionTTL),
	}

	s.uploads.Put(sess)

	writeJSON(w, http.StatusCreated, initUploadResponse{
		UploadID:  uploadID,
		ChunkSize: defaultChunkSize,
		ExpiresAt: sess.ExpiresAt.Format(time.RFC3339),
	})
}

// handleUploadChunk receives a chunk and forwards it to the agent.
// PATCH /api/v1/agents/{agentId}/uploads/{uploadId}
func (s *Server) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	uploadID := chi.URLParam(r, "uploadId")
	claims := middleware.ClaimsFromCtx(r.Context())

	sess := s.uploads.Get(uploadID)
	if sess == nil {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	// Verify ownership
	if sess.TenantID != claims.TenantID || sess.AgentID != agentID {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	if time.Now().After(sess.ExpiresAt) {
		s.uploads.Delete(uploadID)
		apierror.BadRequest(w, r, "Upload session expired.", nil)
		return
	}

	_, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	// Read chunk body (limited to defaultChunkSize + overhead)
	body := http.MaxBytesReader(w, r.Body, defaultChunkSize+1024)
	data, err := io.ReadAll(body)
	if err != nil {
		apierror.BadRequest(w, r, "Failed to read chunk data.", nil)
		return
	}

	if len(data) == 0 {
		apierror.BadRequest(w, r, "Empty chunk.", nil)
		return
	}

	sess.mu.Lock()
	currentOffset := sess.Offset
	isFirstChunk := currentOffset == 0
	sess.mu.Unlock()

	// Would this chunk exceed the declared total size?
	if currentOffset+int64(len(data)) > sess.TotalSize {
		apierror.BadRequest(w, r, "Chunk would exceed declared total size.", nil)
		return
	}

	// Send chunk to agent via FILE_WRITE with offset
	writeReq := protocol.FileWriteRequest{
		Path:     sess.Path,
		Content:  base64.StdEncoding.EncodeToString(data),
		Mode:     sess.Mode,
		Offset:   currentOffset,
		Truncate: isFirstChunk,
		UploadID: uploadID,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileWrite, writeReq)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var writeResp protocol.FileWriteResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &writeResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if writeResp.Error != "" {
		apierror.BadRequest(w, r, writeResp.Error, nil)
		return
	}

	// Update offset
	sess.mu.Lock()
	sess.Offset = currentOffset + int64(len(data))
	sess.ExpiresAt = time.Now().Add(uploadSessionTTL) // refresh TTL
	newOffset := sess.Offset
	sess.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"uploadId":  uploadID,
		"offset":    newOffset,
		"totalSize": sess.TotalSize,
	})
}

// handleUploadStatus returns the current state of an upload session.
// GET /api/v1/agents/{agentId}/uploads/{uploadId}
func (s *Server) handleUploadStatus(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	uploadID := chi.URLParam(r, "uploadId")
	claims := middleware.ClaimsFromCtx(r.Context())

	sess := s.uploads.Get(uploadID)
	if sess == nil {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	if sess.TenantID != claims.TenantID || sess.AgentID != agentID {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	sess.mu.Lock()
	offset := sess.Offset
	sess.mu.Unlock()

	writeJSON(w, http.StatusOK, uploadStatusResponse{
		UploadID:  sess.ID,
		Path:      sess.Path,
		TotalSize: sess.TotalSize,
		Offset:    offset,
		ExpiresAt: sess.ExpiresAt.Format(time.RFC3339),
	})
}

// handleCompleteUpload finalizes a resumable upload.
// POST /api/v1/agents/{agentId}/uploads/{uploadId}/complete
func (s *Server) handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	uploadID := chi.URLParam(r, "uploadId")
	claims := middleware.ClaimsFromCtx(r.Context())

	sess := s.uploads.Get(uploadID)
	if sess == nil {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	if sess.TenantID != claims.TenantID || sess.AgentID != agentID {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	sess.mu.Lock()
	offset := sess.Offset
	sess.mu.Unlock()

	if offset != sess.TotalSize {
		apierror.BadRequest(w, r,
			fmt.Sprintf("Upload incomplete: %d of %d bytes received.", offset, sess.TotalSize), nil)
		return
	}

	// Optional: parse expected checksum from request body
	var completeReq struct {
		Checksum string `json:"checksum,omitempty"` // expected SHA-256
	}
	// Ignore decode errors — checksum is optional
	json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&completeReq)

	// Verify file on agent via FILE_STAT
	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	statReq := protocol.FileStatRequest{Path: sess.Path}
	statResp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileStat, statReq)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var stat protocol.FileStatResponse
	if err := protocol.UnmarshalPayload(statResp.Payload, &stat); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing stat response: %w", err))
		return
	}

	if stat.Error != "" {
		apierror.Internal(w, r, fmt.Errorf("verifying upload: %s", stat.Error))
		return
	}

	if stat.Size != sess.TotalSize {
		apierror.BadRequest(w, r,
			fmt.Sprintf("Size mismatch: expected %d bytes, got %d bytes on agent.", sess.TotalSize, stat.Size), nil)
		return
	}

	// If the client provided a checksum, verify via file read + hash
	checksum := ""
	if completeReq.Checksum != "" {
		readReq := protocol.FileReadRequest{Path: sess.Path, MaxBytes: sess.TotalSize}
		readResp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileRead, readReq)
		if err == nil {
			var fileResp protocol.FileReadResponse
			if err := protocol.UnmarshalPayload(readResp.Payload, &fileResp); err == nil && fileResp.Error == "" {
				if decoded, err := base64.StdEncoding.DecodeString(fileResp.Content); err == nil {
					hash := sha256.Sum256(decoded)
					checksum = fmt.Sprintf("%x", hash)
					if checksum != completeReq.Checksum {
						apierror.BadRequest(w, r,
							fmt.Sprintf("Checksum mismatch: expected %s, got %s.", completeReq.Checksum, checksum), nil)
						s.uploads.Delete(uploadID)
						return
					}
				}
			}
		}
	}

	// Audit log
	s.auditFileOp(r, claims, agent, "file.uploaded", sess.Path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.uploaded",
		Data: map[string]string{
			"agentId": agentID,
			"path":    sess.Path,
		},
	})

	// Clean up session
	s.uploads.Delete(uploadID)

	writeJSON(w, http.StatusOK, uploadCompleteResponse{
		Path:     sess.Path,
		Size:     sess.TotalSize,
		Checksum: checksum,
	})
}

// handleCancelUpload removes an upload session.
// DELETE /api/v1/agents/{agentId}/uploads/{uploadId}
func (s *Server) handleCancelUpload(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	uploadID := chi.URLParam(r, "uploadId")
	claims := middleware.ClaimsFromCtx(r.Context())

	sess := s.uploads.Get(uploadID)
	if sess == nil {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	if sess.TenantID != claims.TenantID || sess.AgentID != agentID {
		apierror.NotFound(w, r, "Upload session not found.", nil)
		return
	}

	s.uploads.Delete(uploadID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
