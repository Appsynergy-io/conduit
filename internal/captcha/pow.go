// Package captcha implements a self-hosted proof-of-work CAPTCHA.
//
// The server issues a challenge containing a random salt, difficulty target,
// expiry, and bound endpoint path, signed with HMAC-SHA256. The client must
// find a nonce such that SHA-256(salt || nonce) as a big-endian integer is
// less than the target. The server re-verifies the HMAC, expiry, endpoint
// binding, and hash condition, and tracks seen salts to prevent replay.
//
// Security properties:
//   - HMAC-SHA256 (NIST SP 800-175B, CLAUDE.md approved)
//   - 32-byte salt from crypto/rand (NIST SP 800-131A CSPRNG)
//   - Constant-time HMAC comparison (hmac.Equal — ASVS V6)
//   - Challenge bound to endpoint path (prevents cross-endpoint reuse)
//   - Single-use salts with expiry-based eviction (replay prevention)
//   - 60-second challenge expiry (limits attacker window)
//   - Zero external dependencies (Go stdlib only)
package captcha

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTTL is the challenge lifetime.
	DefaultTTL = 60 * time.Second
	// DefaultDifficulty is the number of leading zero bits required in the
	// hash. 18 bits ≈ 262k expected SHA-256 iterations ≈ 1-3 seconds in a
	// modern browser Web Worker using crypto.subtle.digest.
	DefaultDifficulty = 18
	// saltBytes is the length of the random salt.
	saltBytes = 32
	// replayCacheMax is the maximum number of recently-seen salts tracked
	// for replay prevention. Exceeding triggers emergency eviction.
	replayCacheMax = 10000
	// maxEndpointLen caps endpoint path size for HMAC input.
	maxEndpointLen = 100
)

// Challenge is an unsolved PoW challenge issued by the server.
type Challenge struct {
	Salt     string `json:"salt"`
	Target   string `json:"target"`
	Expires  int64  `json:"expires"`
	Endpoint string `json:"endpoint"`
	HMAC     string `json:"hmac"`
}

// Solution is a submitted challenge attempt from the client. Echoes all
// fields of the original Challenge plus the found Nonce.
type Solution struct {
	Salt     string `json:"salt"`
	Target   string `json:"target"`
	Expires  int64  `json:"expires"`
	Endpoint string `json:"endpoint"`
	HMAC     string `json:"hmac"`
	Nonce    uint64 `json:"nonce"`
}

// Verifier issues and validates PoW challenges.
type Verifier struct {
	key        []byte
	difficulty int
	ttl        time.Duration
	now        func() time.Time

	mu   sync.Mutex
	seen map[string]int64 // salt → unix expiry
}

// Errors returned by Verify. Callers SHOULD NOT expose specific errors to
// clients (OWASP A07: no enumeration — return identical failure responses).
var (
	ErrInvalidHMAC     = errors.New("captcha: hmac mismatch")
	ErrExpired         = errors.New("captcha: challenge expired")
	ErrReplayed        = errors.New("captcha: salt already used")
	ErrWrongEndpoint   = errors.New("captcha: endpoint mismatch")
	ErrBadFormat       = errors.New("captcha: malformed fields")
	ErrInsufficientPoW = errors.New("captcha: hash does not meet target")
)

// NewVerifier constructs a Verifier with the given HMAC key (minimum 16
// bytes, typically 32 bytes derived via HKDF from a persistent server
// secret). Panics if the key is too short — this is a programming error.
func NewVerifier(key []byte) *Verifier {
	if len(key) < 16 {
		panic("captcha: HMAC key must be at least 16 bytes")
	}
	return &Verifier{
		key:        append([]byte(nil), key...),
		difficulty: DefaultDifficulty,
		ttl:        DefaultTTL,
		now:        time.Now,
		seen:       make(map[string]int64),
	}
}

// SetDifficulty overrides the number of leading zero bits required.
// Safe to call only at initialization (not safe under concurrent Verify).
func (v *Verifier) SetDifficulty(bits int) {
	if bits < 1 || bits > 32 {
		panic("captcha: difficulty must be 1..32")
	}
	v.difficulty = bits
}

// Issue generates a fresh challenge bound to the given endpoint path.
// The endpoint MUST start with "/" and contain only URL-safe characters.
func (v *Verifier) Issue(endpoint string) (*Challenge, error) {
	if !validEndpoint(endpoint) {
		return nil, fmt.Errorf("captcha: invalid endpoint: %q", endpoint)
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("captcha: reading random salt: %w", err)
	}
	target := computeTarget(v.difficulty)
	expires := v.now().Add(v.ttl).Unix()
	c := &Challenge{
		Salt:     hex.EncodeToString(salt),
		Target:   hex.EncodeToString(target),
		Expires:  expires,
		Endpoint: endpoint,
	}
	c.HMAC = hex.EncodeToString(v.sign(c.Salt, c.Target, c.Expires, c.Endpoint))
	return c, nil
}

// Verify validates a submitted Solution for the given endpoint.
//
// Checks performed, in order:
//  1. Field format validity (hex lengths, endpoint shape)
//  2. HMAC authenticity (constant-time)
//  3. Endpoint binding matches
//  4. Challenge not expired
//  5. Salt not replayed
//  6. SHA-256(salt || nonce) < target
//
// On success, the salt is recorded in the replay cache. On ANY failure
// the salt is NOT recorded — otherwise a forged submission could lock out
// the real solver.
func (v *Verifier) Verify(sol *Solution, endpoint string) error {
	if sol == nil {
		return ErrBadFormat
	}
	if err := validateFormat(sol); err != nil {
		return err
	}
	expected := v.sign(sol.Salt, sol.Target, sol.Expires, sol.Endpoint)
	submitted, err := hex.DecodeString(sol.HMAC)
	if err != nil {
		return ErrBadFormat
	}
	if !hmac.Equal(submitted, expected) {
		return ErrInvalidHMAC
	}
	if sol.Endpoint != endpoint {
		return ErrWrongEndpoint
	}
	now := v.now().Unix()
	if sol.Expires < now {
		return ErrExpired
	}
	if v.isSeen(sol.Salt, now) {
		return ErrReplayed
	}
	salt, _ := hex.DecodeString(sol.Salt)
	target, _ := hex.DecodeString(sol.Target)
	if !hashMeetsTarget(salt, sol.Nonce, target) {
		return ErrInsufficientPoW
	}
	v.markSeen(sol.Salt, sol.Expires)
	return nil
}

// sign returns raw HMAC-SHA256 bytes over (salt || target || expires || endpoint).
func (v *Verifier) sign(salt, target string, expires int64, endpoint string) []byte {
	h := hmac.New(sha256.New, v.key)
	h.Write([]byte(salt))
	h.Write([]byte(target))
	var expBytes [8]byte
	binary.BigEndian.PutUint64(expBytes[:], uint64(expires))
	h.Write(expBytes[:])
	h.Write([]byte(endpoint))
	return h.Sum(nil)
}

// isSeen reports whether salt is in the replay cache (and not expired).
// Opportunistically prunes expired entries while holding the lock.
func (v *Verifier) isSeen(salt string, now int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.pruneLocked(now)
	_, ok := v.seen[salt]
	return ok
}

// markSeen records a salt in the replay cache, evicting if over capacity.
func (v *Verifier) markSeen(salt string, expires int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.seen) >= replayCacheMax {
		// Emergency eviction: drop oldest half. Cheap and rare under normal load.
		oldest := expires
		for _, e := range v.seen {
			if e < oldest {
				oldest = e
			}
		}
		cutoff := (oldest + expires) / 2
		for s, e := range v.seen {
			if e < cutoff {
				delete(v.seen, s)
			}
		}
	}
	v.seen[salt] = expires
}

// pruneLocked removes expired entries. MUST be called with mu held.
func (v *Verifier) pruneLocked(now int64) {
	for salt, exp := range v.seen {
		if exp < now {
			delete(v.seen, salt)
		}
	}
}

// validateFormat checks that the solution's string fields are well-formed.
func validateFormat(s *Solution) error {
	if len(s.Salt) != 64 || len(s.Target) != 64 || len(s.HMAC) != 64 {
		return ErrBadFormat
	}
	if _, err := hex.DecodeString(s.Salt); err != nil {
		return ErrBadFormat
	}
	if _, err := hex.DecodeString(s.Target); err != nil {
		return ErrBadFormat
	}
	if _, err := hex.DecodeString(s.HMAC); err != nil {
		return ErrBadFormat
	}
	if !validEndpoint(s.Endpoint) {
		return ErrBadFormat
	}
	return nil
}

// validEndpoint restricts endpoint paths to "/" + URL-safe chars.
func validEndpoint(e string) bool {
	if e == "" || len(e) > maxEndpointLen || e[0] != '/' {
		return false
	}
	for i := 0; i < len(e); i++ {
		c := e[i]
		ok := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '/' || c == '_' || c == '-'
		if !ok {
			return false
		}
	}
	return !strings.Contains(e, "//")
}

// computeTarget returns a 32-byte big-endian target where the top
// `difficulty` bits are zero and all remaining bits are 1. Any SHA-256
// output less than this has at least `difficulty` leading zero bits.
func computeTarget(difficulty int) []byte {
	target := make([]byte, 32)
	for i := range target {
		target[i] = 0xFF
	}
	zeroBytes := difficulty / 8
	for i := 0; i < zeroBytes; i++ {
		target[i] = 0
	}
	if bit := difficulty % 8; bit > 0 {
		target[zeroBytes] = 0xFF >> bit
	}
	return target
}

// hashMeetsTarget reports whether SHA-256(salt || nonce_BE_8B) < target.
// Both sum and target are 32-byte big-endian integers.
func hashMeetsTarget(salt []byte, nonce uint64, target []byte) bool {
	h := sha256.New()
	h.Write(salt)
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], nonce)
	h.Write(nb[:])
	sum := h.Sum(nil)
	for i := 0; i < 32; i++ {
		if sum[i] < target[i] {
			return true
		}
		if sum[i] > target[i] {
			return false
		}
	}
	return false
}
