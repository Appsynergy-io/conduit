package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/apierror"
	"github.com/appsynergy-io/conduit/internal/db"
)

// agentRegisterRequest is the body for POST /api/v1/agents/register.
type agentRegisterRequest struct {
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
}

// agentRegisterResponse is returned on successful registration.
type agentRegisterResponse struct {
	AgentID   string `json:"agentId"`
	AgentKey  string `json:"agentKey"`
	TenantID  string `json:"tenantId"`
	ServerURL string `json:"serverUrl,omitempty"`
}

// handleAgentRegister validates a join token and creates an agent record.
// POST /api/v1/agents/register
func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req agentRegisterRequest
	if err := decodeJSONStrict(r, &req); err != nil {
		apierror.BadRequest(w, r, "Invalid request body.", err)
		return
	}

	// Validate required fields
	if req.Token == "" {
		apierror.BadRequest(w, r, "token is required.", nil)
		return
	}
	if req.Hostname == "" || len(req.Hostname) > 255 {
		apierror.BadRequest(w, r, "hostname is required (max 255 characters).", nil)
		return
	}
	if len(req.OS) > 50 || len(req.Arch) > 50 || len(req.Version) > 50 {
		apierror.BadRequest(w, r, "os, arch, and version must be 50 characters or less.", nil)
		return
	}

	// Decode the token to get the raw bytes
	tokenBytes, err := base64.RawURLEncoding.DecodeString(req.Token)
	if err != nil {
		apierror.Unauthorized(w, r, "Invalid join token.", nil)
		return
	}

	// Compute HMAC-SHA256 hash to look up the token
	mac := hmac.New(sha256.New, []byte("conduit-join-token"))
	mac.Write(tokenBytes)
	tokenHash := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	ctx := r.Context()
	joinToken, err := s.db.GetJoinTokenByHash(ctx, tokenHash)
	if err != nil {
		apierror.Internal(w, r, err)
		return
	}
	if joinToken == nil {
		apierror.Unauthorized(w, r, "Invalid join token.", nil)
		return
	}

	// Check revoked
	if joinToken.Revoked != 0 {
		apierror.Unauthorized(w, r, "Join token has been revoked.", nil)
		return
	}

	// Check expiry
	if joinToken.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *joinToken.ExpiresAt)
		if err == nil && time.Now().UTC().After(expiresAt) {
			apierror.Unauthorized(w, r, "Join token has expired.", nil)
			return
		}
	}

	// Check max uses
	if joinToken.MaxUses != nil && joinToken.UsedCount >= *joinToken.MaxUses {
		apierror.Unauthorized(w, r, "Join token has reached maximum uses.", nil)
		return
	}

	// Generate agent credentials (32 bytes, CSPRNG)
	agentKeyBytes := make([]byte, 32)
	if _, err := rand.Read(agentKeyBytes); err != nil {
		apierror.Internal(w, r, fmt.Errorf("generating agent key: %w", err))
		return
	}
	agentKey := hex.EncodeToString(agentKeyBytes)

	// Hash the agent key for storage
	keyHash := sha256.Sum256(agentKeyBytes)
	agentKeyHash := hex.EncodeToString(keyHash[:])

	agentID := uuid.NewString()

	var osPtr, archPtr, versionPtr *string
	if req.OS != "" {
		osPtr = &req.OS
	}
	if req.Arch != "" {
		archPtr = &req.Arch
	}
	if req.Version != "" {
		versionPtr = &req.Version
	}

	ipAddr := r.RemoteAddr
	agent := &db.Agent{
		ID:           agentID,
		TenantID:     joinToken.TenantID,
		Hostname:     req.Hostname,
		OS:           osPtr,
		Arch:         archPtr,
		Labels:       joinToken.Labels,
		IP:           &ipAddr,
		AgentKeyHash: agentKeyHash,
		Status:       "offline",
		Version:      versionPtr,
	}

	if err := s.db.CreateAgent(ctx, agent); err != nil {
		apierror.Internal(w, r, err)
		return
	}

	// Increment token usage
	if err := s.db.IncrementTokenUsage(ctx, joinToken.ID); err != nil {
		s.logger.Error("failed to increment token usage", "error", err, "token_id", joinToken.ID)
	}

	// Revoke single-use tokens after successful join
	if joinToken.Type == "single_use" {
		if err := s.db.RevokeJoinToken(ctx, joinToken.ID); err != nil {
			s.logger.Error("failed to revoke single-use token", "error", err, "token_id", joinToken.ID)
		}
	}

	// Audit log
	labelsStr := ""
	if joinToken.Labels != nil {
		labelsStr = *joinToken.Labels
	}
	detailsMap := map[string]string{
		"agent_id":   agentID,
		"hostname":   req.Hostname,
		"token_id":   joinToken.ID,
		"token_type": joinToken.Type,
		"labels":     labelsStr,
	}
	detailsJSON, _ := json.Marshal(detailsMap)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      joinToken.TenantID,
		EventType:     "agent.registered",
		AgentID:       &agentID,
		AgentHostname: &req.Hostname,
		SourceIP:      &ipAddr,
		Details:       strPtr(string(detailsJSON)),
		Outcome:       "success",
	})

	// Publish event to EventBus + webhooks
	s.publishEvent(ctx, joinToken.TenantID, Event{
		Channel: "agents",
		Type:    "agent.registered",
		Data: map[string]string{
			"agentId":  agentID,
			"hostname": req.Hostname,
		},
	})

	writeJSON(w, http.StatusCreated, agentRegisterResponse{
		AgentID:  agentID,
		AgentKey: agentKey,
		TenantID: joinToken.TenantID,
	})
}
