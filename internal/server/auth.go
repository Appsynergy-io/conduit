package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
)

// loginRequest is the request body for POST /api/v1/auth/password/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handlePasswordLogin authenticates with email + password (dev mode only).
// POST /api/v1/auth/password/login
func (s *Server) handlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	// Production mode: password auth is disabled
	if s.cfg.Server.Mode != "dev" {
		apierror.NotFound(w, r, "Password authentication is not available.", nil)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.Email == "" || req.Password == "" {
		// Identical error for missing email or password (no enumeration — OWASP A07)
		apierror.Unauthorized(w, r, "Invalid email or password.", nil)
		return
	}

	ctx := r.Context()

	// Look up user — identical error for not-found vs wrong password (NIST AC-7, OWASP A07)
	user, err := s.db.GetUserByEmail(ctx, req.Email)
	if err != nil {
		// User not found — return same error as wrong password
		apierror.Unauthorized(w, r, "Invalid email or password.", nil)
		s.auditLoginFailure(r, nil, &req.Email, "user not found")
		return
	}

	if user.PasswordHash == nil {
		// No password set — same error
		apierror.Unauthorized(w, r, "Invalid email or password.", nil)
		s.auditLoginFailure(r, &user.ID, &req.Email, "no password hash")
		return
	}

	// Verify password (Argon2id, constant-time — NIST SP 800-63B)
	match, err := auth.VerifyPassword(req.Password, *user.PasswordHash)
	if err != nil || !match {
		apierror.Unauthorized(w, r, "Invalid email or password.", nil)
		s.auditLoginFailure(r, &user.ID, &req.Email, "wrong password")
		return
	}

	// Get enabled services for the tenant
	services, err := s.db.ListServices(ctx, user.TenantID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	var serviceSlugs []string
	for _, svc := range services {
		if svc.EnabledAt != "" {
			serviceSlugs = append(serviceSlugs, svc.Slug)
		}
	}

	// Create session
	sessionID := uuid.NewString()
	session := &db.Session{
		ID:       sessionID,
		TenantID: user.TenantID,
		UserID:   user.ID,
		Type:     "web",
		SourceIP: strPtr(r.RemoteAddr),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}
	if err := s.db.CreateSession(ctx, session); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Issue tokens
	accessToken, err := s.jwtMgr.IssueAccessToken(
		user.ID, user.TenantID, sessionID,
		[]string{user.Role}, serviceSlugs,
	)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	refreshToken, err := s.jwtMgr.IssueRefreshToken(user.ID, user.TenantID, sessionID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Update last login
	s.db.UpdateUserLastLogin(ctx, user.ID)

	// Audit log
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  user.TenantID,
		EventType: "auth.login.password",
		UserID:    &user.ID,
		UserEmail: &user.Email,
		SourceIP:  strPtr(r.RemoteAddr),
		Outcome:   "success",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    900, // 15 minutes
	})
}

// auditLoginFailure logs a failed login attempt (NIST AC-7, AU-2).
func (s *Server) auditLoginFailure(r *http.Request, userID, email *string, reason string) {
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  s.db.TenantID(),
		EventType: "auth.login.password",
		UserID:    userID,
		UserEmail: email,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"reason":"` + reason + `"}`),
		Outcome:   "failure",
	})
}
