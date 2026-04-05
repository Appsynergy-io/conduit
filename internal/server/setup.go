package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/appsynergy-io/conduit/internal/db"
)

// setupToken holds the plaintext setup token in memory (never persisted in plain).
// Set during server startup, cleared after setup completes in production.
var setupToken string

// InitSetup generates a setup token and stores its hash in the DB.
// Returns the plaintext token (printed to stdout by the caller).
func (s *Server) InitSetup(ctx context.Context) (string, error) {

	// Check if setup is already complete
	complete, err := s.db.IsSetupComplete(ctx)
	if err != nil {
		return "", err
	}
	if complete {
		return "", nil // Already set up — no token needed
	}

	// Check if setup state already exists (server restarted during setup)
	state, err := s.db.GetSetupState(ctx)
	if err != nil {
		return "", err
	}

	token, err := auth.GenerateSetupToken()
	if err != nil {
		return "", err
	}

	tokenHash := hashSetupToken(token)

	if state == nil {
		// First start — create setup state
		if err := s.db.CreateSetupState(ctx, tokenHash); err != nil {
			return "", err
		}
	}

	setupToken = token
	return token, nil
}

// handleSetupStatus returns whether setup is required.
// GET /api/v1/setup/status
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	complete, err := s.db.IsSetupComplete(r.Context())
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if complete {
		apierror.NotFound(w, r, "Setup already completed.", nil)
		return
	}

	state, err := s.db.GetSetupState(r.Context())
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	resp := map[string]any{
		"setupRequired": true,
		"currentStep":   "domain_config",
	}
	if state != nil {
		resp["currentStep"] = state.CurrentStep
		resp["completedSteps"] = json.RawMessage(state.CompletedSteps)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// configureRequest is the request body for POST /api/v1/setup/configure.
type configureRequest struct {
	SetupToken       string `json:"setupToken"`
	Domain           string `json:"domain"`
	AdminEmail       string `json:"adminEmail"`
	OrganizationName string `json:"organizationName"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
}

// handleSetupConfigure processes the setup wizard configuration.
// POST /api/v1/setup/configure
func (s *Server) handleSetupConfigure(w http.ResponseWriter, r *http.Request) {
	complete, err := s.db.IsSetupComplete(r.Context())
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if complete {
		apierror.NotFound(w, r, "Setup already completed.", nil)
		return
	}

	var req configureRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate required fields
	if req.SetupToken == "" || req.Domain == "" || req.AdminEmail == "" ||
		req.OrganizationName == "" || req.FirstName == "" || req.LastName == "" {
		apierror.BadRequest(w, r, "All fields are required.", nil)
		return
	}

	// Validate setup token (constant-time comparison — NIST IA-12)
	if !verifySetupToken(req.SetupToken) {
		s.logger.WarnContext(r.Context(), "invalid setup token attempt",
			"source_ip", r.RemoteAddr,
		)
		// Identical error to prevent token enumeration
		apierror.Unauthorized(w, r, "Invalid setup token.", nil)
		return
	}

	ctx := r.Context()

	// Create tenant
	tenantID := uuid.NewString()
	if err := s.db.CreateTenant(ctx, tenantID, req.OrganizationName); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Create admin user
	userID := uuid.NewString()
	user := &db.User{
		ID:        userID,
		TenantID:  tenantID,
		Email:     req.AdminEmail,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Role:      "platform_owner",
		Status:    "active",
	}

	// In dev mode, hash the setup token as the password
	if s.cfg.Server.Mode == "dev" {
		hash, err := auth.HashPassword(req.SetupToken)
		if err != nil {
			apierror.Internal(w, r, err)
			return
		}
		user.PasswordHash = &hash
	}

	if err := s.db.CreateUser(ctx, user); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Seed remote-access service
	serviceID := uuid.NewString()
	if err := s.db.CreateService(ctx, serviceID, "remote-access",
		"Conduit Remote Access",
		"Secure remote shell, file management, and agent orchestration."); err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if err := s.db.EnableServiceForTenant(ctx, tenantID, serviceID); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Update setup state
	if err := s.db.UpdateSetupStep(ctx, "passkey_registration",
		`["domain_config","admin_account"]`); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Get enabled services for JWT claims
	services, err := s.db.ListServices(ctx, tenantID)
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

	// Create session so the admin can register a passkey immediately
	sessionID := uuid.NewString()
	session := &db.Session{
		ID:        sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Type:      "web",
		SourceIP:  strPtr(r.RemoteAddr),
		ExpiresAt: time.Now().UTC().Add(s.jwtMgr.RefreshTTL()).Format(time.RFC3339),
	}
	if err := s.db.CreateSession(ctx, session); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Issue JWT so the browser can call WebAuthn register endpoints (NIST IA-2)
	accessToken, err := s.jwtMgr.IssueAccessToken(
		userID, tenantID, sessionID,
		[]string{"platform_owner"}, serviceSlugs,
	)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Set httpOnly cookies (NIST SC-23, OWASP V3)
	setAuthCookie(w, accessToken, int(s.jwtMgr.AccessTTL().Seconds()))
	refreshToken, err := s.jwtMgr.IssueRefreshToken(userID, tenantID, sessionID)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	setRefreshCookie(w, refreshToken, int(s.jwtMgr.RefreshTTL().Seconds()))

	// Audit log
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		EventType: "setup.configured",
		UserID:    &userID,
		UserEmail: &req.AdminEmail,
		SourceIP:  strPtr(r.RemoteAddr),
		Outcome:   "success",
	})

	slog.InfoContext(ctx, "setup configured",
		"tenant_id", tenantID,
		"admin_email", req.AdminEmail,
		"domain", req.Domain,
	)

	redirectURL := "https://" + req.Domain + "/setup/passkey"
	if s.cfg.Server.Mode == "dev" {
		redirectURL = "https://localhost" + s.cfg.Server.HTTPAddr + "/setup/passkey"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":           "provisioning",
		"message":          "Configuration accepted. Complete passkey registration to finish setup.",
		"redirectUrl":      redirectURL,
		"estimatedSeconds": 5,
	})
}

// handleSetupPasskey finalizes setup after passkey registration (or skip in dev).
// POST /api/v1/setup/passkey
// The setup token authenticates this request (not JWT) — this is called before
// normal auth is fully established. Passkey registration itself uses the JWT
// issued by handleSetupConfigure via the standard WebAuthn endpoints.
func (s *Server) handleSetupPasskey(w http.ResponseWriter, r *http.Request) {
	complete, err := s.db.IsSetupComplete(r.Context())
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if complete {
		apierror.NotFound(w, r, "Setup already completed.", nil)
		return
	}

	var req struct {
		SetupToken string `json:"setupToken"`
	}
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	if !verifySetupToken(req.SetupToken) {
		apierror.Unauthorized(w, r, "Invalid setup token.", nil)
		return
	}

	ctx := r.Context()
	if err := s.db.CompleteSetup(ctx); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	if s.cfg.Server.Mode == "dev" {
		// Dev mode: keep setup token for password auth
		slog.InfoContext(ctx, "setup completed (dev mode)")
	} else {
		// Production: clear setup token from memory (NIST IA-5, single-use)
		setupToken = ""
		slog.InfoContext(ctx, "setup completed")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "setup_complete",
		"message": "Setup complete.",
	})
}

// hashSetupToken computes SHA-256 hash of the setup token.
func hashSetupToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// verifySetupToken compares a candidate token against the in-memory token
// using constant-time comparison (NIST IA-12).
func verifySetupToken(candidate string) bool {
	if setupToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(setupToken)) == 1
}

func strPtr(s string) *string {
	return &s
}
