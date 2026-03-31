package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// agentResponse is the API response shape for an agent.
type agentResponse struct {
	ID          string           `json:"id"`
	TenantID    string           `json:"tenantId"`
	Hostname    string           `json:"hostname"`
	DisplayName *string          `json:"displayName,omitempty"`
	OS          *string          `json:"os,omitempty"`
	Arch        *string          `json:"arch,omitempty"`
	Labels      *json.RawMessage `json:"labels,omitempty"`
	IP          *string          `json:"ip,omitempty"`
	Status      string           `json:"status"`
	Transport   *string          `json:"transport,omitempty"`
	Version     *string          `json:"version,omitempty"`
	LastSeenAt  *string          `json:"lastSeenAt,omitempty"`
	ConnectedAt *string          `json:"connectedAt,omitempty"`
	CreatedAt   string           `json:"createdAt"`
}

func toAgentResponse(a db.Agent) agentResponse {
	resp := agentResponse{
		ID:          a.ID,
		TenantID:    a.TenantID,
		Hostname:    a.Hostname,
		DisplayName: a.DisplayName,
		OS:          a.OS,
		Arch:        a.Arch,
		IP:          a.IP,
		Status:      a.Status,
		Transport:   a.Transport,
		Version:     a.Version,
		LastSeenAt:  a.LastSeenAt,
		ConnectedAt: a.ConnectedAt,
		CreatedAt:   a.CreatedAt,
	}
	if a.Labels != nil {
		raw := json.RawMessage(*a.Labels)
		resp.Labels = &raw
	}
	return resp
}

// handleListAgents returns paginated agents.
// GET /api/v1/agents
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	q := r.URL.Query()
	agents, err := s.db.ListAgentsPaginated(r.Context(), db.AgentListParams{
		TenantID:  claims.TenantID,
		CursorAt:  pp.CursorAt,
		CursorID:  pp.CursorID,
		Limit:     pp.Limit,
		Status:    q.Get("status"),
		Transport: q.Get("transport"),
		OS:        q.Get("os"),
		Search:    truncate(q.Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(agents) > pp.Limit
	if hasMore {
		agents = agents[:pp.Limit]
	}

	data := make([]agentResponse, len(agents))
	for i, a := range agents {
		data[i] = toAgentResponse(a)
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(agents) > 0 {
		last := agents[len(agents)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// handleGetAgent returns a single agent by ID.
// GET /api/v1/agents/{agentId}
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	agentID := chi.URLParam(r, "agentId")

	if _, err := uuid.Parse(agentID); err != nil {
		apierror.BadRequest(w, r, "Invalid agent ID format.", nil)
		return
	}

	agent, err := s.db.GetAgentByID(r.Context(), agentID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if agent == nil || agent.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Agent not found.", nil)
		return
	}

	writeJSON(w, http.StatusOK, toAgentResponse(*agent))
}

// handleDeleteAgent removes an agent.
// DELETE /api/v1/agents/{agentId}
func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	agentID := chi.URLParam(r, "agentId")

	if _, err := uuid.Parse(agentID); err != nil {
		apierror.BadRequest(w, r, "Invalid agent ID format.", nil)
		return
	}

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	ctx := r.Context()
	agent, err := s.db.GetAgentByID(ctx, agentID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if agent == nil || agent.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Agent not found.", nil)
		return
	}

	if err := s.db.DeleteAgent(ctx, agentID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "agent.deleted",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Details:       strPtr(`{"agent_id":"` + agentID + `","hostname":"` + agent.Hostname + `"}`),
		Outcome:       "success",
	})

	w.WriteHeader(http.StatusNoContent)
}
