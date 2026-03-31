package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// groupResponse is the API response shape for a group.
type groupResponse struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenantId"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	MemberCount int     `json:"memberCount"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

func toGroupResponse(g db.Group, memberCount int) groupResponse {
	return groupResponse{
		ID:          g.ID,
		TenantID:    g.TenantID,
		Name:        g.Name,
		Description: g.Description,
		MemberCount: memberCount,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// handleListGroups returns paginated groups.
// GET /api/v1/groups
func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	groups, counts, err := s.db.ListGroupsPaginated(r.Context(), db.GroupListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
		Search:   truncate(r.URL.Query().Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(groups) > pp.Limit
	if hasMore {
		groups = groups[:pp.Limit]
		counts = counts[:pp.Limit]
	}

	data := make([]groupResponse, len(groups))
	for i, g := range groups {
		data[i] = toGroupResponse(g, counts[i])
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(groups) > 0 {
		last := groups[len(groups)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// createGroupRequest is the request body for POST /api/v1/groups.
type createGroupRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// handleCreateGroup creates a new group.
// POST /api/v1/groups
func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req createGroupRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.Name == "" || len(req.Name) > 255 {
		apierror.BadRequest(w, r, "name is required (max 255 characters).", nil)
		return
	}
	if req.Description != nil && len(*req.Description) > 1000 {
		apierror.BadRequest(w, r, "description too long (max 1000 characters).", nil)
		return
	}

	ctx := r.Context()
	groupID := uuid.NewString()

	group := &db.Group{
		ID:          groupID,
		TenantID:    claims.TenantID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := s.db.CreateGroup(ctx, group); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apierror.Conflict(w, r, "A group with this name already exists.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "group.created",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"group_id":"` + groupID + `","name":"` + req.Name + `"}`),
		Outcome:   "success",
	})

	writeJSON(w, http.StatusCreated, toGroupResponse(*group, 0))
}

// handleGetGroup returns a single group by ID.
// GET /api/v1/groups/{groupId}
func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	groupID := chi.URLParam(r, "groupId")

	if _, err := uuid.Parse(groupID); err != nil {
		apierror.BadRequest(w, r, "Invalid group ID format.", nil)
		return
	}

	ctx := r.Context()
	group, err := s.db.GetGroupByID(ctx, groupID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if group == nil || group.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Group not found.", nil)
		return
	}

	count, _ := s.db.CountGroupMembersByID(ctx, groupID)
	writeJSON(w, http.StatusOK, toGroupResponse(*group, count))
}

// updateGroupRequest is the request body for PATCH /api/v1/groups/{groupId}.
type updateGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// handleUpdateGroup updates a group.
// PATCH /api/v1/groups/{groupId}
func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	groupID := chi.URLParam(r, "groupId")

	if _, err := uuid.Parse(groupID); err != nil {
		apierror.BadRequest(w, r, "Invalid group ID format.", nil)
		return
	}

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req updateGroupRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	ctx := r.Context()
	group, err := s.db.GetGroupByID(ctx, groupID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if group == nil || group.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Group not found.", nil)
		return
	}

	name := group.Name
	desc := group.Description
	if req.Name != nil {
		if *req.Name == "" || len(*req.Name) > 255 {
			apierror.BadRequest(w, r, "name must be 1-255 characters.", nil)
			return
		}
		name = *req.Name
	}
	if req.Description != nil {
		if len(*req.Description) > 1000 {
			apierror.BadRequest(w, r, "description too long (max 1000).", nil)
			return
		}
		desc = req.Description
	}

	if err := s.db.UpdateGroup(ctx, groupID, name, desc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apierror.NotFound(w, r, "Group not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "group.updated",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"group_id":"` + groupID + `"}`),
		Outcome:   "success",
	})

	// Re-fetch to return updated state
	group, _ = s.db.GetGroupByID(ctx, groupID)
	count, _ := s.db.CountGroupMembersByID(ctx, groupID)
	writeJSON(w, http.StatusOK, toGroupResponse(*group, count))
}

// handleDeleteGroup deletes a group.
// DELETE /api/v1/groups/{groupId}
func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	groupID := chi.URLParam(r, "groupId")

	if _, err := uuid.Parse(groupID); err != nil {
		apierror.BadRequest(w, r, "Invalid group ID format.", nil)
		return
	}

	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	ctx := r.Context()
	group, err := s.db.GetGroupByID(ctx, groupID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if group == nil || group.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "Group not found.", nil)
		return
	}

	if err := s.db.DeleteGroup(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			apierror.NotFound(w, r, "Group not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "group.deleted",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"group_id":"` + groupID + `","name":"` + group.Name + `"}`),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusNoContent)
}
