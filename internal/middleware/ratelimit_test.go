package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Allow_UnderLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     10,
		Interval: 1 * time.Second,
		Burst:    10,
	})

	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow("192.168.1.1"), "request %d should be allowed", i+1)
	}
}

func TestRateLimiter_Allow_OverLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     5,
		Interval: 1 * time.Hour, // slow refill
		Burst:    5,
	})

	// Exhaust all tokens
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("10.0.0.1"), "request %d should be allowed", i+1)
	}

	// Next request should be rejected
	assert.False(t, rl.Allow("10.0.0.1"), "request 6 should be rate limited")
	assert.False(t, rl.Allow("10.0.0.1"), "request 7 should be rate limited")
}

func TestRateLimiter_Allow_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     2,
		Interval: 1 * time.Hour,
		Burst:    2,
	})

	// Exhaust key A
	assert.True(t, rl.Allow("A"))
	assert.True(t, rl.Allow("A"))
	assert.False(t, rl.Allow("A"), "key A should be limited")

	// Key B is independent
	assert.True(t, rl.Allow("B"), "key B should not be affected by key A")
	assert.True(t, rl.Allow("B"))
	assert.False(t, rl.Allow("B"), "key B should now be limited")
}

func TestRateLimiter_Allow_Refill(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     100,
		Interval: 1 * time.Second,
		Burst:    2,
	})

	// Exhaust tokens
	assert.True(t, rl.Allow("refill"))
	assert.True(t, rl.Allow("refill"))
	assert.False(t, rl.Allow("refill"))

	// Simulate time passing by directly adjusting the bucket
	rl.mu.Lock()
	rl.buckets["refill"].lastSeen = time.Now().Add(-50 * time.Millisecond)
	rl.mu.Unlock()

	// Should have refilled enough for at least one request
	// 100 tokens/sec * 0.05s = 5 tokens refilled
	assert.True(t, rl.Allow("refill"), "should be allowed after refill")
}

func TestRateLimiter_BurstDefaults(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     50,
		Interval: 1 * time.Hour,
	})

	assert.Equal(t, 50, rl.burst, "burst should default to rate when not specified")
}

func TestRateLimiter_BurstExplicit(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     100,
		Interval: 1 * time.Hour,
		Burst:    20,
	})

	assert.Equal(t, 20, rl.burst, "burst should use configured value")

	// Can burst up to 20, not 100
	for i := 0; i < 20; i++ {
		assert.True(t, rl.Allow("burst"), "request %d should be allowed", i+1)
	}
	assert.False(t, rl.Allow("burst"), "request 21 should be rate limited (burst exceeded)")
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     1,
		Interval: 1 * time.Hour,
		Burst:    1,
	})

	handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: allowed
	req1 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req1.RemoteAddr = "1.2.3.4:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second request: rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req2.RemoteAddr = "1.2.3.4:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	assert.Contains(t, w2.Header().Get("Content-Type"), "application/problem+json")
}

func TestRateLimitMiddleware_DifferentIPsNotAffected(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     1,
		Interval: 1 * time.Hour,
		Burst:    1,
	})

	handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust IP A
	reqA := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	reqA.RemoteAddr = "1.2.3.4:12345"
	wA := httptest.NewRecorder()
	handler.ServeHTTP(wA, reqA)
	assert.Equal(t, http.StatusOK, wA.Code)

	// IP A blocked
	reqA2 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	reqA2.RemoteAddr = "1.2.3.4:54321"
	wA2 := httptest.NewRecorder()
	handler.ServeHTTP(wA2, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, wA2.Code)

	// IP B still allowed
	reqB := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	reqB.RemoteAddr = "5.6.7.8:12345"
	wB := httptest.NewRecorder()
	handler.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusOK, wB.Code)
}

func TestClientIP_RemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.100:54321"

	assert.Equal(t, "192.168.1.100", clientIP(r))
}

func TestClientIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18, 150.172.238.178")

	assert.Equal(t, "203.0.113.50", clientIP(r), "should use first IP from X-Forwarded-For")
}

func TestClientIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Real-IP", "198.51.100.23")

	assert.Equal(t, "198.51.100.23", clientIP(r))
}

func TestClientIP_XForwardedForPriority(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")
	r.Header.Set("X-Real-IP", "198.51.100.23")

	assert.Equal(t, "203.0.113.50", clientIP(r), "X-Forwarded-For should take priority over X-Real-IP")
}

func TestClientIP_RemoteAddrNoPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.100"

	assert.Equal(t, "192.168.1.100", clientIP(r))
}

func TestRateLimitMiddleware_XForwardedFor(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     1,
		Interval: 1 * time.Hour,
		Burst:    1,
	})

	handler := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from proxied IP
	req1 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	req1.Header.Set("X-Forwarded-For", "203.0.113.50")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second request from same proxied IP — rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req2.RemoteAddr = "10.0.0.2:5678"
	req2.Header.Set("X-Forwarded-For", "203.0.113.50")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		Rate:     1000,
		Interval: 1 * time.Second,
		Burst:    1000,
	})

	// Hammer concurrently to check for race conditions
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				rl.Allow("concurrent-key")
			}
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify the limiter is still functional after concurrent hammering
	require.NotNil(t, rl.buckets["concurrent-key"])
}
