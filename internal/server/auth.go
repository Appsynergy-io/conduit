package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// handleAuthConfig returns public auth configuration for the login UI.
// GET /api/v1/auth/config
// Exposes only what the client needs to render the correct login form.
// No internal details leaked (NIST SI-11, OWASP A05).
func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"passwordAuth": s.cfg.Server.Mode == "dev",
	})
}

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
	if err := decodeJSONStrict(r, &req); err != nil {
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

	// Reuse existing web session from same browser, or create a new one
	ua := r.UserAgent()
	sessionID, err := s.findOrCreateSession(ctx, user.ID, user.TenantID, "web", r.RemoteAddr, ua)
	if err != nil {
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

	s.publishEvent(ctx, user.TenantID, Event{
		Channel: "auth",
		Type:    "auth.login",
		Data: map[string]string{
			"userId": user.ID,
			"email":  user.Email,
		},
	})

	// Set httpOnly cookies before writing the response body (NIST SC-23, OWASP V3).
	setAuthCookie(w, accessToken, int(s.jwtMgr.AccessTTL().Seconds()))
	setRefreshCookie(w, refreshToken, int(s.jwtMgr.RefreshTTL().Seconds()))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    int(s.jwtMgr.AccessTTL().Seconds()),
		"user": map[string]any{
			"id":       user.ID,
			"email":    user.Email,
			"tenantId": user.TenantID,
			"roles":    []string{user.Role},
		},
	})
}

// setAuthCookie sets the httpOnly auth cookie with strict security attributes.
// Uses __Host- prefix which requires Secure, Path=/, and no Domain (OWASP V3, NIST SC-23).
func setAuthCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// setRefreshCookie sets the httpOnly refresh cookie (matches configured refresh token TTL).
func setRefreshCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.RefreshCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearAuthCookies removes both auth and refresh cookies by setting them to
// expire immediately.
func clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.RefreshCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// handleAuthMe returns the authenticated user's identity from the JWT claims.
// GET /api/v1/auth/me
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"userId":   claims.Subject,
		"tenantId": claims.TenantID,
		"roles":    claims.Roles,
	})
}

// handleLogout clears the auth cookie and returns 204 No Content.
// POST /api/v1/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims != nil {
		s.publishEvent(r.Context(), claims.TenantID, Event{
			Channel: "auth",
			Type:    "auth.logout",
			Data: map[string]string{
				"userId": claims.Subject,
			},
		})
	}

	clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleRefresh reads the httpOnly refresh cookie, validates the refresh JWT,
// and issues a new access token + rotated refresh token as httpOnly cookies.
// POST /api/v1/auth/refresh
//
// Public (no auth middleware) — the access cookie has expired, so Auth
// middleware would reject it. The refresh cookie is the proof of identity.
// NIST IA-11 (re-authentication), OWASP ASVS V3 (token rotation).
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.RefreshCookieName)
	if err != nil || cookie.Value == "" {
		apierror.Unauthorized(w, r, "Missing refresh token.", nil)
		return
	}

	claims, err := s.jwtMgr.ValidateToken(cookie.Value)
	if err != nil {
		// Expired or tampered refresh token — must re-login.
		clearAuthCookies(w)
		apierror.Unauthorized(w, r, "Refresh token expired. Please sign in again.", nil)
		return
	}

	ctx := r.Context()

	// Verify the user still exists and load their current role + services.
	user, err := s.db.GetUserByID(ctx, claims.Subject)
	if err != nil {
		clearAuthCookies(w)
		apierror.Unauthorized(w, r, "User not found.", nil)
		return
	}

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

	// Re-use existing session ID from the refresh token.
	sessionID := claims.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	// Issue fresh tokens (NIST IA-11 — token rotation on each refresh).
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

	// Refresh the DB session expiry.
	_ = s.db.RefreshSession(ctx, sessionID)

	setAuthCookie(w, accessToken, int(s.jwtMgr.AccessTTL().Seconds()))
	setRefreshCookie(w, refreshToken, int(s.jwtMgr.RefreshTTL().Seconds()))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"expiresIn": int(s.jwtMgr.AccessTTL().Seconds()),
	})
}

// findOrCreateSession looks for an existing non-expired session with matching
// user, type, user-agent, AND source IP. If found, it refreshes the session's
// expiry. A different IP always creates a new session for security visibility —
// the user can spot unauthorized access from unfamiliar IPs in Active Sessions.
// (NIST AC-3, AU-2: every distinct origin is a distinct session for audit.)
func (s *Server) findOrCreateSession(ctx context.Context, userID, tenantID, sessType, remoteAddr, userAgent string) (string, error) {
	// Try to find an existing session from the same browser + IP
	existing, err := s.db.FindActiveSession(ctx, userID, sessType, userAgent, remoteAddr)
	if err != nil {
		return "", err
	}

	if existing != nil {
		// Refresh the existing session's expiry (same device, same network)
		if err := s.db.RefreshSession(ctx, existing.ID); err != nil {
			return "", err
		}
		return existing.ID, nil
	}

	// Create a new session — new device, new network, or first login
	sessionID := uuid.NewString()
	session := &db.Session{
		ID:        sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Type:      sessType,
		SourceIP:  strPtr(remoteAddr),
		UserAgent: strPtr(userAgent),
		ExpiresAt: time.Now().UTC().Add(s.jwtMgr.RefreshTTL()).Format(time.RFC3339),
	}
	if err := s.db.CreateSession(ctx, session); err != nil {
		return "", err
	}
	return sessionID, nil
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
