package auth

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// recoveryCodeAlphabet is lowercase alphanumeric for recovery codes.
// Excludes visually ambiguous characters (0/o, 1/l/i) for readability.
const recoveryCodeAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// recoveryCodeLen is the number of alphabet characters per code (excluding
// the formatting dashes). 16 chars × log2(31) ≈ 77.8 bits of entropy, which
// comfortably exceeds NIST SP 800-63B look-up-secret guidance (20 bits min)
// and matches GitHub / AWS recovery-code strength.
const recoveryCodeLen = 16

// GenerateRecoveryCodes generates count recovery codes formatted as
// "xxxx-xxxx-xxxx-xxxx" (19 characters: 16 alphabet chars + 3 dashes).
// Uses crypto/rand with rejection sampling so every output is uniform over
// the alphabet (NIST SP 800-131A, OWASP A02).
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

// generateSingleCode produces one formatted recovery code. Characters are
// sampled uniformly from the alphabet using rejection sampling — bytes
// whose value would skew the modulo are discarded, eliminating the bias
// introduced by plain `byte % len(alphabet)`.
func generateSingleCode() (string, error) {
	alphaLen := byte(len(recoveryCodeAlphabet))
	// Largest multiple of alphaLen that fits in a byte. Any random byte
	// >= threshold is rejected to keep the distribution uniform.
	threshold := byte(256 - (256 % int(alphaLen)))

	chars := make([]byte, recoveryCodeLen)
	buf := make([]byte, 1)
	for i := 0; i < recoveryCodeLen; i++ {
		for {
			if _, err := rand.Read(buf); err != nil {
				return "", fmt.Errorf("reading random bytes: %w", err)
			}
			if buf[0] < threshold {
				chars[i] = recoveryCodeAlphabet[buf[0]%alphaLen]
				break
			}
		}
	}

	// Insert dashes every 4 characters: xxxx-xxxx-xxxx-xxxx
	var b strings.Builder
	b.Grow(recoveryCodeLen + (recoveryCodeLen/4 - 1))
	for i := 0; i < recoveryCodeLen; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(chars[i])
	}
	return b.String(), nil
}
