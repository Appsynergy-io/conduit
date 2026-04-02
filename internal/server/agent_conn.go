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
	AgentID   string
	TenantID  string
	Hostname  string
	Mux       protocol.FrameMux
	Transport string // "websocket" or "quic"
	cancel    context.CancelFunc
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

	readErr := make(chan error, 1)
	go func() {
		readErr <- mux.ReadLoop(ctx)
	}()

	agent, hello, err := s.authenticateAgent(ctx, mux, readErr, r.RemoteAddr)
	if err != nil {
		s.logger.Warn("agent WebSocket auth failed", "error", err, "remote_addr", r.RemoteAddr)
		mux.Close()
		return
	}

	s.onAgentAuthenticated(ctx, cancel, mux, readErr, agent, hello, "websocket", r.RemoteAddr)
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
		// Agent metrics — update last seen + publish to EventBus (NIST SI-4)
		s.db.UpdateAgentStatus(ctx, agent.AgentID, "online")

		var metrics protocol.AgentInfoPayload
		if err := protocol.UnmarshalPayload(f.Payload, &metrics); err == nil {
			s.publishEvent(ctx, agent.TenantID, Event{
				Channel: "metrics",
				Type:    "agent.metrics",
				Data: map[string]interface{}{
					"agentId":    agent.AgentID,
					"hostname":   agent.Hostname,
					"cpuPercent": metrics.CPUPercent,
					"memTotal":   metrics.MemTotal,
					"memUsed":    metrics.MemUsed,
					"diskTotal":  metrics.DiskTotal,
					"diskUsed":   metrics.DiskUsed,
					"uptime":     metrics.Uptime,
					"loadAvg1":   metrics.LoadAvg1,
					"loadAvg5":   metrics.LoadAvg5,
					"loadAvg15":  metrics.LoadAvg15,
				},
			})
		}

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
func sendAuthOK(ctx context.Context, mux protocol.FrameMux) error {
	f := &protocol.Frame{Type: protocol.FrameAuthOK}
	return mux.Send(ctx, f)
}

// sendAuthReject sends an AUTH_REJECT frame with a reason.
func sendAuthReject(ctx context.Context, mux protocol.FrameMux, reason string) {
	f := &protocol.Frame{
		Type:    protocol.FrameAuthReject,
		Payload: []byte(reason),
	}
	mux.Send(ctx, f)
}

// authenticateAgent performs the CWP HELLO+AUTH handshake and returns the
// authenticated agent. This is transport-agnostic — works over both WebSocket and QUIC.
func (s *Server) authenticateAgent(ctx context.Context, mux protocol.FrameMux, readErr <-chan error, remoteAddr string) (*db.Agent, *protocol.HelloPayload, error) {
	// Wait for HELLO frame (5 second timeout)
	helloCtx, helloCancel := context.WithTimeout(ctx, 5*time.Second)
	defer helloCancel()

	var hello *protocol.Frame
	select {
	case f := <-mux.Global():
		if f.Type != protocol.FrameHello {
			sendAuthReject(ctx, mux, "Expected HELLO frame")
			return nil, nil, fmt.Errorf("expected HELLO, got %s", f.Type.String())
		}
		hello = f
	case <-helloCtx.Done():
		return nil, nil, fmt.Errorf("HELLO timeout")
	case err := <-readErr:
		return nil, nil, fmt.Errorf("disconnected during handshake: %w", err)
	}

	var helloPayload protocol.HelloPayload
	if err := protocol.UnmarshalPayload(hello.Payload, &helloPayload); err != nil {
		sendAuthReject(ctx, mux, "Invalid HELLO payload")
		return nil, nil, fmt.Errorf("invalid HELLO payload: %w", err)
	}

	if helloPayload.AgentID == "" {
		sendAuthReject(ctx, mux, "Missing agent ID")
		return nil, nil, fmt.Errorf("missing agent ID")
	}

	// Wait for AUTH frame (5 second timeout)
	authCtx, authCancel := context.WithTimeout(ctx, 5*time.Second)
	defer authCancel()

	var authFrame *protocol.Frame
	select {
	case f := <-mux.Global():
		if f.Type != protocol.FrameAuth {
			sendAuthReject(ctx, mux, "Expected AUTH frame")
			return nil, nil, fmt.Errorf("expected AUTH, got %s", f.Type.String())
		}
		authFrame = f
	case <-authCtx.Done():
		return nil, nil, fmt.Errorf("AUTH timeout")
	case err := <-readErr:
		return nil, nil, fmt.Errorf("disconnected during auth: %w", err)
	}

	var authPayload protocol.AuthPayload
	if err := protocol.UnmarshalPayload(authFrame.Payload, &authPayload); err != nil {
		sendAuthReject(ctx, mux, "Invalid AUTH payload")
		return nil, nil, fmt.Errorf("invalid AUTH payload: %w", err)
	}

	agent, err := s.db.GetAgentByID(ctx, helloPayload.AgentID)
	if err != nil {
		sendAuthReject(ctx, mux, "Internal error")
		return nil, nil, fmt.Errorf("database error: %w", err)
	}
	if agent == nil {
		sendAuthReject(ctx, mux, "Unknown agent")
		return nil, nil, fmt.Errorf("unknown agent %s", helloPayload.AgentID)
	}

	if !verifyAgentAuth(agent.AgentKeyHash, hello.Payload, authPayload.Nonce, authPayload.Signature) {
		sendAuthReject(ctx, mux, "Authentication failed")

		s.db.InsertAuditLog(ctx, &db.AuditEntry{
			ID:            uuid.NewString(),
			TenantID:      agent.TenantID,
			EventType:     "agent.auth_failed",
			AgentID:       &agent.ID,
			AgentHostname: &agent.Hostname,
			SourceIP:      strPtr(remoteAddr),
			Outcome:       "failure",
		})

		return nil, nil, fmt.Errorf("auth failed for agent %s", helloPayload.AgentID)
	}

	if err := sendAuthOK(ctx, mux); err != nil {
		return nil, nil, fmt.Errorf("sending AUTH_OK: %w", err)
	}

	return agent, &helloPayload, nil
}

// onAgentAuthenticated handles post-auth setup: DB update, registry, audit, events.
// Returns after the agent loop completes (disconnect).
func (s *Server) onAgentAuthenticated(ctx context.Context, cancel context.CancelFunc, mux protocol.FrameMux, readErr <-chan error, agent *db.Agent, hello *protocol.HelloPayload, transport, remoteAddr string) {
	if err := s.db.UpdateAgentConnection(ctx, agent.ID, "online", transport, remoteAddr); err != nil {
		s.logger.Error("failed to update agent connection", "error", err)
	}

	if hello.Hostname != "" || hello.OS != "" || hello.Arch != "" || hello.Version != "" {
		s.db.UpdateAgentInfo(ctx, agent.ID, hello.Hostname, hello.OS, hello.Arch, hello.Version)
	}

	connAgent := &ConnectedAgent{
		AgentID:   agent.ID,
		TenantID:  agent.TenantID,
		Hostname:  agent.Hostname,
		Mux:       mux,
		Transport: transport,
		cancel:    cancel,
	}
	s.agentRegistry.Register(connAgent)

	s.logger.Info("agent connected",
		"agent_id", agent.ID,
		"hostname", hello.Hostname,
		"transport", transport,
	)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      agent.TenantID,
		EventType:     "agent.connected",
		AgentID:       &agent.ID,
		AgentHostname: &hello.Hostname,
		SourceIP:      strPtr(remoteAddr),
		Details:       strPtr(fmt.Sprintf(`{"transport":"%s","os":"%s","arch":"%s","version":"%s"}`, transport, hello.OS, hello.Arch, hello.Version)),
		Outcome:       "success",
	})

	s.publishEvent(ctx, agent.TenantID, Event{
		Channel: "agents",
		Type:    "agent.connected",
		Data: map[string]string{
			"agentId":   agent.ID,
			"hostname":  hello.Hostname,
			"transport": transport,
		},
	})

	s.runAgentLoop(ctx, connAgent, readErr)

	// Agent disconnected — cleanup
	s.sessionMgr.CleanupAgentSessions(ctx, agent.ID)
	s.agentRegistry.Unregister(agent.ID)
	s.db.UpdateAgentStatus(ctx, agent.ID, "offline")

	s.logger.Info("agent disconnected", "agent_id", agent.ID, "hostname", agent.Hostname)

	s.db.InsertAuditLog(ctx, &db.AuditEntry{
		ID:            uuid.NewString(),
		TenantID:      agent.TenantID,
		EventType:     "agent.disconnected",
		AgentID:       &agent.ID,
		AgentHostname: &agent.Hostname,
		SourceIP:      strPtr(remoteAddr),
		Outcome:       "success",
	})

	s.publishEvent(ctx, agent.TenantID, Event{
		Channel: "agents",
		Type:    "agent.disconnected",
		Data: map[string]string{
			"agentId":  agent.ID,
			"hostname": agent.Hostname,
		},
	})
}
