package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

const recoveryCodeCount = 10

// handleGenerateRecoveryCodes generates 10 recovery codes for the authenticated user.
// POST /api/v1/auth/recovery/generate
//
// NIST IA-5: Recovery codes as backup authenticator.
// OWASP A07: Codes are Argon2id hashed before storage.
func (s *Server) handleGenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	ctx := r.Context()

	// Generate plaintext codes (crypto/rand — NIST SP 800-131A)
	plaintextCodes, err := auth.GenerateRecoveryCodes(recoveryCodeCount)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Hash each code with Argon2id (m=64MB, t=3, p=4)
	codes := make([]db.RecoveryCode, 0, recoveryCodeCount)
	now := db.Now()
	for _, plaintext := range plaintextCodes {
		hash, err := auth.HashPassword(plaintext)
		if err != nil {
			apierror.Internal(w, r, err)
			return
		}
		codes = append(codes, db.RecoveryCode{
			ID:        uuid.NewString(),
			TenantID:  claims.TenantID,
			UserID:    claims.Subject,
			CodeHash:  hash,
			CreatedAt: now,
		})
	}

	// Store hashes (replaces existing codes)
	if err := s.db.CreateRecoveryCodes(ctx, claims.Subject, codes); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit log (NIST AU-2)
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "recovery.generated",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Outcome:   "success",
	})

	// Return plaintext codes (shown once, never retrievable again)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"codes": plaintextCodes,
	})
}

// handleRecoveryCodeCount returns how many unused recovery codes the user has.
// GET /api/v1/auth/recovery/count
func (s *Server) handleRecoveryCodeCount(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	count, err := s.db.CountUnusedRecoveryCodes(r.Context(), claims.Subject)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": count,
	})
}

// recoveryVerifyRequest is the request body for POST /api/v1/auth/recovery/verify.
type recoveryVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// handleVerifyRecoveryCode verifies a recovery code and returns a scoped JWT.
// POST /api/v1/auth/recovery/verify
//
// Public endpoint (user is locked out and cannot authenticate normally).
// Returns a 5-minute JWT scoped to "passkey:register" only.
//
// NIST IA-5: Backup authenticator verification.
// OWASP A07: Identical error for all failure modes (no enumeration).
func (s *Server) handleVerifyRecoveryCode(w http.ResponseWriter, r *http.Request) {
	var req recoveryVerifyRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.Email == "" || req.Code == "" {
		// Identical error — no enumeration (OWASP A07)
		apierror.Unauthorized(w, r, "Invalid email or recovery code.", nil)
		return
	}

	ctx := r.Context()

	// Look up user by email — identical error for not-found (OWASP A07)
	user, err := s.db.GetUserByEmail(ctx, req.Email)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid email or recovery code.", nil)
		s.auditRecoveryFailure(r, nil, &req.Email, "user not found")
		return
	}

	// Load unused codes for the user
	codes, err := s.db.GetUnusedRecoveryCodesByUser(ctx, user.ID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	if len(codes) == 0 {
		apierror.Unauthorized(w, r, "Invalid email or recovery code.", nil)
		s.auditRecoveryFailure(r, &user.ID, &user.Email, "no unused codes")
		return
	}

	// Try each code hash with Argon2id verify (constant-time — NIST SP 800-131A)
	var matchedCode *db.RecoveryCode
	for i := range codes {
		match, verifyErr := auth.VerifyPassword(req.Code, codes[i].CodeHash)
		if verifyErr != nil {
			continue
		}
		if match {
			matchedCode = &codes[i]
			break
		}
	}

	if matchedCode == nil {
		apierror.Unauthorized(w, r, "Invalid email or recovery code.", nil)
		s.auditRecoveryFailure(r, &user.ID, &user.Email, "no matching code")
		return
	}

	// Mark the matched code as used
	if err := s.db.MarkRecoveryCodeUsed(ctx, matchedCode.ID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Issue 5-minute JWT scoped to passkey:register only
	token, err := s.jwtMgr.IssueScopedToken(
		user.ID,
		user.TenantID,
		[]string{"passkey:register"},
		5*time.Minute,
	)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit log (NIST AU-2)
	remaining, _ := s.db.CountUnusedRecoveryCodes(ctx, user.ID)
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  user.TenantID,
		EventType: "recovery.verified",
		UserID:    &user.ID,
		UserEmail: &user.Email,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"remaining":` + itoa(remaining) + `}`),
		Outcome:   "success",
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":     token,
		"expiresIn": 300,
		"scope":     "passkey:register",
		"remaining": remaining,
	})
}

// handleAdminRecoveryReset invalidates all passkeys and recovery codes for a user.
// POST /api/v1/users/{userId}/recovery/reset
//
// Admin-only: platform_owner or org_admin required.
// NIST IA-5: Staff-assisted credential reset.
// OWASP API5: Function-level authorization check.
func (s *Server) handleAdminRecoveryReset(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	// RBAC: admin+ required (NIST AC-6, OWASP API5)
	if !isAdmin(claims.Roles) {
		apierror.Forbidden(w, r, "Insufficient permissions.", nil)
		return
	}

	targetUserID := chi.URLParam(r, "userId")
	if _, err := uuid.Parse(targetUserID); err != nil {
		apierror.BadRequest(w, r, "Invalid user ID format.", nil)
		return
	}

	ctx := r.Context()

	// Verify target user exists and belongs to the same tenant
	targetUser, err := s.db.GetUserByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			apierror.NotFound(w, r, "User not found.", nil)
			return
		}
		apierror.Internal(w, r, err)
		return
	}
	if targetUser.TenantID != claims.TenantID {
		apierror.NotFound(w, r, "User not found.", nil)
		return
	}

	// Delete all passkeys for the target user
	if err := s.db.DeletePasskeysByUser(ctx, targetUserID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Delete all recovery codes for the target user
	if err := s.db.DeleteRecoveryCodesByUser(ctx, targetUserID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Audit log (NIST AU-2)
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "recovery.admin_reset",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"targetUserId":"` + targetUserID + `","targetEmail":"` + targetUser.Email + `"}`),
		Outcome:   "success",
	})

	s.publishEvent(ctx, claims.TenantID, Event{
		Channel: "users",
		Type:    "user.recovery_reset",
		Data: map[string]string{
			"userId":       targetUserID,
			"resetByUserId": claims.Subject,
		},
	})

	w.WriteHeader(http.StatusNoContent)
}

// auditRecoveryFailure logs a failed recovery code attempt (NIST AC-7, AU-2).
func (s *Server) auditRecoveryFailure(r *http.Request, userID, email *string, reason string) {
	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  s.db.TenantID(),
		EventType: "recovery.verify",
		UserID:    userID,
		UserEmail: email,
		SourceIP:  strPtr(r.RemoteAddr),
		Details:   strPtr(`{"reason":"` + reason + `"}`),
		Outcome:   "failure",
	})
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
