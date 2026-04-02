package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// powerRequest is the request body for POST /api/v1/agents/{agentId}/power.
type powerRequest struct {
	Action string `json:"action"` // "reboot" or "poweroff"
}

// powerResponse is the response for a power command.
type powerResponse struct {
	AgentID string `json:"agentId"`
	Action  string `json:"action"`
	Status  string `json:"status"` // "sent"
}

// handlePowerAction sends a reboot or poweroff command to an agent.
// POST /api/v1/agents/{agentId}/power
func (s *Server) handlePowerAction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	agentID := chi.URLParam(r, "agentId")

	if _, err := uuid.Parse(agentID); err != nil {
		apierror.BadRequest(w, r, "Invalid agent ID format.", nil)
		return
	}

	// Admin-only — power commands are high-risk operations
	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req powerRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", nil)
		return
	}

	// Validate action
	if req.Action != "reboot" && req.Action != "poweroff" {
		apierror.BadRequest(w, r, "Action must be 'reboot' or 'poweroff'.", nil)
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
		apierror.BadRequest(w, r, "Agent is not connected.", nil)
		return
	}

	// Build OS-appropriate command
	command := powerCommand(agent.OS, req.Action)

	// Audit log before sending — the agent may go offline immediately
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "agent.power." + req.Action,
		UserID:        &claims.Subject,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(fmt.Sprintf(`{"agent_id":"%s","action":"%s"}`, agentID, req.Action)),
		Outcome:       "success",
	})

	// Send the power command via EXEC_START — fire and forget.
	// We don't wait for EXEC_EXIT because the machine will reboot/shutdown
	// before the process can report back.
	go s.sendPowerCommand(connAgent, command)

	s.publishEvent(r.Context(), claims.TenantID, Event{
		Channel: "agents",
		Type:    "agent.power." + req.Action,
		Data: map[string]string{
			"agentId":  agentID,
			"hostname": agent.Hostname,
			"action":   req.Action,
		},
	})

	writeJSON(w, http.StatusAccepted, powerResponse{
		AgentID: agentID,
		Action:  req.Action,
		Status:  "sent",
	})
}

// sendPowerCommand sends an EXEC_START frame with the power command to the agent.
// This is fire-and-forget: the agent will likely disconnect before responding.
func (s *Server) sendPowerCommand(agent *ConnectedAgent, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streamID := agent.Mux.NextStreamID()

	payload := protocol.ExecStartPayload{
		JobID:   uuid.NewString(),
		Command: command,
		Timeout: 30,
	}
	frame, err := protocol.NewFrame(protocol.FrameExecStart, streamID, payload)
	if err != nil {
		s.logger.Error("failed to create power command frame", "error", err)
		return
	}

	if err := agent.Mux.Send(ctx, frame); err != nil {
		s.logger.Warn("failed to send power command", "agent_id", agent.AgentID, "error", err)
	}
}

// powerCommand returns the OS-appropriate shell command for the given action.
func powerCommand(agentOS *string, action string) string {
	isWindows := agentOS != nil && *agentOS == "windows"
	switch action {
	case "reboot":
		if isWindows {
			return "shutdown /r /t 0"
		}
		return "shutdown -r now"
	case "poweroff":
		if isWindows {
			return "shutdown /s /t 0"
		}
		return "shutdown -h now"
	default:
		return ""
	}
}
