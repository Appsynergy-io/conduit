package auth

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// WebAuthnUser interface
// ---------------------------------------------------------------------------

func TestWebAuthnUser_Interface(t *testing.T) {
	u := &WebAuthnUser{
		ID:          "user-123",
		Email:       "test@example.com",
		DisplayName: "Test User",
		Credentials: []webauthn.Credential{
			{ID: []byte("cred-1"), PublicKey: []byte("pk-1")},
		},
	}

	assert.Equal(t, []byte("user-123"), u.WebAuthnID())
	assert.Equal(t, "test@example.com", u.WebAuthnName())
	assert.Equal(t, "Test User", u.WebAuthnDisplayName())
	assert.Len(t, u.WebAuthnCredentials(), 1)
}

func TestWebAuthnUser_EmptyCredentials(t *testing.T) {
	u := &WebAuthnUser{ID: "u1", Email: "e@e.com", DisplayName: "E"}
	assert.Empty(t, u.WebAuthnCredentials())
}

// ---------------------------------------------------------------------------
// PasskeyToCredential
// ---------------------------------------------------------------------------

func TestPasskeyToCredential_Platform(t *testing.T) {
	cred := PasskeyToCredential([]byte("cid"), []byte("pk"), 5, "platform")
	assert.Equal(t, []byte("cid"), cred.ID)
	assert.Equal(t, []byte("pk"), cred.PublicKey)
	assert.Equal(t, uint32(5), cred.Authenticator.SignCount)
	assert.Equal(t, protocol.Platform, cred.Authenticator.Attachment)
}

func TestPasskeyToCredential_CrossPlatform(t *testing.T) {
	cred := PasskeyToCredential([]byte("cid"), []byte("pk"), 0, "cross-platform")
	assert.Equal(t, protocol.CrossPlatform, cred.Authenticator.Attachment)
}

func TestPasskeyToCredential_UnknownType(t *testing.T) {
	cred := PasskeyToCredential([]byte("cid"), []byte("pk"), 0, "unknown")
	assert.Equal(t, protocol.AuthenticatorAttachment(""), cred.Authenticator.Attachment)
}

// ---------------------------------------------------------------------------
// AlgorithmName
// ---------------------------------------------------------------------------

func TestAlgorithmName(t *testing.T) {
	tests := []struct {
		alg  int64
		name string
	}{
		{-7, "ECDSA-P256"},
		{-8, "Ed25519"},
		{-35, "ECDSA-P384"},
		{-36, "ECDSA-P521"},
		{-257, "RSA-PKCS1-v1_5"},
		{-37, "RSA-PSS"},
		{999, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, AlgorithmName(tt.alg))
		})
	}
}

// ---------------------------------------------------------------------------
// AlgorithmWarning
// ---------------------------------------------------------------------------

func TestAlgorithmWarning_AlwaysWarns(t *testing.T) {
	// All current WebAuthn algorithms are classical (no PQC authenticators yet)
	for _, alg := range []int64{-7, -8, -35, -257} {
		w := AlgorithmWarning(alg)
		require.NotNil(t, w, "algorithm %d should produce a warning", alg)
		assert.Contains(t, *w, "classical")
	}
}

// ---------------------------------------------------------------------------
// WebAuthnSessionStore
// ---------------------------------------------------------------------------

func TestSessionStore_StoreAndGet(t *testing.T) {
	store := NewWebAuthnSessionStore()
	data := &webauthn.SessionData{
		Challenge: "test-challenge",
	}

	store.Store("key1", data)
	got, ok := store.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "test-challenge", got.Challenge)
}

func TestSessionStore_GetNotFound(t *testing.T) {
	store := NewWebAuthnSessionStore()
	got, ok := store.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewWebAuthnSessionStore()
	store.Store("key1", &webauthn.SessionData{Challenge: "c"})
	store.Delete("key1")

	got, ok := store.Get("key1")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestSessionStore_Overwrite(t *testing.T) {
	store := NewWebAuthnSessionStore()
	store.Store("key1", &webauthn.SessionData{Challenge: "old"})
	store.Store("key1", &webauthn.SessionData{Challenge: "new"})

	got, ok := store.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "new", got.Challenge)
}

func TestSessionStore_ExpiredSession(t *testing.T) {
	store := NewWebAuthnSessionStore()
	// Directly inject an expired entry
	store.mu.Lock()
	store.sessions["expired"] = &sessionEntry{
		data:      &webauthn.SessionData{Challenge: "old"},
		expiresAt: time.Now().Add(-1 * time.Minute),
	}
	store.mu.Unlock()

	got, ok := store.Get("expired")
	assert.False(t, ok, "expired session should not be returned")
	assert.Nil(t, got)
}

func TestSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewWebAuthnSessionStore()
	done := make(chan bool, 20)

	for i := 0; i < 10; i++ {
		go func(n int) {
			key := "key"
			store.Store(key, &webauthn.SessionData{Challenge: "c"})
			store.Get(key)
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		go func(n int) {
			key := "key"
			store.Delete(key)
			store.Store(key, &webauthn.SessionData{Challenge: "c2"})
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
