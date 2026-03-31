package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// sessionResponse is the API response shape for a session.
type sessionResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	UserID       string  `json:"userId"`
	SourceIP     *string `json:"sourceIp,omitempty"`
	UserAgent    *string `json:"userAgent,omitempty"`
	CreatedAt    string  `json:"createdAt"`
	LastActiveAt string  `json:"lastActiveAt"`
	ExpiresAt    string  `json:"expiresAt"`
}

func toSessionResponse(s db.Session) sessionResponse {
	return sessionResponse{
		ID:           s.ID,
		Type:         s.Type,
		UserID:       s.UserID,
		SourceIP:     s.SourceIP,
		UserAgent:    s.UserAgent,
		CreatedAt:    s.CreatedAt,
		LastActiveAt: s.LastActiveAt,
		ExpiresAt:    s.ExpiresAt,
	}
}

// handleListSessions returns paginated sessions.
// GET /api/v1/sessions
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	sessions, err := s.db.ListSessionsPaginated(r.Context(), db.SessionListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
		Type:     r.URL.Query().Get("type"),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(sessions) > pp.Limit
	if hasMore {
		sessions = sessions[:pp.Limit]
	}

	data := make([]sessionResponse, len(sessions))
	for i, sess := range sessions {
		data[i] = toSessionResponse(sess)
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(sessions) > 0 {
		last := sessions[len(sessions)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// handleRevokeSession deletes a session.
// DELETE /api/v1/sessions/{sessionId}
func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	sessionID := chi.URLParam(r, "sessionId")

	if _, err := uuid.Parse(sessionID); err != nil {
		apierror.BadRequest(w, r, "Invalid session ID format.", nil)
		return
	}

	ctx := r.Context()

	// Verify session exists and belongs to this tenant
	session, err := s.db.GetSessionByID(ctx, sessionID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if session == nil || session.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Session not found.", nil)
		return
	}

	// Users can revoke their own sessions; admins can revoke any
	if session.UserID != claims.Subject && !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	if err := s.db.DeleteSession(ctx, sessionID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "session.revoked",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"session_id":"` + sessionID + `","target_user":"` + session.UserID + `"}`),
		Outcome:   "success",
	})

	s.publishEvent(ctx, claims.TenantID, Event{
		Channel: "auth",
		Type:    "session.revoked",
		Data: map[string]string{
			"sessionId":    sessionID,
			"targetUserId": session.UserID,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}
