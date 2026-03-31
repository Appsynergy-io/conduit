package auth

import (
	"crypto/rand"
	"fmt"
)

// recoveryCodeAlphabet is lowercase alphanumeric for recovery codes.
// Excludes visually ambiguous characters (0/o, 1/l) for readability.
const recoveryCodeAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// GenerateRecoveryCodes generates count recovery codes formatted as "xxxx-xxxx".
// Uses crypto/rand for all randomness (NIST SP 800-131A, OWASP A02).
func GenerateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code, err := generateSingleCode()
		if err != nil {
			return nil, fmt.Errorf("generating recovery code %d: %w", i, err)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// generateSingleCode produces one "xxxx-xxxx" code using crypto/rand.
func generateSingleCode() (string, error) {
	// 8 random characters from the alphabet, formatted as xxxx-xxxx
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}

	chars := make([]byte, 8)
	for i, b := range buf {
		chars[i] = recoveryCodeAlphabet[int(b)%len(recoveryCodeAlphabet)]
	}

	return string(chars[:4]) + "-" + string(chars[4:]), nil
}
