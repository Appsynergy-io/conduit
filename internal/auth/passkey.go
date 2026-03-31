package auth

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// sessionTTL is how long a WebAuthn challenge session is valid.
const sessionTTL = 5 * time.Minute

// WebAuthnUser wraps a Conduit user with WebAuthn credential data,
// implementing the webauthn.User interface required by the go-webauthn library.
type WebAuthnUser struct {
	ID          string
	Email       string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID returns the user's opaque identifier as bytes.
func (u *WebAuthnUser) WebAuthnID() []byte {
	return []byte(u.ID)
}

// WebAuthnName returns the user's email (human-readable identifier).
func (u *WebAuthnUser) WebAuthnName() string {
	return u.Email
}

// WebAuthnDisplayName returns the user's display name.
func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.DisplayName
}

// WebAuthnCredentials returns the user's registered credentials.
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// PasskeyToCredential converts a Conduit DB passkey record to a webauthn.Credential.
func PasskeyToCredential(credentialID, publicKey []byte, signCount uint32, authenticatorType string) webauthn.Credential {
	var attachment protocol.AuthenticatorAttachment
	switch authenticatorType {
	case "platform":
		attachment = protocol.Platform
	case "cross-platform":
		attachment = protocol.CrossPlatform
	}

	return webauthn.Credential{
		ID:              credentialID,
		PublicKey:       publicKey,
		AttestationType: "",
		Authenticator: webauthn.Authenticator{
			SignCount:  signCount,
			Attachment: attachment,
		},
	}
}

// AlgorithmName returns a human-readable name for a COSE algorithm identifier.
// See https://www.iana.org/assignments/cose/cose.xhtml#algorithms
func AlgorithmName(alg int64) string {
	switch alg {
	case -7:
		return "ECDSA-P256"
	case -8:
		return "Ed25519"
	case -35:
		return "ECDSA-P384"
	case -36:
		return "ECDSA-P521"
	case -257:
		return "RSA-PKCS1-v1_5"
	case -37:
		return "RSA-PSS"
	default:
		return "unknown"
	}
}

// AlgorithmWarning returns a warning if the algorithm is classical (not PQC).
// Per CLAUDE.md: WebAuthn allows ECDSA P-256, Ed25519, RSA because the hardware
// authenticator chooses the algorithm, but each use should be logged.
func AlgorithmWarning(alg int64) *string {
	// All current WebAuthn algorithms are classical; PQC authenticators don't exist yet.
	// Ed25519 is the closest to PQC-ready but is still classical.
	w := "classical algorithm used: hardware authenticator selected non-PQC algorithm"
	return &w
}

// WebAuthnSessionStore provides in-memory storage for WebAuthn challenge sessions.
// Sessions auto-expire after sessionTTL (5 minutes).
type WebAuthnSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

type sessionEntry struct {
	data      *webauthn.SessionData
	expiresAt time.Time
}

// NewWebAuthnSessionStore creates a new session store and starts the cleanup goroutine.
func NewWebAuthnSessionStore() *WebAuthnSessionStore {
	s := &WebAuthnSessionStore{
		sessions: make(map[string]*sessionEntry),
	}
	go s.cleanup()
	return s
}

// Store saves session data keyed by a string (typically userID).
func (s *WebAuthnSessionStore) Store(key string, data *webauthn.SessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[key] = &sessionEntry{
		data:      data,
		expiresAt: time.Now().Add(sessionTTL),
	}
}

// Get retrieves session data by key. Returns nil, false if not found or expired.
func (s *WebAuthnSessionStore) Get(key string) (*webauthn.SessionData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.sessions[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

// Delete removes session data by key.
func (s *WebAuthnSessionStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

// cleanup periodically removes expired sessions every 60 seconds.
func (s *WebAuthnSessionStore) cleanup() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for key, entry := range s.sessions {
			if now.After(entry.expiresAt) {
				delete(s.sessions, key)
			}
		}
		s.mu.Unlock()
	}
}
