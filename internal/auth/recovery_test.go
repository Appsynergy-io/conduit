package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRecoveryCodes_Count(t *testing.T) {
	codes, err := GenerateRecoveryCodes(10)
	require.NoError(t, err)
	assert.Len(t, codes, 10)
}

func TestGenerateRecoveryCodes_Format(t *testing.T) {
	codes, err := GenerateRecoveryCodes(5)
	require.NoError(t, err)

	for _, code := range codes {
		// Format: xxxx-xxxx-xxxx-xxxx (16 alphabet chars + 3 dashes = 19)
		assert.Len(t, code, 19, "code should be 19 characters: %s", code)
		assert.Equal(t, "-", string(code[4]), "dash expected at position 4: %s", code)
		assert.Equal(t, "-", string(code[9]), "dash expected at position 9: %s", code)
		assert.Equal(t, "-", string(code[14]), "dash expected at position 14: %s", code)

		// Every non-dash character must be in the alphabet.
		for i, c := range code {
			if i == 4 || i == 9 || i == 14 {
				continue
			}
			assert.True(t, strings.ContainsRune(recoveryCodeAlphabet, c),
				"character %c not in alphabet: %s", c, code)
		}
	}
}

func TestGenerateRecoveryCodes_Uniqueness(t *testing.T) {
	codes, err := GenerateRecoveryCodes(100)
	require.NoError(t, err)

	seen := make(map[string]bool)
	for _, code := range codes {
		assert.False(t, seen[code], "duplicate code generated: %s", code)
		seen[code] = true
	}
}

func TestGenerateRecoveryCodes_Zero(t *testing.T) {
	codes, err := GenerateRecoveryCodes(0)
	require.NoError(t, err)
	assert.Empty(t, codes)
}

func TestGenerateRecoveryCodes_UsesCSPRNG(t *testing.T) {
	// Generate two batches and verify they differ (crypto/rand)
	batch1, err := GenerateRecoveryCodes(10)
	require.NoError(t, err)
	batch2, err := GenerateRecoveryCodes(10)
	require.NoError(t, err)

	// Extremely unlikely that all 10 match if using CSPRNG
	allMatch := true
	for i := range batch1 {
		if batch1[i] != batch2[i] {
			allMatch = false
			break
		}
	}
	assert.False(t, allMatch, "two batches of recovery codes should not be identical")
}

func TestGenerateRecoveryCodes_AlphabetExcludesAmbiguous(t *testing.T) {
	// Verify the alphabet excludes visually ambiguous characters
	assert.NotContains(t, recoveryCodeAlphabet, "0")
	assert.NotContains(t, recoveryCodeAlphabet, "o")
	assert.NotContains(t, recoveryCodeAlphabet, "1")
	assert.NotContains(t, recoveryCodeAlphabet, "l")
	assert.NotContains(t, recoveryCodeAlphabet, "i")
}
