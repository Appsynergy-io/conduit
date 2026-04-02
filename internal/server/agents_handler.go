package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

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

// agentDetailResponse extends agentResponse with live metrics.
type agentDetailResponse struct {
	agentResponse
	SystemInfo *agentSystemInfo `json:"systemInfo,omitempty"`
}

type agentSystemInfo struct {
	CPUPercent      float64 `json:"cpuPercent"`
	MemoryTotalBytes uint64 `json:"memoryTotalBytes"`
	MemoryUsedBytes  uint64 `json:"memoryUsedBytes"`
	DiskTotalBytes   uint64 `json:"diskTotalBytes"`
	DiskUsedBytes    uint64 `json:"diskUsedBytes"`
	UptimeSeconds    int64  `json:"uptimeSeconds"`
	LoadAvg1         float64 `json:"loadAvg1,omitempty"`
	LoadAvg5         float64 `json:"loadAvg5,omitempty"`
	LoadAvg15        float64 `json:"loadAvg15,omitempty"`
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

	labelFilters, err := parseLabelFilters(q["label"])
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	agents, err := s.db.ListAgentsPaginated(r.Context(), db.AgentListParams{
		TenantID:  claims.TenantID,
		CursorAt:  pp.CursorAt,
		CursorID:  pp.CursorID,
		Limit:     pp.Limit,
		Status:    q.Get("status"),
		Transport: q.Get("transport"),
		OS:        q.Get("os"),
		Search:    truncate(q.Get("search"), 500),
		Labels:    labelFilters,
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

	detail := agentDetailResponse{agentResponse: toAgentResponse(*agent)}
	if m := s.cachedMetrics(agentID); m != nil {
		detail.SystemInfo = &agentSystemInfo{
			CPUPercent:       m.CPUPercent,
			MemoryTotalBytes: m.MemTotal,
			MemoryUsedBytes:  m.MemUsed,
			DiskTotalBytes:   m.DiskTotal,
			DiskUsedBytes:    m.DiskUsed,
			UptimeSeconds:    m.Uptime,
			LoadAvg1:         m.LoadAvg1,
			LoadAvg5:         m.LoadAvg5,
			LoadAvg15:        m.LoadAvg15,
		}
	}
	writeJSON(w, http.StatusOK, detail)
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

// handleUpdateAgent updates agent display name and/or labels.
// PATCH /api/v1/agents/{agentId}
func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
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

	var req updateAgentRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", nil)
		return
	}

	// Validate labels
	if req.Labels != nil {
		if len(req.Labels) > 50 {
			apierror.BadRequest(w, r, "Maximum 50 labels allowed.", nil)
			return
		}
		for k, v := range req.Labels {
			if !labelKeyPattern.MatchString(k) {
				apierror.BadRequest(w, r, fmt.Sprintf("Invalid label key: %q. Must match [a-zA-Z0-9._-]{1,63}.", k), nil)
				return
			}
			if len(v) > 255 {
				apierror.BadRequest(w, r, fmt.Sprintf("Label value for key %q exceeds 255 characters.", k), nil)
				return
			}
		}
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

	// Build updated fields
	displayName := agent.DisplayName
	if req.DisplayName != nil {
		if *req.DisplayName == "" {
			displayName = nil
		} else {
			displayName = req.DisplayName
		}
	}

	labels := agent.Labels
	if req.Labels != nil {
		if len(req.Labels) == 0 {
			labels = nil
		} else {
			b, _ := json.Marshal(req.Labels)
			labelsStr := string(b)
			labels = &labelsStr
		}
	}

	if err := s.db.UpdateAgentMetadata(ctx, agentID, displayName, labels); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Re-read the agent to return updated state
	updated, err := s.db.GetAgentByID(ctx, agentID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      claims.TenantID,
		EventType:     "agent.updated",
		UserID:        &claims.Subject,
		AgentID:       &agentID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(r.RemoteAddr),
		Outcome:       "success",
	})

	writeJSON(w, http.StatusOK, toAgentResponse(*updated))
}

type updateAgentRequest struct {
	DisplayName *string           `json:"displayName"`
	Labels      map[string]string `json:"labels"`
}

// labelKeyPattern validates label keys per OpenAPI spec.
var labelKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,63}$`)

// parseLabelFilters parses the ?label= query parameters into LabelFilter structs.
// Format: key:value — with support for negation (!), OR (comma-separated values), and existence (empty value).
func parseLabelFilters(params []string) ([]db.LabelFilter, error) {
	if len(params) == 0 {
		return nil, nil
	}
	filters := make([]db.LabelFilter, 0, len(params))
	for _, p := range params {
		if len(p) > 500 {
			return nil, fmt.Errorf("Label filter exceeds 500 characters.")
		}
		negate := false
		s := p
		if strings.HasPrefix(s, "!") {
			negate = true
			s = s[1:]
		}
		idx := strings.Index(s, ":")
		if idx < 0 {
			return nil, fmt.Errorf("Invalid label filter %q. Expected format: key:value.", p)
		}
		key := s[:idx]
		valPart := s[idx+1:]

		if !labelKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("Invalid label key %q in filter.", key)
		}

		var values []string
		if valPart != "" {
			values = strings.Split(valPart, ",")
			for _, v := range values {
				if len(v) > 255 {
					return nil, fmt.Errorf("Label value exceeds 255 characters in filter.")
				}
			}
		}

		filters = append(filters, db.LabelFilter{
			Key:    key,
			Values: values,
			Negate: negate,
		})
	}
	return filters, nil
}
