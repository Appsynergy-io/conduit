package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// handleShellSession handles a browser WebSocket connection for an interactive
// shell session on a specific agent. Creates a persistent session via
// SessionManager and attaches the browser. When the browser disconnects,
// the session enters detached state instead of closing.
//
// GET /api/v1/agents/{agentId}/shell/new (WebSocket upgrade)
func (s *Server) handleShellSession(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	// Authenticate via Authorization header or httpOnly cookie (NIST IA-2, OWASP A07)
	tokenStr := middleware.ExtractToken(r)
	if tokenStr == "" {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	claims, err := s.jwtMgr.ValidateToken(tokenStr)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid or expired token.", nil)
		return
	}

	// Verify agent exists and belongs to this tenant
	agent, err := s.db.GetAgentByID(r.Context(), agentID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if agent == nil || agent.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Agent not found.", nil)
		return
	}

	// Verify agent is connected
	connAgent := s.agentRegistry.Get(agentID)
	if connAgent == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"Agent is not connected.", nil)
		return
	}

	// Parse terminal size with bounds enforcement (OWASP API4)
	cols := 80
	rows := 24
	if c := r.URL.Query().Get("cols"); c != "" {
		fmt.Sscanf(c, "%d", &cols)
	}
	if ro := r.URL.Query().Get("rows"); ro != "" {
		fmt.Sscanf(ro, "%d", &rows)
	}
	if cols <= 0 || cols > 500 {
		cols = 80
	}
	if rows <= 0 || rows > 500 {
		rows = 24
	}

	// Upgrade browser connection to WebSocket
	browserConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"conduit-shell-v1"},
	})
	if err != nil {
		s.logger.Error("shell websocket upgrade failed", "error", err)
		return
	}
	defer browserConn.Close(websocket.StatusNormalClosure, "")

	// Create persistent session via SessionManager
	ls, err := s.sessionMgr.CreateSession(
		r.Context(), connAgent, agent,
		claims.Subject, claims.TenantID,
		cols, rows,
		false, // not pinned by default
		3600,  // 1 hour idle timeout
		true,  // recording enabled
	)
	if err != nil {
		s.logger.Error("failed to create session", "error", err)
		return
	}

	// Audit
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "shell.start",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(fmt.Sprintf(`{"session_id":"%s","cols":%d,"rows":%d}`, ls.ID, cols, rows)),
		Outcome:       "success",
	})

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "shell",
		Type:    "shell.start",
		Data: map[string]string{
			"sessionId": ls.ID,
			"agentId":   agentID,
			"userId":    claims.Subject,
		},
	})

	s.logger.Info("shell session started",
		"session_id", ls.ID,
		"agent_id", agentID,
		"user_id", claims.Subject,
	)

	// Send session ID to browser so it can use pin/pop-out features
	sessionMsg, _ := json.Marshal(map[string]string{
		"type":      "session",
		"sessionId": ls.ID,
	})
	writeCtx, writeCancel := context.WithTimeout(r.Context(), 5*time.Second)
	if err := browserConn.Write(writeCtx, websocket.MessageText, sessionMsg); err != nil {
		writeCancel()
		s.logger.Error("failed to send session ID to browser", "error", err)
		return
	}
	writeCancel()

	// Attach browser — blocks until browser disconnects.
	// When browser disconnects, session enters detached state (PTY stays alive).
	if err := s.sessionMgr.AttachBrowser(r.Context(), ls, browserConn); err != nil {
		s.logger.Debug("browser attach ended", "session_id", ls.ID, "error", err)
	}

	// Audit detach
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "shell.detach",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(`{"session_id":"` + ls.ID + `"}`),
		Outcome:       "success",
	})
}

// handleShellAttach handles a browser WebSocket reconnecting to an existing
// detached shell session. Replays buffered output, then resumes live I/O.
//
// GET /api/v1/agents/{agentId}/shell/sessions/{sessionId}/ws (WebSocket upgrade)
func (s *Server) handleShellAttach(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	sessionID := chi.URLParam(r, "sessionId")

	// Authenticate (NIST IA-2, IA-11)
	tokenStr := middleware.ExtractToken(r)
	if tokenStr == "" {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	claims, err := s.jwtMgr.ValidateToken(tokenStr)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid or expired token.", nil)
		return
	}

	// Look up live session
	ls := s.sessionMgr.Get(sessionID)
	if ls == nil {
		apierror.NotFound(w, r, "Session not found or already closed.", nil)
		return
	}

	// Ownership check (NIST AC-3, OWASP API1 BOLA)
	if ls.UserID != claims.Subject {
		apierror.Forbidden(w, r, "Not the session owner.", nil)
		return
	}
	if ls.AgentID != agentID {
		apierror.NotFound(w, r, "Session not found on this agent.", nil)
		return
	}

	// Upgrade browser connection
	browserConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"conduit-shell-v1"},
	})
	if err != nil {
		s.logger.Error("shell attach websocket upgrade failed", "error", err)
		return
	}
	defer browserConn.Close(websocket.StatusNormalClosure, "")

	s.logger.Info("browser reattaching to session",
		"session_id", sessionID,
		"agent_id", agentID,
		"user_id", claims.Subject,
	)

	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "shell.attach",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(`{"session_id":"` + sessionID + `"}`),
		Outcome:       "success",
	})

	// Attach — blocks until browser disconnects again
	if err := s.sessionMgr.AttachBrowser(r.Context(), ls, browserConn); err != nil {
		s.logger.Debug("browser reattach ended", "session_id", sessionID, "error", err)
	}
}

// handleListAllShellSessions lists shell sessions across all agents.
// GET /api/v1/shell/sessions
func (s *Server) handleListAllShellSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	status := r.URL.Query().Get("status")
	agentID := r.URL.Query().Get("agentId")
	userID := r.URL.Query().Get("userId")

	var pinnedOnly *bool
	if p := r.URL.Query().Get("pinned"); p == "true" {
		t := true
		pinnedOnly = &t
	} else if p == "false" {
		f := false
		pinnedOnly = &f
	}

	// Non-admin users can only see their own sessions (NIST AC-3)
	if !isAdmin(claims.Roles) {
		userID = claims.Subject
	}

	sessions, err := s.db.ListShellSessions(r.Context(), claims.TenantID, status, agentID, userID, pinnedOnly)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": sessions})
}

// handleListAgentShellSessions lists shell sessions for a specific agent.
// GET /api/v1/agents/{agentId}/shell/sessions
func (s *Server) handleListAgentShellSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	agentID := chi.URLParam(r, "agentId")

	status := r.URL.Query().Get("status")
	userID := ""

	var pinnedOnly *bool
	if p := r.URL.Query().Get("pinned"); p == "true" {
		t := true
		pinnedOnly = &t
	} else if p == "false" {
		f := false
		pinnedOnly = &f
	}

	// Non-admin users can only see their own sessions (NIST AC-3)
	if !isAdmin(claims.Roles) {
		userID = claims.Subject
	}

	sessions, err := s.db.ListShellSessions(r.Context(), claims.TenantID, status, agentID, userID, pinnedOnly)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": sessions})
}

// handleGetShellSession returns details for a specific session.
// GET /api/v1/agents/{agentId}/shell/sessions/{sessionId}
func (s *Server) handleGetShellSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	session, err := s.db.GetShellSessionByID(r.Context(), sessionID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if session == nil || session.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Session not found.", nil)
		return
	}

	// Ownership check (NIST AC-3)
	if session.UserID != claims.Subject && !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Not the session owner.", nil)
		return
	}

	writeJSON(w, http.StatusOK, session)
}

// handleUpdateShellSession updates session settings (pin, idle timeout).
// PATCH /api/v1/agents/{agentId}/shell/sessions/{sessionId}
func (s *Server) handleUpdateShellSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	var req struct {
		Pinned      *bool `json:"pinned"`
		IdleTimeout *int  `json:"idleTimeout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", nil)
		return
	}

	// Validate idle timeout bounds
	if req.IdleTimeout != nil && (*req.IdleTimeout < 60 || *req.IdleTimeout > 86400) {
		apierror.BadRequest(w, r, "idleTimeout must be between 60 and 86400 seconds.", nil)
		return
	}

	// Look up live session
	ls := s.sessionMgr.Get(sessionID)
	if ls == nil {
		apierror.NotFound(w, r, "Session not found or already closed.", nil)
		return
	}

	// Ownership check (NIST AC-3)
	if ls.UserID != claims.Subject && !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Not the session owner.", nil)
		return
	}

	if err := s.sessionMgr.UpdateSession(r.Context(), ls, req.Pinned, req.IdleTimeout); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "shell.session.updated",
		UserID:    &claims.Subject,
		AgentID:   &ls.AgentID,
		Details:   strPtr(fmt.Sprintf(`{"session_id":"%s","pinned":%v}`, sessionID, ls.Pinned)),
		Outcome:   "success",
	})

	// Return updated session from DB
	session, err := s.db.GetShellSessionByID(r.Context(), sessionID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// handleTerminateShellSession terminates a live session.
// DELETE /api/v1/agents/{agentId}/shell/sessions/{sessionId}
func (s *Server) handleTerminateShellSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	ls := s.sessionMgr.Get(sessionID)
	if ls == nil {
		// Session not in memory — may be orphaned from a server restart or already closed.
		session, err := s.db.GetShellSessionByID(r.Context(), sessionID)
		if err != nil {
			apierror.Internal(w, r, err)
			return
		}
		if session == nil || session.TenantID != claims.TenantID {
			apierror.NotFound(w, r, "Session not found.", nil)
			return
		}
		// Close in DB if still active/detached (orphaned after restart)
		if session.Status != "closed" {
			if err := s.db.CloseShellSession(r.Context(), session.ID); err != nil {
				apierror.Internal(w, r, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Ownership check (NIST AC-3)
	if ls.UserID != claims.Subject && !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Not the session owner.", nil)
		return
	}

	s.sessionMgr.TerminateSession(r.Context(), ls)

	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "shell.session.terminated",
		UserID:    &claims.Subject,
		AgentID:   &ls.AgentID,
		Details:   strPtr(`{"session_id":"` + sessionID + `"}`),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusNoContent)
}

// browserResizeMsg is the JSON structure sent by the browser for terminal resize.
type browserResizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// parseBrowserResize checks if a browser WebSocket message is a resize command.
// Returns a SHELL_RESIZE frame if it is, nil otherwise.
func parseBrowserResize(data []byte, streamID uint32) *protocol.Frame {
	// Quick check — resize messages are JSON starting with '{'
	if len(data) == 0 || data[0] != '{' {
		return nil
	}

	var msg browserResizeMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	if msg.Type != "resize" || msg.Cols <= 0 || msg.Rows <= 0 {
		return nil
	}

	// Enforce sane terminal bounds (OWASP API4)
	if msg.Cols > 500 || msg.Rows > 500 {
		return nil
	}

	payload := protocol.ShellResizePayload{
		Cols: msg.Cols,
		Rows: msg.Rows,
	}
	f, err := protocol.NewFrame(protocol.FrameShellResize, streamID, payload)
	if err != nil {
		return nil
	}
	return f
}
