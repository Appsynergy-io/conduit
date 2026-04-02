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
