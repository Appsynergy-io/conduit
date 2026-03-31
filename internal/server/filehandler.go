package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

const fileRequestTimeout = 30 * time.Second

// handleListFiles returns a directory listing from the agent.
// GET /api/v1/agents/{agentId}/files?path=/var/log&showHidden=true
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		apierror.BadRequest(w, r, "path parameter is required.", nil)
		return
	}
	path = sanitizeFilePath(path)

	showHidden := r.URL.Query().Get("showHidden") == "true"

	req := protocol.FileListRequest{
		Path:       path,
		ShowHidden: showHidden,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileList, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var listResp protocol.FileListResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &listResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if listResp.Error != "" {
		apierror.NotFound(w, r, listResp.Error, nil)
		return
	}

	s.auditFileOp(r, claims, agent, "file.listed", path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.listed",
		Data: map[string]string{
			"agentId": agentID,
			"path":    path,
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    listResp.Path,
		"entries": listResp.Entries,
	})
}

// handleDownloadFile downloads a file from the agent.
// GET /api/v1/agents/{agentId}/files/download?path=/var/log/syslog
func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		apierror.BadRequest(w, r, "path parameter is required.", nil)
		return
	}
	path = sanitizeFilePath(path)

	req := protocol.FileReadRequest{
		Path: path,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileRead, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var readResp protocol.FileReadResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &readResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if readResp.Error != "" {
		apierror.NotFound(w, r, readResp.Error, nil)
		return
	}

	data, err := base64.StdEncoding.DecodeString(readResp.Content)
	if err != nil {
		apierror.Internal(w, r, fmt.Errorf("decoding file content: %w", err))
		return
	}

	s.auditFileOp(r, claims, agent, "file.downloaded", path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.downloaded",
		Data: map[string]string{
			"agentId": agentID,
			"path":    path,
		},
	})

	filename := filepath.Base(path)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleUploadFile uploads a file to the agent.
// POST /api/v1/agents/{agentId}/files/upload?path=/tmp/data.txt
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		apierror.BadRequest(w, r, "path parameter is required.", nil)
		return
	}
	path = sanitizeFilePath(path)

	// Read the request body (limited to 16MB by MaxBody middleware, can override here)
	body := http.MaxBytesReader(w, r.Body, 16<<20) // 16MB
	data := make([]byte, 0, 4096)
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	req := protocol.FileWriteRequest{
		Path:    path,
		Content: base64.StdEncoding.EncodeToString(data),
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileWrite, req)
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

	s.auditFileOp(r, claims, agent, "file.uploaded", path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.uploaded",
		Data: map[string]string{
			"agentId": agentID,
			"path":    path,
		},
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"path":     writeResp.Path,
		"size":     writeResp.Size,
		"checksum": writeResp.Checksum,
	})
}

// handleDeleteFile deletes a file or directory on the agent.
// POST /api/v1/agents/{agentId}/files/delete
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	var body struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := decodeJSONStrict(r, &body); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}
	if body.Path == "" {
		apierror.BadRequest(w, r, "path is required.", nil)
		return
	}
	body.Path = sanitizeFilePath(body.Path)

	req := protocol.FileDeleteRequest{
		Path:      body.Path,
		Recursive: body.Recursive,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileDelete, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var delResp protocol.FileDeleteResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &delResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if delResp.Error != "" {
		apierror.BadRequest(w, r, delResp.Error, nil)
		return
	}

	s.auditFileOp(r, claims, agent, "file.deleted", body.Path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.deleted",
		Data: map[string]string{
			"agentId": agentID,
			"path":    body.Path,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

// handleRenameFile renames/moves a file on the agent.
// POST /api/v1/agents/{agentId}/files/rename
func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	var body struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := decodeJSONStrict(r, &body); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}
	if body.OldPath == "" || body.NewPath == "" {
		apierror.BadRequest(w, r, "oldPath and newPath are required.", nil)
		return
	}
	body.OldPath = sanitizeFilePath(body.OldPath)
	body.NewPath = sanitizeFilePath(body.NewPath)

	req := protocol.FileRenameRequest{
		OldPath: body.OldPath,
		NewPath: body.NewPath,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileRename, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var renameResp protocol.FileRenameResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &renameResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if renameResp.Error != "" {
		apierror.BadRequest(w, r, renameResp.Error, nil)
		return
	}

	s.auditFileOp(r, claims, agent, "file.renamed", body.OldPath+" -> "+body.NewPath)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.renamed",
		Data: map[string]string{
			"agentId": agentID,
			"oldPath": body.OldPath,
			"newPath": body.NewPath,
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"oldPath": renameResp.OldPath,
		"newPath": renameResp.NewPath,
	})
}

// handleMkdir creates a directory on the agent.
// POST /api/v1/agents/{agentId}/files/mkdir
func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	agent, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	var body struct {
		Path    string `json:"path"`
		Parents bool   `json:"parents"`
	}
	if err := decodeJSONStrict(r, &body); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}
	if body.Path == "" {
		apierror.BadRequest(w, r, "path is required.", nil)
		return
	}
	body.Path = sanitizeFilePath(body.Path)

	req := protocol.FileMkdirRequest{
		Path:    body.Path,
		Parents: body.Parents,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileMkdir, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var mkdirResp protocol.FileMkdirResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &mkdirResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if mkdirResp.Error != "" {
		apierror.BadRequest(w, r, mkdirResp.Error, nil)
		return
	}

	s.auditFileOp(r, claims, agent, "file.mkdir", body.Path)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "files",
		Type:    "file.mkdir",
		Data: map[string]string{
			"agentId": agentID,
			"path":    body.Path,
		},
	})

	w.WriteHeader(http.StatusCreated)
}

// handlePreviewFile returns a preview of a file's content.
// GET /api/v1/agents/{agentId}/files/preview?path=/etc/hosts&maxBytes=65536
func (s *Server) handlePreviewFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	claims := middleware.ClaimsFromCtx(r.Context())

	_, connAgent, err := s.resolveConnectedAgent(r.Context(), agentID, claims.TenantID)
	if err != nil {
		writeAgentError(w, r, err)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		apierror.BadRequest(w, r, "path parameter is required.", nil)
		return
	}
	path = sanitizeFilePath(path)

	maxBytes := int64(65536)
	if mb := r.URL.Query().Get("maxBytes"); mb != "" {
		if v, err := strconv.ParseInt(mb, 10, 64); err == nil && v > 0 && v <= 1048576 {
			maxBytes = v
		}
	}

	req := protocol.FileReadRequest{
		Path:     path,
		MaxBytes: maxBytes,
	}

	resp, err := s.sendFileRequest(r.Context(), connAgent, protocol.FrameFileRead, req)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	var readResp protocol.FileReadResponse
	if err := protocol.UnmarshalPayload(resp.Payload, &readResp); err != nil {
		apierror.Internal(w, r, fmt.Errorf("parsing agent response: %w", err))
		return
	}

	if readResp.Error != "" {
		apierror.NotFound(w, r, readResp.Error, nil)
		return
	}

	// Decode content and try to present as UTF-8 text, fall back to base64
	data, _ := base64.StdEncoding.DecodeString(readResp.Content)
	contentStr := readResp.Content // default to base64
	mimeType := readResp.MimeType
	if strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" || mimeType == "application/xml" {
		contentStr = string(data) // plain text
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":      readResp.Path,
		"content":   contentStr,
		"truncated": readResp.Truncated,
		"totalSize": readResp.Size,
		"mimeType":  mimeType,
	})
}

// sendFileRequest sends a CWP file request to the agent and waits for the response.
// Uses a dedicated stream for the request-response pair.
func (s *Server) sendFileRequest(ctx context.Context, connAgent *ConnectedAgent, frameType protocol.FrameType, payload interface{}) (*protocol.Frame, error) {
	streamID := connAgent.Mux.NextStreamID()
	streamCh := connAgent.Mux.OpenStream(streamID)
	defer connAgent.Mux.CloseStream(streamID)

	f, err := protocol.NewFrame(frameType, streamID, payload)
	if err != nil {
		return nil, fmt.Errorf("creating frame: %w", err)
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, fileRequestTimeout)
	defer reqCancel()

	if err := connAgent.Mux.Send(reqCtx, f); err != nil {
		return nil, fmt.Errorf("sending frame to agent: %w", err)
	}

	// Wait for response
	select {
	case resp, ok := <-streamCh:
		if !ok {
			return nil, fmt.Errorf("stream closed before response")
		}
		return resp, nil
	case <-reqCtx.Done():
		return nil, fmt.Errorf("agent response timeout")
	}
}

// resolveConnectedAgent validates the agent exists, belongs to the tenant, and is connected.
func (s *Server) resolveConnectedAgent(ctx context.Context, agentID, tenantID string) (*db.Agent, *ConnectedAgent, error) {
	agent, err := s.db.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, nil, &agentErr{code: http.StatusInternalServerError, msg: "Internal error"}
	}
	if agent == nil || agent.TenantID != tenantID {
		return nil, nil, &agentErr{code: http.StatusNotFound, msg: "Agent not found"}
	}

	connAgent := s.agentRegistry.Get(agentID)
	if connAgent == nil {
		return nil, nil, &agentErr{code: http.StatusServiceUnavailable, msg: "Agent is not connected"}
	}

	return agent, connAgent, nil
}

// agentErr is a structured error for agent resolution failures.
type agentErr struct {
	code int
	msg  string
}

func (e *agentErr) Error() string { return e.msg }

// writeAgentError writes an appropriate error response for agent resolution failures.
func writeAgentError(w http.ResponseWriter, r *http.Request, err error) {
	if ae, ok := err.(*agentErr); ok {
		switch ae.code {
		case http.StatusNotFound:
			apierror.NotFound(w, r, ae.msg, nil)
		case http.StatusServiceUnavailable:
			apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable", ae.msg, nil)
		default:
			apierror.Internal(w, r, err)
		}
		return
	}
	apierror.Internal(w, r, err)
}

// sanitizeFilePath cleans an absolute path to prevent traversal attacks.
func sanitizeFilePath(path string) string {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join("/", cleaned)
	}
	return cleaned
}

// auditFileOp logs a file operation audit event.
func (s *Server) auditFileOp(r *http.Request, claims *auth.Claims, agent *db.Agent, eventType, path string) {
	details, _ := json.Marshal(map[string]string{"path": path})
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     eventType,
		UserID:        &claims.Subject,
		AgentID:       &agent.ID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(string(details)),
		Outcome:       "success",
	})
}
