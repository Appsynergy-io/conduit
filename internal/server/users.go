package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

var validRoles = map[string]bool{
	"org_owner": true, "org_admin": true, "org_member": true,
}

// userResponse is the API response shape for a user.
type userResponse struct {
	ID           string   `json:"id"`
	TenantID     string   `json:"tenantId"`
	Email        string   `json:"email"`
	FirstName    string   `json:"firstName"`
	LastName     string   `json:"lastName"`
	DisplayName  *string  `json:"displayName,omitempty"`
	Role         string   `json:"role"`
	Status       string   `json:"status"`
	PasskeyCount int      `json:"passkeyCount"`
	GroupIDs     []string `json:"groupIds"`
	LastLoginAt  *string  `json:"lastLoginAt,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
	CreatedBy    *string  `json:"createdBy,omitempty"`
}

func toUserResponse(u db.User, passkeyCount int, groupIDs []string) userResponse {
	if groupIDs == nil {
		groupIDs = []string{}
	}
	return userResponse{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		DisplayName:  u.DisplayName,
		Role:         u.Role,
		Status:       u.Status,
		PasskeyCount: passkeyCount,
		GroupIDs:     groupIDs,
		LastLoginAt:  u.LastLoginAt,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
		CreatedBy:    u.CreatedBy,
	}
}

// handleListUsers returns paginated users.
// GET /api/v1/users
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	pp, err := parsePaginationParams(r)
	if err != nil {
		apierror.BadRequest(w, r, err.Error(), nil)
		return
	}

	q := r.URL.Query()
	users, err := s.db.ListUsersPaginated(r.Context(), db.UserListParams{
		TenantID: claims.TenantID,
		CursorAt: pp.CursorAt,
		CursorID: pp.CursorID,
		Limit:    pp.Limit,
		Status:   q.Get("status"),
		Role:     q.Get("role"),
		Search:   truncate(q.Get("search"), 500),
	})
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	hasMore := len(users) > pp.Limit
	if hasMore {
		users = users[:pp.Limit]
	}

	// Batch-enrich passkey counts and group IDs
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	pkCounts, _ := s.db.CountPasskeysByUsers(r.Context(), ids)
	grpIDs, _ := s.db.GetGroupIDsByUsers(r.Context(), ids)

	data := make([]userResponse, len(users))
	for i, u := range users {
		data[i] = toUserResponse(u, pkCounts[u.ID], grpIDs[u.ID])
	}

	pg := paginationMeta{HasMore: hasMore}
	if hasMore && len(users) > 0 {
		last := users[len(users)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		pg.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":       data,
		"pagination": pg,
	})
}

// createUserRequest is the request body for POST /api/v1/users.
type createUserRequest struct {
	Email       string   `json:"email"`
	FirstName   string   `json:"firstName"`
	LastName    string   `json:"lastName"`
	DisplayName string   `json:"displayName"`
	Role        string   `json:"role"`
	GroupIDs    []string `json:"groupIds"`
}

// handleCreateUser creates a new user.
// POST /api/v1/users
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())

	// RBAC: admin+ can create users
	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req createUserRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate
	if req.Email == "" || req.FirstName == "" || req.LastName == "" || req.Role == "" {
		apierror.BadRequest(w, r, "email, firstName, lastName, and role are required.", nil)
		return
	}
	if !emailRe.MatchString(req.Email) || len(req.Email) > 254 {
		apierror.BadRequest(w, r, "Invalid email address.", nil)
		return
	}
	if len(req.FirstName) > 100 || len(req.LastName) > 100 {
		apierror.BadRequest(w, r, "Name too long (max 100 characters).", nil)
		return
	}
	if len(req.DisplayName) > 255 {
		apierror.BadRequest(w, r, "Display name too long (max 255 characters).", nil)
		return
	}
	if !validRoles[req.Role] {
		apierror.BadRequest(w, r, "Invalid role. Must be org_owner, org_admin, or org_member.", nil)
		return
	}
	if len(req.GroupIDs) > 100 {
		apierror.BadRequest(w, r, "Too many group IDs (max 100).", nil)
		return
	}

	ctx := r.Context()

	// Check email uniqueness
	existing, err := s.db.GetUserByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		apierror.Conflict(w, r, "A user with this email already exists.", nil)
		return
	}

	userID := uuid.NewString()
	var displayName *string
	if req.DisplayName != "" {
		displayName = &req.DisplayName
	}

	user := &db.User{
		ID:          userID,
		TenantID:    claims.TenantID,
		Email:       req.Email,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		DisplayName: displayName,
		Role:        req.Role,
		Status:      "invited",
		CreatedBy:   &claims.Subject,
	}

	if err := s.db.CreateUser(ctx, user); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Add to groups
	for _, gid := range req.GroupIDs {
		_ = s.db.AddUserToGroup(ctx, claims.TenantID, gid, userID)
	}

	// Audit
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "user.created",
		UserID:    &claims.Subject,
		UserEmail: &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"created_user_id":"` + userID + `","email":"` + req.Email + `"}`),
		Outcome:   "success",
	})

	writeJSON(w, http.StatusCreated, toUserResponse(*user, 0, req.GroupIDs))
}

// handleGetUser returns a single user by ID.
// GET /api/v1/users/{userId}
func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	userID := chi.URLParam(r, "userId")

	if _, err := uuid.Parse(userID); err != nil {
		apierror.BadRequest(w, r, "Invalid user ID format.", nil)
		return
	}

	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			apierror.NotFound(w, r, "User not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}

	// Tenant isolation
	if user.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "User not found.", nil)
		return
	}

	pkCounts, _ := s.db.CountPasskeysByUsers(r.Context(), []string{userID})
	grpIDs, _ := s.db.GetGroupIDsByUsers(r.Context(), []string{userID})

	writeJSON(w, http.StatusOK, toUserResponse(*user, pkCounts[userID], grpIDs[userID]))
}

// updateUserRequest is the request body for PATCH /api/v1/users/{userId}.
type updateUserRequest struct {
	FirstName   *string  `json:"firstName"`
	LastName    *string  `json:"lastName"`
	DisplayName *string  `json:"displayName"`
	Role        *string  `json:"role"`
	GroupIDs    []string `json:"groupIds"`
}

// handleUpdateUser updates a user.
// PATCH /api/v1/users/{userId}
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	userID := chi.URLParam(r, "userId")

	if _, err := uuid.Parse(userID); err != nil {
		apierror.BadRequest(w, r, "Invalid user ID format.", nil)
		return
	}

	// RBAC: admin+ can update users (or self for limited fields)
	isSelf := claims.Subject == userID
	if !isSelf && !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	var req updateUserRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	ctx := r.Context()

	user, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			apierror.NotFound(w, r, "User not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}
	if user.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "User not found.", nil)
		return
	}

	// Apply changes
	if req.FirstName != nil {
		if len(*req.FirstName) > 100 {
			apierror.BadRequest(w, r, "First name too long (max 100).", nil)
			return
		}
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		if len(*req.LastName) > 100 {
			apierror.BadRequest(w, r, "Last name too long (max 100).", nil)
			return
		}
		user.LastName = *req.LastName
	}
	if req.DisplayName != nil {
		if len(*req.DisplayName) > 255 {
			apierror.BadRequest(w, r, "Display name too long (max 255).", nil)
			return
		}
		user.DisplayName = req.DisplayName
	}
	if req.Role != nil {
		if !isAdmin(claims.Roles) {
			apierror.Forbidden(w, r, "Only admins can change roles.", nil)
			return
		}
		if !validRoles[*req.Role] {
			apierror.BadRequest(w, r, "Invalid role.", nil)
			return
		}
		user.Role = *req.Role
	}

	if err := s.db.UpdateUser(ctx, user); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Update group memberships if provided
	groupIDs := req.GroupIDs
	if groupIDs != nil && isAdmin(claims.Roles) {
		// Remove all existing memberships, then re-add
		existingGroups, _ := s.db.ListGroupsForUser(ctx, userID)
		for _, g := range existingGroups {
			_ = s.db.RemoveUserFromGroup(ctx, g.ID, userID)
		}
		for _, gid := range groupIDs {
			_ = s.db.AddUserToGroup(ctx, claims.TenantID, gid, userID)
		}
	}

	// Audit
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "user.updated",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"updated_user_id":"` + userID + `"}`),
		Outcome:   "success",
	})

	pkCounts, _ := s.db.CountPasskeysByUsers(ctx, []string{userID})
	grpIDsMap, _ := s.db.GetGroupIDsByUsers(ctx, []string{userID})

	writeJSON(w, http.StatusOK, toUserResponse(*user, pkCounts[userID], grpIDsMap[userID]))
}

// isAdmin returns true if roles include platform_owner, org_owner, or org_admin.
func isAdmin(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "platform_owner", "org_owner", "org_admin":
			return true
		}
	}
	return false
}

// truncate returns s truncated to maxLen.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// jsonEncode is a helper that encodes v to JSON bytes, ignoring errors.
func jsonEncode(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
