package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/middleware"
)

const (
	deviceCodeExpiry   = 15 * time.Minute
	devicePollInterval = 5 // seconds
)

// beginDeviceFlowRequest is the request body for POST /api/v1/auth/device/begin.
type beginDeviceFlowRequest struct {
	ClientID    string `json:"clientId"`
	ProfileName string `json:"profileName"`
}

// handleBeginDeviceFlow starts a CLI device authorization flow.
// POST /api/v1/auth/device/begin
func (s *Server) handleBeginDeviceFlow(w http.ResponseWriter, r *http.Request) {
	var req beginDeviceFlowRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if len(req.ClientID) > 255 {
		apierror.BadRequest(w, r, "clientId exceeds maximum length.", nil)
		return
	}
	if len(req.ProfileName) > 255 {
		apierror.BadRequest(w, r, "profileName exceeds maximum length.", nil)
		return
	}

	deviceCode, err := generateDeviceCode()
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	userCode := generateUserCode()

	dc := &db.DeviceCode{
		ID:          uuid.NewString(),
		TenantID:    s.db.TenantID(),
		DeviceCode:  deviceCode,
		UserCode:    userCode,
		ClientID:    req.ClientID,
		ProfileName: req.ProfileName,
		ExpiresAt:   time.Now().UTC().Add(deviceCodeExpiry).Format(time.RFC3339),
	}

	if err := s.db.CreateDeviceCode(r.Context(), dc); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Build verification URI
	scheme := "https"
	host := r.Host
	verificationURI := fmt.Sprintf("%s://%s/login?device=%s", scheme, host, userCode)

	s.db.InsertAuditLog(r.Context(), &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  s.db.TenantID(),
		EventType: "auth.device.flow_started",
		SourceIP:  strPtr(r.RemoteAddr),
		Outcome:   "success",
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deviceCode":      deviceCode,
		"userCode":        userCode,
		"verificationUri": verificationURI,
		"expiresIn":       int(deviceCodeExpiry.Seconds()),
		"interval":        devicePollInterval,
	})
}

// pollDeviceFlowRequest is the request body for POST /api/v1/auth/device/poll.
type pollDeviceFlowRequest struct {
	DeviceCode string `json:"deviceCode"`
}

// handlePollDeviceFlow checks if a device code has been authorized.
// POST /api/v1/auth/device/poll
func (s *Server) handlePollDeviceFlow(w http.ResponseWriter, r *http.Request) {
	var req pollDeviceFlowRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.DeviceCode == "" || len(req.DeviceCode) > 255 {
		apierror.BadRequest(w, r, "Invalid device code.", nil)
		return
	}

	ctx := r.Context()
	dc, err := s.db.GetDeviceCodeByDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if dc == nil {
		apierror.BadRequest(w, r, "Invalid or expired device code.", nil)
		return
	}

	// Check expiry
	expiresAt, err := time.Parse(time.RFC3339, dc.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		s.db.DeleteDeviceCode(ctx, dc.ID)
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "expired_token",
		})
		return
	}

	// Not yet authorized — tell client to keep polling
	if !dc.Authorized || dc.UserID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "authorization_pending",
		})
		return
	}

	// Authorized — look up the user and issue tokens
	user, err := s.db.GetUserByID(ctx, *dc.UserID)
	if err != nil || user == nil {
		apierror.Internal(w, r, fmt.Errorf("device flow: user not found after authorization"))
		return
	}

	// Get enabled services
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
		ID:        sessionID,
		TenantID:  user.TenantID,
		UserID:    user.ID,
		Type:      "cli",
		SourceIP:  strPtr(r.RemoteAddr),
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

	// Clean up the device code — single use
	s.db.DeleteDeviceCode(ctx, dc.ID)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  user.TenantID,
		EventType: "auth.device.token_issued",
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
			"method": "device_flow",
		},
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"tokenType":    "Bearer",
		"expiresIn":    900,
		"user": map[string]interface{}{
			"id":       user.ID,
			"email":    user.Email,
			"tenantId": user.TenantID,
			"roles":    []string{user.Role},
		},
	})
}

// authorizeDeviceRequest is the request body for POST /api/v1/auth/device/authorize.
type authorizeDeviceRequest struct {
	UserCode string `json:"userCode"`
}

// handleAuthorizeDevice authorizes a CLI device from the browser.
// POST /api/v1/auth/device/authorize
func (s *Server) handleAuthorizeDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFromCtx(r.Context())
	if claims == nil {
		apierror.Unauthorized(w, r, "Authentication required.", nil)
		return
	}

	var req authorizeDeviceRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if req.UserCode == "" || len(req.UserCode) > 20 {
		apierror.BadRequest(w, r, "Invalid user code.", nil)
		return
	}

	ctx := r.Context()
	dc, err := s.db.GetDeviceCodeByUserCode(ctx, strings.ToUpper(req.UserCode))
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if dc == nil {
		apierror.NotFound(w, r, "Invalid or expired user code.", nil)
		return
	}

	// Check expiry
	expiresAt, err := time.Parse(time.RFC3339, dc.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		s.db.DeleteDeviceCode(ctx, dc.ID)
		apierror.NotFound(w, r, "Invalid or expired user code.", nil)
		return
	}

	// Authorize
	if err := s.db.AuthorizeDeviceCode(ctx, dc.ID, claims.Subject); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  claims.TenantID,
		EventType: "auth.device.authorized",
		UserID:    &claims.Subject,
		SourceIP:  strPtr(r.RemoteAddr),
		Outcome:   "success",
	})

	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "authorized",
	})
}

// generateDeviceCode creates a CSPRNG device code (32 bytes hex).
func generateDeviceCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating device code: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// generateUserCode creates a human-readable user code (e.g., "ABCD-1234").
func generateUserCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1 for readability
	b := make([]byte, 8)
	rand.Read(b)
	code := make([]byte, 9) // 8 chars + 1 dash
	for i := 0; i < 4; i++ {
		code[i] = charset[int(b[i])%len(charset)]
	}
	code[4] = '-'
	for i := 4; i < 8; i++ {
		code[i+1] = charset[int(b[i])%len(charset)]
	}
	return string(code)
}
