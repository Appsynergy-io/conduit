package server

import "github.com/appsynergy-io/conduit/internal/protocol"

// SetSetupTokenForTesting exposes the package-level setupToken for external tests.
// Only compiled during test runs (_test.go suffix).
func SetSetupTokenForTesting(token string) {
	setupToken = token
}

// VerifyAgentAuthForTesting exposes verifyAgentAuth for external tests.
func VerifyAgentAuthForTesting(storedKeyHash string, helloPayload []byte, nonce, signature string) bool {
	return verifyAgentAuth(storedKeyHash, helloPayload, nonce, signature)
}

// StoreAgentMetricsForTesting caches metrics for an agent (test helper).
func (s *Server) StoreAgentMetricsForTesting(agentID string, m *protocol.AgentInfoPayload) {
	s.agentMetrics.Store(agentID, m)
}

// RegisterAgentForTesting adds a fake connected agent to the registry (test helper).
func (s *Server) RegisterAgentForTesting(agentID, tenantID, hostname string, mux protocol.FrameMux) {
	s.agentRegistry.Register(&ConnectedAgent{
		AgentID:  agentID,
		TenantID: tenantID,
		Hostname: hostname,
		Mux:      mux,
		cancel:   func() {},
	})
}

// UnregisterAgentForTesting removes a fake connected agent from the registry (test helper).
func (s *Server) UnregisterAgentForTesting(agentID string) {
	if a := s.agentRegistry.Get(agentID); a != nil {
		s.agentRegistry.Unregister(a)
	}
}
