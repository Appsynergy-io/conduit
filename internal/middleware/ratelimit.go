package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/appsynergy-io/conduit/internal/apierror"
)

// RateLimiter provides per-key token bucket rate limiting.
// Keys are typically client IP addresses. Thread-safe.
//
// NIST SP 800-53: SC-5 (DoS protection), SI-10 (input validation).
// OWASP: API4 (unrestricted resource consumption), API6 (unrestricted access to sensitive business flows).
// OWASP A07: Anti-brute-force on auth endpoints.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens added per interval
	interval time.Duration // refill interval
	burst    int           // max tokens (bucket capacity)
	cleanup  time.Duration // remove idle buckets after this duration
}

// bucket tracks tokens for a single key.
type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimitConfig configures a rate limiter instance.
type RateLimitConfig struct {
	// Rate is the number of requests allowed per interval.
	Rate int
	// Interval is the time window for the rate (e.g., 1*time.Hour).
	Interval time.Duration
	// Burst is the maximum burst size (bucket capacity).
	// If zero, defaults to Rate.
	Burst int
}

// NewRateLimiter creates a rate limiter with the given configuration.
// Starts a background goroutine for bucket cleanup.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	burst := cfg.Burst
	if burst == 0 {
		burst = cfg.Rate
	}

	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     cfg.Rate,
		interval: cfg.Interval,
		burst:    burst,
		cleanup:  5 * time.Minute,
	}

	go rl.cleanupLoop()
	return rl
}

// Allow checks whether a request from the given key is allowed.
// Returns true if the request is within the rate limit.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:   float64(rl.burst) - 1, // consume one token
			lastSeen: now,
		}
		rl.buckets[key] = b
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastSeen)
	tokensPerSec := float64(rl.rate) / rl.interval.Seconds()
	b.tokens += elapsed.Seconds() * tokensPerSec
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}

// cleanupLoop removes stale buckets periodically to prevent memory leaks.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-2 * rl.interval)
		for key, b := range rl.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit creates middleware that applies per-IP rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded (RFC 9457).
//
// NIST SP 800-228: REC-API-15 (rate limiting per user/service).
// OWASP API4: Unrestricted resource consumption.
func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIP(r)

			if !limiter.Allow(key) {
				apierror.Write(w, r, http.StatusTooManyRequests, "Too Many Requests",
					"Rate limit exceeded. Try again later.", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client IP from the request.
// Checks X-Forwarded-For and X-Real-IP before falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	// X-Forwarded-For: first IP in chain is the original client
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP only (client IP before any proxies)
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Strip port from RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
