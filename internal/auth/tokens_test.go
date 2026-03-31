package auth_test

import (
	"encoding/base64"
	"testing"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSetupToken(t *testing.T) {
	token, err := auth.GenerateSetupToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(token)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
}

func TestGenerateSetupToken_Unique(t *testing.T) {
	t1, err := auth.GenerateSetupToken()
	require.NoError(t, err)

	t2, err := auth.GenerateSetupToken()
	require.NoError(t, err)

	assert.NotEqual(t, t1, t2)
}

func TestGenerateAgentKey(t *testing.T) {
	key, err := auth.GenerateAgentKey()
	require.NoError(t, err)
	assert.NotEmpty(t, key)

	raw, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(key)
	require.NoError(t, err)
	assert.Len(t, raw, 32)
}

func TestHashAgentKey_Deterministic(t *testing.T) {
	key, err := auth.GenerateAgentKey()
	require.NoError(t, err)

	h1 := auth.HashAgentKey(key)
	h2 := auth.HashAgentKey(key)
	assert.Equal(t, h1, h2)
}

func TestVerifyAgentKey_Valid(t *testing.T) {
	key, err := auth.GenerateAgentKey()
	require.NoError(t, err)

	hash := auth.HashAgentKey(key)
	assert.True(t, auth.VerifyAgentKey(key, hash))
}

func TestVerifyAgentKey_Invalid(t *testing.T) {
	key, err := auth.GenerateAgentKey()
	require.NoError(t, err)

	hash := auth.HashAgentKey(key)

	other, err := auth.GenerateAgentKey()
	require.NoError(t, err)

	assert.False(t, auth.VerifyAgentKey(other, hash))
}

func TestSignVerifyHMAC(t *testing.T) {
	key := []byte("test-hmac-key")
	data := []byte("hello world")

	sig := auth.SignHMAC(key, data)
	assert.Len(t, sig, 32) // HMAC-SHA256 produces 32 bytes

	assert.True(t, auth.VerifyHMAC(key, data, sig))
}

func TestVerifyHMAC_WrongSig(t *testing.T) {
	key := []byte("test-hmac-key")
	data := []byte("hello world")

	sig := auth.SignHMAC(key, data)

	wrongSig := make([]byte, len(sig))
	copy(wrongSig, sig)
	wrongSig[0] ^= 0xff

	assert.False(t, auth.VerifyHMAC(key, data, wrongSig))
}

func TestVerifyHMAC_WrongKey(t *testing.T) {
	key := []byte("test-hmac-key")
	data := []byte("hello world")

	sig := auth.SignHMAC(key, data)

	wrongKey := []byte("wrong-hmac-key")
	assert.False(t, auth.VerifyHMAC(wrongKey, data, sig))
}
