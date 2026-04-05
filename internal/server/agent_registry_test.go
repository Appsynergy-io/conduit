package server

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRegistry_UnregisterIdentityCheck verifies that Unregister only
// removes an entry when the provided ConnectedAgent is still the current
// one bound to that agent ID. Without this check, a reconnecting agent's
// stale cleanup would evict the fresh connection.
func TestAgentRegistry_UnregisterIdentityCheck(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ar := NewAgentRegistry(logger)

	agentID := "agent-1"

	// Seed the registry directly to simulate two sequential connections
	// from the same agent without needing a real Mux (Register would call
	// Mux.Close() on the previous entry when replacing it).
	conn1 := &ConnectedAgent{AgentID: agentID, cancel: func() {}}
	conn2 := &ConnectedAgent{AgentID: agentID, cancel: func() {}}
	ar.agents[agentID] = conn2
	require.Same(t, conn2, ar.Get(agentID))

	// Simulate conn1's stale cleanup running AFTER conn2 is registered.
	// Should NOT evict conn2.
	wasActive := ar.Unregister(conn1)
	assert.False(t, wasActive, "stale conn1 should not report active")
	require.Same(t, conn2, ar.Get(agentID), "conn2 must still be current after stale conn1 cleanup")

	// conn2's own cleanup should now succeed.
	wasActive = ar.Unregister(conn2)
	assert.True(t, wasActive, "conn2 should be the active one to remove")
	assert.Nil(t, ar.Get(agentID))
}

func TestAgentRegistry_UnregisterNil(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ar := NewAgentRegistry(logger)
	assert.False(t, ar.Unregister(nil))
}

func TestAgentRegistry_UnregisterUnknown(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ar := NewAgentRegistry(logger)
	stranger := &ConnectedAgent{AgentID: "never-registered", cancel: func() {}}
	assert.False(t, ar.Unregister(stranger))
}
