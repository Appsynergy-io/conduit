package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAddJitter(t *testing.T) {
	base := 10 * time.Second

	// Run multiple times to test randomness
	for i := 0; i < 100; i++ {
		result := addJitter(base, 0.3)
		// Should be within +/- 30% of base
		minExpected := time.Duration(float64(base) * 0.7)
		maxExpected := time.Duration(float64(base) * 1.3)
		assert.GreaterOrEqual(t, result, minExpected, "jitter too low: %v", result)
		assert.LessOrEqual(t, result, maxExpected, "jitter too high: %v", result)
	}
}

func TestAddJitter_ZeroFraction(t *testing.T) {
	base := 5 * time.Second
	result := addJitter(base, 0)
	assert.Equal(t, base, result)
}

func TestAddJitter_ZeroDuration(t *testing.T) {
	result := addJitter(0, 0.3)
	// With zero duration, jitter range is zero, should return base
	assert.Equal(t, time.Duration(0), result)
}

func TestHostname(t *testing.T) {
	h := Hostname()
	assert.NotEmpty(t, h)
}

func TestNew(t *testing.T) {
	cfg := &Config{
		ServerURL: "https://conduit.example.com",
		AgentID:   "test-agent",
		AgentKey:  "deadbeef",
		TenantID:  "test-tenant",
	}

	a := New(cfg, nil)
	assert.NotNil(t, a)
	assert.Equal(t, cfg, a.cfg)
	assert.NotNil(t, a.shells)
}
