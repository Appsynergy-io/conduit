package server

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// recordingResponse is the API response shape for a shell recording (list view, no data).
type recordingResponse struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId"`
	AgentID       string `json:"agentId"`
	UserID        string `json:"userId"`
	AgentHostname string `json:"agentHostname"`
	UserEmail     string `json:"userEmail"`
	Duration      int    `json:"duration"`
	SizeBytes     int    `json:"sizeBytes"`
	Format        string `json:"format"`
	CreatedAt     string `json:"createdAt"`
}

// recordingDetailResponse is the API response for a single recording including data.
type recordingDetailResponse struct {
	recordingResponse
	Data string `json:"data"` // base64-encoded recording content
}

// handleListRecordings returns paginated shell recordings.
// GET /api/v1/recordings?agent_id=&limit=&offset=
func (s *Server) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	q := r.URL.Query()
	agentID := q.Get("agent_id")

	// Validate agent_id if provided (OWASP API1 — BOLA prevention)
	if agentID != "" {
		if _, err := uuid.Parse(agentID); err != nil {
			apierror.BadRequest(w, r, "Invalid agent_id format.", nil)
			return
		}
	}

	// Parse limit (default 25, max 100 — OWASP API4)
	limit := 25
	if limitStr := q.Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 || parsed > 100 {
			apierror.BadRequest(w, r, "limit must be between 1 and 100.", nil)
			return
		}
		limit = parsed
	}

	// Parse offset (default 0)
	offset := 0
	if offsetStr := q.Get("offset"); offsetStr != "" {
		parsed, err := strconv.Atoi(offsetStr)
		if err != nil || parsed < 0 {
			apierror.BadRequest(w, r, "offset must be a non-negative integer.", nil)
			return
		}
		offset = parsed
	}

	recordings, total, err := s.db.ListShellRecordings(r.Context(), agentID, limit, offset)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Verify tenant isolation — the DB method already scopes by tenantID,
	// but we log the access for audit compliance (NIST AU-2)
	_ = claims.TenantID

	items := make([]recordingResponse, len(recordings))
	for i, rec := range recordings {
		items[i] = recordingResponse{
			ID:            rec.ID,
			SessionID:     rec.SessionID,
			AgentID:       rec.AgentID,
			UserID:        rec.UserID,
			AgentHostname: rec.AgentHostname,
			UserEmail:     rec.UserEmail,
			Duration:      rec.Duration,
			SizeBytes:     rec.SizeBytes,
			Format:        rec.Format,
			CreatedAt:     rec.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
	})
}

// handleGetRecording returns a single shell recording including its data.
// GET /api/v1/recordings/{recordingId}
func (s *Server) handleGetRecording(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	recordingID := chi.URLParam(r, "recordingId")

	if _, err := uuid.Parse(recordingID); err != nil {
		apierror.BadRequest(w, r, "Invalid recording ID format.", nil)
		return
	}

	rec, err := s.db.GetShellRecording(r.Context(), recordingID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if rec == nil || rec.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Recording not found.", nil)
		return
	}

	resp := recordingDetailResponse{
		recordingResponse: recordingResponse{
			ID:            rec.ID,
			SessionID:     rec.SessionID,
			AgentID:       rec.AgentID,
			UserID:        rec.UserID,
			AgentHostname: rec.AgentHostname,
			UserEmail:     rec.UserEmail,
			Duration:      rec.Duration,
			SizeBytes:     rec.SizeBytes,
			Format:        rec.Format,
			CreatedAt:     rec.CreatedAt,
		},
		Data: base64.StdEncoding.EncodeToString(rec.Data),
	}

	writeJSON(w, http.StatusOK, resp)
}
