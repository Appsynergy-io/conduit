package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// handleShellSession handles a browser WebSocket connection for an interactive
// shell session on a specific agent. It bridges the browser WebSocket to a
// CWP SHELL stream on the agent's multiplexed connection.
//
// GET /api/v1/shell/{agentId} (WebSocket upgrade)
func (s *Server) handleShellSession(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	// Authenticate via query param token (WebSocket upgrade)
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = authHeader[7:]
		}
	}
	if tokenStr == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	claims, err := s.jwtMgr.ValidateToken(tokenStr)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Verify agent exists and belongs to this tenant
	agent, err := s.db.GetAgentByID(r.Context(), agentID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if agent == nil || agent.TenantID != claims.TenantID {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	// Verify agent is connected
	connAgent := s.agentRegistry.Get(agentID)
	if connAgent == nil {
		http.Error(w, "Agent is not connected", http.StatusServiceUnavailable)
		return
	}

	// Parse terminal size
	cols := 80
	rows := 24
	if c := r.URL.Query().Get("cols"); c != "" {
		fmt.Sscanf(c, "%d", &cols)
	}
	if ro := r.URL.Query().Get("rows"); ro != "" {
		fmt.Sscanf(ro, "%d", &rows)
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

	if s.eventBus != nil {
		s.eventBus.Publish(claims.TenantID, Event{
			Channel: "shell",
			Type:    "shell.start",
			Data: map[string]string{
				"sessionId": sessionID,
				"agentId":   agentID,
				"userId":    claims.Subject,
			},
		})
	}

	s.logger.Info("shell session started",
		"session_id", sessionID,
		"agent_id", agentID,
		"user_id", claims.Subject,
	)

	// Bridge: browser ↔ agent
	done := make(chan struct{})

	// Browser → Agent: read from browser, send as SHELL_DATA to agent
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

	if s.eventBus != nil {
		s.eventBus.Publish(claims.TenantID, Event{
			Channel: "shell",
			Type:    "shell.end",
			Data: map[string]string{
				"sessionId": sessionID,
				"agentId":   agentID,
			},
		})
	}
}
