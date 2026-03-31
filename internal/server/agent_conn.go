package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/appsynergy-io/conduit/internal/protocol"
)

// AgentRegistry tracks connected agents and their multiplexed connections.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*ConnectedAgent // agentID → connection
	logger *slog.Logger
}

// ConnectedAgent represents a live agent connection.
type ConnectedAgent struct {
	AgentID  string
	TenantID string
	Hostname string
	Mux      *protocol.Mux
	cancel   context.CancelFunc
}

// NewAgentRegistry creates a new registry for tracking connected agents.
func NewAgentRegistry(logger *slog.Logger) *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*ConnectedAgent),
		logger: logger,
	}
}

// Get returns a connected agent by ID, or nil if not found.
func (ar *AgentRegistry) Get(agentID string) *ConnectedAgent {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	return ar.agents[agentID]
}

// Register adds a connected agent to the registry.
func (ar *AgentRegistry) Register(agent *ConnectedAgent) {
	ar.mu.Lock()
	// If there's an existing connection, close it (agent reconnected)
	if existing, ok := ar.agents[agent.AgentID]; ok {
		existing.cancel()
		existing.Mux.Close()
	}
	ar.agents[agent.AgentID] = agent
	ar.mu.Unlock()
}

// Unregister removes a connected agent from the registry.
func (ar *AgentRegistry) Unregister(agentID string) {
	ar.mu.Lock()
	delete(ar.agents, agentID)
	ar.mu.Unlock()
}

// Count returns the number of connected agents.
func (ar *AgentRegistry) Count() int {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	return len(ar.agents)
}

// List returns all connected agent IDs.
func (ar *AgentRegistry) List() []string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	ids := make([]string, 0, len(ar.agents))
	for id := range ar.agents {
		ids = append(ids, id)
	}
	return ids
}

// handleAgentConnect handles inbound agent WebSocket connections.
// The agent authenticates via CWP HELLO + AUTH frames.
// GET /agent/v1/connect (WebSocket upgrade)
func (s *Server) handleAgentConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"conduit-cwp-v1"},
	})
	if err != nil {
		s.logger.Error("agent websocket upgrade failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	mux := protocol.NewMux(conn, s.logger)

	// Start read loop in background
	readErr := make(chan error, 1)
	go func() {
		readErr <- mux.ReadLoop(ctx)
	}()

	// Wait for HELLO frame (5 second timeout)
	helloCtx, helloCancel := context.WithTimeout(ctx, 5*time.Second)
	defer helloCancel()

	var hello *protocol.Frame
	select {
	case f := <-mux.Global():
		if f.Type != protocol.FrameHello {
			s.logger.Warn("expected HELLO frame, got", "type", f.Type.String())
			sendAuthReject(ctx, mux, "Expected HELLO frame")
			mux.Close()
			return
		}
		hello = f
	case <-helloCtx.Done():
		s.logger.Warn("agent HELLO timeout")
		mux.Close()
		return
	case err := <-readErr:
		s.logger.Debug("agent disconnected during handshake", "error", err)
		return
	}

	// Parse HELLO payload
	var helloPayload protocol.HelloPayload
	if err := protocol.UnmarshalPayload(hello.Payload, &helloPayload); err != nil {
		s.logger.Warn("invalid HELLO payload", "error", err)
		sendAuthReject(ctx, mux, "Invalid HELLO payload")
		mux.Close()
		return
	}

	if helloPayload.AgentID == "" {
		sendAuthReject(ctx, mux, "Missing agent ID")
		mux.Close()
		return
	}

	// Wait for AUTH frame (5 second timeout)
	authCtx, authCancel := context.WithTimeout(ctx, 5*time.Second)
	defer authCancel()

	var authFrame *protocol.Frame
	select {
	case f := <-mux.Global():
		if f.Type != protocol.FrameAuth {
			s.logger.Warn("expected AUTH frame, got", "type", f.Type.String())
			sendAuthReject(ctx, mux, "Expected AUTH frame")
			mux.Close()
			return
		}
		authFrame = f
	case <-authCtx.Done():
		s.logger.Warn("agent AUTH timeout")
		mux.Close()
		return
	case err := <-readErr:
		s.logger.Debug("agent disconnected during auth", "error", err)
		return
	}

	// Parse AUTH payload
	var authPayload protocol.AuthPayload
	if err := protocol.UnmarshalPayload(authFrame.Payload, &authPayload); err != nil {
		s.logger.Warn("invalid AUTH payload", "error", err)
		sendAuthReject(ctx, mux, "Invalid AUTH payload")
		mux.Close()
		return
	}

	// Look up agent
	agent, err := s.db.GetAgentByID(ctx, helloPayload.AgentID)
	if err != nil {
		s.logger.Error("database error during agent auth", "error", err)
		sendAuthReject(ctx, mux, "Internal error")
		mux.Close()
		return
	}
	if agent == nil {
		s.logger.Warn("unknown agent", "agent_id", helloPayload.AgentID)
		sendAuthReject(ctx, mux, "Unknown agent")
		mux.Close()
		return
	}

	// Verify HMAC-SHA256 signature
	// Agent computes: HMAC-SHA256(key=agentKey, message=helloPayload || nonce)
	if !verifyAgentAuth(agent.AgentKeyHash, hello.Payload, authPayload.Nonce, authPayload.Signature) {
		s.logger.Warn("agent auth failed", "agent_id", helloPayload.AgentID)
		sendAuthReject(ctx, mux, "Authentication failed")

		s.db.InsertAuditLog(ctx, &db.AuditEntry{
			ID:            uuid.NewString(),
			TenantID:      agent.TenantID,
			EventType:     "agent.auth_failed",
			AgentID:       &agent.ID,
			AgentHostname: &agent.Hostname,
			SourceIP:      strPtr(r.RemoteAddr),
			Outcome:       "failure",
		})

		mux.Close()
		return
	}

	// Auth successful — send AUTH_OK
	if err := sendAuthOK(ctx, mux); err != nil {
		s.logger.Error("failed to send AUTH_OK", "error", err)
		mux.Close()
		return
	}

	// Update agent status in DB
	ip := r.RemoteAddr
	if err := s.db.UpdateAgentConnection(ctx, agent.ID, "online", "websocket", ip); err != nil {
		s.logger.Error("failed to update agent connection", "error", err)
	}

	// Update agent info if provided
	if helloPayload.Hostname != "" || helloPayload.OS != "" || helloPayload.Arch != "" || helloPayload.Version != "" {
		s.db.UpdateAgentInfo(ctx, agent.ID, helloPayload.Hostname, helloPayload.OS, helloPayload.Arch, helloPayload.Version)
	}

	// Register in agent registry
	connAgent := &ConnectedAgent{
		AgentID:  agent.ID,
		TenantID: agent.TenantID,
		Hostname: agent.Hostname,
		Mux:      mux,
		cancel:   cancel,
	}
	s.agentRegistry.Register(connAgent)

	s.logger.Info("agent connected",
		"agent_id", agent.ID,
		"hostname", helloPayload.Hostname,
		"transport", "websocket",
	)

	// Audit
	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      agent.TenantID,
		EventType:     "agent.connected",
		AgentID:       &agent.ID,
		AgentHostname: &helloPayload.Hostname,
		SourceIP:      strPtr(ip),
		Details:       strPtr(fmt.Sprintf(`{"transport":"websocket","os":"%s","arch":"%s","version":"%s"}`, helloPayload.OS, helloPayload.Arch, helloPayload.Version)),
		Outcome:       "success",
	})

	// Publish connect event
	if s.eventBus != nil {
		s.eventBus.Publish(agent.TenantID, Event{
			Channel: "agents",
			Type:    "agent.connected",
			Data: map[string]string{
				"agentId":   agent.ID,
				"hostname":  helloPayload.Hostname,
				"transport": "websocket",
			},
		})
	}

	// Run the agent connection loop — handles PING/PONG and routes frames
	s.runAgentLoop(ctx, connAgent, readErr)

	// Agent disconnected — cleanup
	s.agentRegistry.Unregister(agent.ID)
	s.db.UpdateAgentStatus(ctx, agent.ID, "offline")

	s.logger.Info("agent disconnected", "agent_id", agent.ID, "hostname", agent.Hostname)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      agent.TenantID,
		EventType:     "agent.disconnected",
		AgentID:       &agent.ID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(ip),
		Outcome:       "success",
	})

	if s.eventBus != nil {
		s.eventBus.Publish(agent.TenantID, Event{
			Channel: "agents",
			Type:    "agent.disconnected",
			Data: map[string]string{
				"agentId":  agent.ID,
				"hostname": agent.Hostname,
			},
		})
	}
}

// runAgentLoop processes frames from a connected agent until disconnect.
func (s *Server) runAgentLoop(ctx context.Context, agent *ConnectedAgent, readErr <-chan error) {
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case f, ok := <-agent.Mux.Global():
			if !ok {
				return
			}
			s.handleAgentFrame(ctx, agent, f)

		case <-pingTicker.C:
			ping := &protocol.Frame{Type: protocol.FramePing}
			if err := agent.Mux.Send(ctx, ping); err != nil {
				s.logger.Debug("ping failed", "agent_id", agent.AgentID, "error", err)
				return
			}

		case err := <-readErr:
			if err != nil {
				s.logger.Debug("agent read error", "agent_id", agent.AgentID, "error", err)
			}
			return

		case <-ctx.Done():
			return
		}
	}
}

// handleAgentFrame processes a single frame from a connected agent.
func (s *Server) handleAgentFrame(ctx context.Context, agent *ConnectedAgent, f *protocol.Frame) {
	switch f.Type {
	case protocol.FramePong:
		// Update last seen
		s.db.UpdateAgentStatus(ctx, agent.AgentID, "online")

	case protocol.FrameAgentInfo:
		// Agent metrics — update last seen, could store metrics
		s.db.UpdateAgentStatus(ctx, agent.AgentID, "online")

	default:
		s.logger.Debug("unhandled agent frame",
			"agent_id", agent.AgentID,
			"type", f.Type.String(),
			"stream_id", f.StreamID,
		)
	}
}

// verifyAgentAuth verifies the agent's HMAC-SHA256 signature.
// The agent signs: HELLO_payload || nonce with its raw key.
// We compare against the stored SHA-256 hash of the key.
func verifyAgentAuth(storedKeyHash string, helloPayload []byte, nonce, signature string) bool {
	// Decode the signature
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	// The stored hash is SHA-256(agentKey). We can't recover the key from
	// the hash, so the agent must sign with the raw key and we verify by
	// trying all possible... Actually, we need to rethink this.
	//
	// The auth flow works as follows:
	// 1. Agent has raw key (from registration)
	// 2. Server stores SHA-256(key) as agentKeyHash
	// 3. Agent computes HMAC-SHA256(key, hello_payload || nonce) and sends it
	// 4. Server needs to verify this, but only has SHA-256(key)
	//
	// Solution: Store the HMAC key itself (not a one-way hash) since HMAC keys
	// need to be available for verification. For now, use the stored hash AS
	// the HMAC key (both sides derive the same HMAC key from the raw key).
	//
	// Protocol: HMAC-SHA256(key=SHA256(agentKey), msg=helloPayload||nonce)
	// This way the server uses storedKeyHash as the HMAC key.

	storedHashBytes, err := hex.DecodeString(storedKeyHash)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, storedHashBytes)
	mac.Write(helloPayload)
	mac.Write([]byte(nonce))
	expected := mac.Sum(nil)

	return hmac.Equal(sigBytes, expected)
}

// sendAuthOK sends an AUTH_OK frame to the agent.
func sendAuthOK(ctx context.Context, mux *protocol.Mux) error {
	f := &protocol.Frame{Type: protocol.FrameAuthOK}
	return mux.Send(ctx, f)
}

// sendAuthReject sends an AUTH_REJECT frame with a reason.
func sendAuthReject(ctx context.Context, mux *protocol.Mux, reason string) {
	f := &protocol.Frame{
		Type:    protocol.FrameAuthReject,
		Payload: []byte(reason),
	}
	mux.Send(ctx, f)
}
