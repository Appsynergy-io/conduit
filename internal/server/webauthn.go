package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

// handleWebAuthnRegisterBegin starts passkey registration for the authenticated user.
// POST /api/v1/auth/webauthn/register/begin
func (s *Server) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"WebAuthn is not configured.", nil)
		return
	}

	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	ctx := r.Context()

	user, err := s.db.GetUserByID(ctx, claims.Subject)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Load existing passkeys to exclude re-registration
	passkeys, err := s.db.GetPasskeysByUserID(ctx, user.ID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	waUser := buildWebAuthnUser(user, passkeys)

	// Build exclude list from existing credentials
	var excludeList []protocol.CredentialDescriptor
	for _, cred := range waUser.Credentials {
		excludeList = append(excludeList, cred.Descriptor())
	}

	creation, session, err := s.webAuthn.BeginRegistration(
		waUser,
		withExcludeList(excludeList),
	)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Store session keyed by "register:<userID>"
	s.webAuthnSessions.Store("register:"+user.ID, session)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(creation)
}

// handleWebAuthnRegisterFinish completes passkey registration for the authenticated user.
// POST /api/v1/auth/webauthn/register/finish
func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"WebAuthn is not configured.", nil)
		return
	}

	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	ctx := r.Context()

	user, err := s.db.GetUserByID(ctx, claims.Subject)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	passkeys, err := s.db.GetPasskeysByUserID(ctx, user.ID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	waUser := buildWebAuthnUser(user, passkeys)

	sessionData, ok := s.webAuthnSessions.Get("register:" + user.ID)
	if !ok {
		apierror.BadRequest(w, r, "No pending registration session. Call register/begin first.", nil)
		return
	}

	// Parse the credential creation response from the request body
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		apierror.BadRequest(w, r, "Invalid credential creation response.", err)
		return
	}

	credential, err := s.webAuthn.CreateCredential(waUser, *sessionData, parsedResponse)
	if err != nil {
		apierror.BadRequest(w, r, "Failed to verify credential.", err)
		return
	}

	// Clean up session
	s.webAuthnSessions.Delete("register:" + user.ID)

	// Determine algorithm info from the attestation
	algID := parsedResponse.Raw.AttestationResponse.PublicKeyAlgorithm
	algName := auth.AlgorithmName(algID)
	algWarning := auth.AlgorithmWarning(algID)

	// Determine authenticator type
	authenticatorType := "cross-platform"
	if credential.Authenticator.Attachment == protocol.Platform {
		authenticatorType = "platform"
	}

	passkey := &db.Passkey{
		ID:                uuid.NewString(),
		TenantID:          user.TenantID,
		UserID:            user.ID,
		CredentialID:      credential.ID,
		PublicKey:         credential.PublicKey,
		Algorithm:         algName,
		AlgorithmWarning:  algWarning,
		AuthenticatorType: authenticatorType,
		SignCount:         credential.Authenticator.SignCount,
		BackupEligible:    credential.Flags.BackupEligible,
		BackupState:       credential.Flags.BackupState,
	}

	if err := s.db.CreatePasskey(ctx, passkey); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit log (NIST AU-2, AU-3)
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:               uuid.NewString(),
		TenantID:         user.TenantID,
		EventType:        "auth.passkey.registered",
		UserID:           &user.ID,
		UserEmail:        &user.Email,
		SourceIP:         strPtr(r.RemoteAddr),
		Details:          strPtr(`{"passkey_id":"` + passkey.ID + `","algorithm":"` + algName + `"}`),
		Outcome:          "success",
		AlgorithmUsed:    &algName,
		AlgorithmWarning: algWarning,
	})

	s.publishEvent(ctx, user.TenantID, Event{
		Channel: "auth",
		Type:    "auth.passkey.registered",
		Data: map[string]string{
			"userId":    user.ID,
			"passkeyId": passkey.ID,
		},
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id":                passkey.ID,
		"authenticatorType": authenticatorType,
		"algorithm":         algName,
		"createdAt":         passkey.CreatedAt,
	})
}

// webauthnLoginBeginRequest is the request body for login/begin.
type webauthnLoginBeginRequest struct {
	Email string `json:"email"`
}

// handleWebAuthnLoginBegin starts the passkey login ceremony.
// POST /api/v1/auth/webauthn/login/begin
func (s *Server) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"WebAuthn is not configured.", nil)
		return
	}

	var req webauthnLoginBeginRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.Email == "" {
		// Use identical error message for all cases (no user enumeration — OWASP A07)
		apierror.Unauthorized(w, r, "Invalid credentials.", nil)
		return
	}

	ctx := r.Context()

	user, err := s.db.GetUserByEmail(ctx, req.Email)
	if err != nil {
		// User not found — same error as no passkeys (NIST AC-7, OWASP A07)
		apierror.Unauthorized(w, r, "Invalid credentials.", nil)
		s.auditWebAuthnLoginFailure(r, nil, &req.Email, "user not found")
		return
	}

	passkeys, err := s.db.GetPasskeysByUserID(ctx, user.ID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	if len(passkeys) == 0 {
		// No passkeys registered — same error (no enumeration)
		apierror.Unauthorized(w, r, "Invalid credentials.", nil)
		s.auditWebAuthnLoginFailure(r, &user.ID, &req.Email, "no passkeys registered")
		return
	}

	waUser := buildWebAuthnUser(user, passkeys)

	assertion, session, err := s.webAuthn.BeginLogin(waUser)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Store session keyed by "login:<userID>"
	s.webAuthnSessions.Store("login:"+user.ID, session)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assertion)
}

// handleWebAuthnLoginFinish completes the passkey login ceremony and issues a JWT.
// POST /api/v1/auth/webauthn/login/finish
func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "Service Unavailable",
			"WebAuthn is not configured.", nil)
		return
	}

	// The request body contains the WebAuthn assertion response.
	// We need the email to look up the user, but the browser sends the credential assertion.
	// Strategy: use the userHandle from the assertion response to look up the user,
	// or use query param for email hint.
	// For now, parse the credential assertion from the body and use the session store
	// which maps login sessions by user ID found during BeginLogin.

	// Parse the assertion response from the request body
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		apierror.BadRequest(w, r, "Invalid credential assertion response.", err)
		return
	}

	ctx := r.Context()

	// Look up the user by the credential ID from the assertion
	dbPasskey, err := s.db.GetPasskeyByCredentialID(ctx, parsedResponse.RawID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			apierror.Unauthorized(w, r, "Invalid credentials.", nil)
			return
		}
		// Same error to avoid leaking credential lookup details (OWASP A07)
		apierror.Unauthorized(w, r, "Invalid credentials.", nil)
		return
	}

	user, err := s.db.GetUserByID(ctx, dbPasskey.UserID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	passkeys, err := s.db.GetPasskeysByUserID(ctx, user.ID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	waUser := buildWebAuthnUser(user, passkeys)

	sessionData, ok := s.webAuthnSessions.Get("login:" + user.ID)
	if !ok {
		apierror.BadRequest(w, r, "No pending login session. Call login/begin first.", nil)
		return
	}

	credential, err := s.webAuthn.ValidateLogin(waUser, *sessionData, parsedResponse)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid credentials.", err)
		s.auditWebAuthnLoginFailure(r, &user.ID, &user.Email, "assertion validation failed")
		return
	}

	// Clean up session
	s.webAuthnSessions.Delete("login:" + user.ID)

	// Update sign count in DB (OWASP A07 — detect cloned authenticators)
	for _, p := range passkeys {
		if bytes.Equal(p.CredentialID, credential.ID) {
			if err := s.db.UpdatePasskeySignCount(ctx, p.ID, credential.Authenticator.SignCount); err != nil {
				s.logger.WarnContext(ctx, "failed to update passkey sign count",
					"passkey_id", p.ID,
					"error", err,
				)
			}
			break
		}
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

	// Issue tokens (Ed25519 — NIST SP 800-175B)
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

	// Audit log (NIST AU-2, AU-3)
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      user.TenantID,
		EventType:     "auth.login.webauthn",
		UserID:        &user.ID,
		UserEmail:     &user.Email,
		SourceIP:      strPtr(r.RemoteAddr),
		Outcome:       "success",
		AlgorithmUsed: strPtr(dbPasskey.Algorithm),
	})

	s.publishEvent(ctx, user.TenantID, Event{
		Channel: "auth",
		Type:    "auth.login",
		Data: map[string]string{
			"userId": user.ID,
			"email":  user.Email,
			"method": "webauthn",
		},
	})

	// Set httpOnly cookies (NIST SC-23, OWASP V3)
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

// handleListPasskeys returns the authenticated user's registered passkeys.
// GET /api/v1/auth/webauthn/credentials
func (s *Server) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	passkeys, err := s.db.GetPasskeysByUserID(r.Context(), claims.Subject)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	type passkeyResponse struct {
		ID                string  `json:"id"`
		Algorithm         string  `json:"algorithm"`
		AlgorithmWarning  *string `json:"algorithmWarning,omitempty"`
		AuthenticatorType string  `json:"authenticatorType"`
		DisplayName       *string `json:"displayName,omitempty"`
		CreatedAt         string  `json:"createdAt"`
		LastUsedAt        *string `json:"lastUsedAt,omitempty"`
	}

	items := make([]passkeyResponse, 0, len(passkeys))
	for _, p := range passkeys {
		items = append(items, passkeyResponse{
			ID:                p.ID,
			Algorithm:         p.Algorithm,
			AlgorithmWarning:  p.AlgorithmWarning,
			AuthenticatorType: p.AuthenticatorType,
			DisplayName:       p.DisplayName,
			CreatedAt:         p.CreatedAt,
			LastUsedAt:        p.LastUsedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"items": items,
		"total": len(items),
	})
}

// handleDeletePasskey removes a passkey belonging to the authenticated user.
// DELETE /api/v1/auth/webauthn/credentials/{credentialId}
func (s *Server) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	credentialID := chi.URLParam(r, "credentialId")
	if credentialID == "" {
		apierror.BadRequest(w, r, "Credential ID is required.", nil)
		return
	}

	ctx := r.Context()

	// Verify ownership — BOLA prevention (OWASP API1, A01)
	passkey, err := s.db.GetPasskeyByID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			apierror.NotFound(w, r, "Passkey not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}

	if passkey.UserID != claims.Subject {
		apierror.NotFound(w, r, "Passkey not found.", nil)
		return
	}

	// Prevent deleting the last passkey in production mode (user would be locked out)
	if s.cfg.Server.Mode != "dev" {
		passkeys, err := s.db.GetPasskeysByUserID(ctx, claims.Subject)
		if err != nil {
			apierror.Internal(w, r, err)
			return
		}
		if len(passkeys) <= 1 {
			apierror.BadRequest(w, r, "Cannot delete your only passkey. Register another passkey first.", nil)
			return
		}
	}

	if err := s.db.DeletePasskey(ctx, credentialID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit log
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "auth.passkey.deleted",
		UserID:    strPtr(claims.Subject),
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"passkey_id":"` + credentialID + `"}`),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusNoContent)
}

// buildWebAuthnUser creates a WebAuthnUser from a DB user and passkeys.
func buildWebAuthnUser(user *db.User, passkeys []db.Passkey) *auth.WebAuthnUser {
	credentials := make([]webauthn.Credential, 0, len(passkeys))
	for _, p := range passkeys {
		credentials = append(credentials, auth.PasskeyToCredential(
			p.CredentialID, p.PublicKey, p.SignCount, p.AuthenticatorType,
			p.BackupEligible, p.BackupState,
		))
	}

	displayName := user.Email
	if user.DisplayName != nil && *user.DisplayName != "" {
		displayName = *user.DisplayName
	} else if user.FirstName != "" {
		displayName = user.FirstName + " " + user.LastName
	}

	return &auth.WebAuthnUser{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: displayName,
		Credentials: credentials,
	}
}

// withExcludeList returns a registration option that excludes existing credentials.
func withExcludeList(excludeList []protocol.CredentialDescriptor) webauthn.RegistrationOption {
	return webauthn.WithExclusions(excludeList)
}

// auditWebAuthnLoginFailure logs a failed WebAuthn login attempt (NIST AC-7, AU-2).
func (s *Server) auditWebAuthnLoginFailure(r *http.Request, userID, email *string, reason string) {
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  s.db.TenantID(),
		EventType: "auth.login.webauthn",
		UserID:    userID,
		UserEmail: email,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"reason":"` + reason + `"}`),
		Outcome:   "failure",
	})
}
