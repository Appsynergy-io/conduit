package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/auth"
)

func TestHashPassword(t *testing.T) {
	hash, err := auth.HashPassword("mysecurepassword")
	require.NoError(t, err)
	assert.Contains(t, hash, "$argon2id$")
	assert.Contains(t, hash, "m=65536,t=3,p=4")
}

func TestVerifyPassword_Correct(t *testing.T) {
	password := "test-password-123"
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	match, err := auth.VerifyPassword(password, hash)
	require.NoError(t, err)
	assert.True(t, match)
}

func TestVerifyPassword_Wrong(t *testing.T) {
	hash, err := auth.HashPassword("correct-password")
	require.NoError(t, err)

	match, err := auth.VerifyPassword("wrong-password", hash)
	require.NoError(t, err)
	assert.False(t, match)
}

func TestVerifyPassword_InvalidFormat(t *testing.T) {
	_, err := auth.VerifyPassword("password", "not-a-valid-hash")
	assert.Error(t, err)
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	hash1, err := auth.HashPassword("same-password")
	require.NoError(t, err)

	hash2, err := auth.HashPassword("same-password")
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "same password must produce different hashes (unique salts)")

	// Both must still verify
	match1, err := auth.VerifyPassword("same-password", hash1)
	require.NoError(t, err)
	assert.True(t, match1)

	match2, err := auth.VerifyPassword("same-password", hash2)
	require.NoError(t, err)
	assert.True(t, match2)
}

func TestVerifyPassword_Unicode(t *testing.T) {
	password := "пароль-密码-パスワード"
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	match, err := auth.VerifyPassword(password, hash)
	require.NoError(t, err)
	assert.True(t, match)
}

func TestVerifyPassword_LongPassword(t *testing.T) {
	// NIST 800-63B: support 64+ chars
	password := "a-very-long-password-that-exceeds-sixty-four-characters-because-nist-says-we-must-support-it"
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)

	match, err := auth.VerifyPassword(password, hash)
	require.NoError(t, err)
	assert.True(t, match)
}

func TestVerifyPassword_EmptyAgainstHash(t *testing.T) {
	hash, err := auth.HashPassword("realpassword")
	require.NoError(t, err)

	match, err := auth.VerifyPassword("", hash)
	require.NoError(t, err)
	assert.False(t, match)
}
