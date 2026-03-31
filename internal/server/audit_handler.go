package server

import (
	"encoding/json"
	"net/http"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// auditEventResponse is the API response shape for an audit event.
type auditEventResponse struct {
	ID               string           `json:"id"`
	TenantID         string           `json:"tenantId"`
	EventType        string           `json:"eventType"`
	Timestamp        string           `json:"timestamp"`
	UserID           *string          `json:"userId,omitempty"`
	UserEmail        *string          `json:"userEmail,omitempty"`
	AgentID          *string          `json:"agentId,omitempty"`
	AgentHostname    *string          `json:"agentHostname,omitempty"`
	SourceIP         *string          `json:"sourceIp,omitempty"`
	UserAgent        *string          `json:"userAgent,omitempty"`
	Outcome          string           `json:"outcome"`
	Details          *json.RawMessage `json:"details,omitempty"`
	AlgorithmUsed    *string          `json:"algorithmUsed,omitempty"`
	AlgorithmWarning *string          `json:"algorithmWarning,omitempty"`
}

func toAuditEventResponse(e db.AuditEntry) auditEventResponse {
	resp := auditEventResponse{
		ID:               e.ID,
		TenantID:         e.TenantID,
		EventType:        e.EventType,
		Timestamp:        e.Timestamp,
		UserID:           e.UserID,
		UserEmail:        e.UserEmail,
		AgentID:          e.AgentID,
		AgentHostname:    e.AgentHostname,
		SourceIP:         e.SourceIP,
		UserAgent:        e.UserAgent,
		Outcome:          e.Outcome,
		AlgorithmUsed:    e.AlgorithmUsed,
		AlgorithmWarning: e.AlgorithmWarning,
	}
	if e.Details != nil {
		raw := json.RawMessage(*e.Details)
		resp.Details = &raw
	}
	return resp
}

// handleListAuditEvents returns paginated audit log entries.
// GET /api/v1/audit/events
func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	q := r.URL.Query()
	entries, err := s.db.ListAuditLogsPaginated(r.Context(), db.AuditListParams{
		TenantID:  claims.TenantID,
		CursorAt:  pp.CursorAt,
		CursorID:  pp.CursorID,
		Limit:     pp.Limit,
		EventType: truncate(q.Get("eventType"), 100),
		UserID:    q.Get("userId"),
		AgentID:   q.Get("agentId"),
		SourceIP:  truncate(q.Get("sourceIp"), 45),
		From:      truncate(q.Get("from"), 35),
		To:        truncate(q.Get("to"), 35),
		Outcome:   q.Get("outcome"),
		Search:    truncate(q.Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(entries) > pp.Limit
	if hasMore {
		entries = entries[:pp.Limit]
	}

	data := make([]auditEventResponse, len(entries))
	for i, e := range entries {
		data[i] = toAuditEventResponse(e)
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		c := encodeCursor(last.Timestamp, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}
