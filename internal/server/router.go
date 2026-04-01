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
// shell session on a specific agent. It bridges the browser WebSocket to a
// CWP SHELL stream on the agent's multiplexed connection.
//
// GET /api/v1/shell/{agentId} (WebSocket upgrade)
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

	// Allocate a stream on the agent's mux
	streamID := connAgent.Mux.NextStreamID()
	agentCh := connAgent.Mux.OpenStream(streamID)
	defer connAgent.Mux.CloseStream(streamID)

	// Create shell session record
	sessionID := uuid.NewString()
	shellSession := &db.ShellSession{
		ID:        sessionID,
		TenantID:  claims.TenantID,
		AgentID:   agentID,
		UserID:    claims.Subject,
		Status:    "active",
		Cols:      &cols,
		Rows:      &rows,
		Recording: 1,
	}
	if err := s.db.CreateShellSession(r.Context(), shellSession); err != nil {
		s.logger.Error("failed to create shell session", "error", err)
		return
	}

	// Initialize asciicast v2 recorder if recording is enabled (NIST AU-2)
	var recorder *asciicastRecorder
	if shellSession.Recording == 1 {
		recorder = newAsciicastRecorder(cols, rows)
	}

	// Send SHELL_START to agent
	startPayload := protocol.ShellStartPayload{
		SessionID: sessionID,
		Cols:      cols,
		Rows:      rows,
	}
	startFrame, err := protocol.NewFrame(protocol.FrameShellStart, streamID, startPayload)
	if err != nil {
		s.logger.Error("failed to create SHELL_START frame", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if err := connAgent.Mux.Send(ctx, startFrame); err != nil {
		s.logger.Error("failed to send SHELL_START", "error", err)
		return
	}

	// Audit
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "shell.start",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(fmt.Sprintf(`{"session_id":"%s","cols":%d,"rows":%d}`, sessionID, cols, rows)),
		Outcome:       "success",
	})

	s.publishEvent(ctx, claims.TenantID, Event{
		Channel: "shell",
		Type:    "shell.start",
		Data: map[string]string{
			"sessionId": sessionID,
			"agentId":   agentID,
			"userId":    claims.Subject,
		},
	})

	s.logger.Info("shell session started",
		"session_id", sessionID,
		"agent_id", agentID,
		"user_id", claims.Subject,
	)

	// Bridge: browser ↔ agent
	done := make(chan struct{})

	// Browser → Agent: read from browser, send as SHELL_DATA or SHELL_RESIZE to agent
	go func() {
		defer func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}()
		for {
			_, data, err := browserConn.Read(ctx)
			if err != nil {
				return
			}

			// Check if this is a resize command (JSON with "type":"resize")
			if f := parseBrowserResize(data, streamID); f != nil {
				if err := connAgent.Mux.Send(ctx, f); err != nil {
					return
				}
				continue
			}

			// Regular terminal data
			f := &protocol.Frame{
				Type:     protocol.FrameShellData,
				StreamID: streamID,
				Payload:  data,
			}
			if err := connAgent.Mux.Send(ctx, f); err != nil {
				return
			}
		}
	}()

	// Agent → Browser: read from agent stream, send to browser
	go func() {
		defer func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}()
		for {
			select {
			case f, ok := <-agentCh:
				if !ok {
					return
				}
				switch f.Type {
				case protocol.FrameShellData:
					writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
					browserConn.Write(writeCtx, websocket.MessageBinary, f.Payload)
					writeCancel()
					if recorder != nil {
						recorder.WriteOutput(f.Payload)
					}
				case protocol.FrameShellExit:
					// Shell exited
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for either direction to finish
	select {
	case <-done:
	case <-ctx.Done():
	}

	// Close shell session
	s.db.CloseShellSession(ctx, sessionID)

	// Save shell recording if enabled (NIST AU-2, AU-12)
	if recorder != nil && recorder.buf.Len() > 0 {
		rec := &db.ShellRecording{
			ID:            uuid.NewString(),
			TenantID:      claims.TenantID,
			SessionID:     sessionID,
			AgentID:       agentID,
			UserID:        claims.Subject,
			AgentHostname: agent.Hostname,
			Duration:      recorder.Duration(),
			SizeBytes:     recorder.buf.Len(),
			Format:        "asciicast-v2",
			Data:          recorder.Bytes(),
		}
		if err := s.db.CreateShellRecording(ctx, rec); err != nil {
			s.logger.Error("failed to save shell recording", "error", err, "session_id", sessionID)
		}
	}

	s.logger.Info("shell session ended",
		"session_id", sessionID,
		"agent_id", agentID,
	)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "shell.end",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(`{"session_id":"` + sessionID + `"}`),
		Outcome:       "success",
	})

	s.publishEvent(ctx, claims.TenantID, Event{
		Channel: "shell",
		Type:    "shell.end",
		Data: map[string]string{
			"sessionId": sessionID,
			"agentId":   agentID,
		},
	})
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
