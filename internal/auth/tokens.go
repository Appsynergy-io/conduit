package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GenerateSetupToken creates a CSPRNG 32-byte setup token (NIST IA-12).
func GenerateSetupToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating setup token: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// GenerateAgentKey creates a CSPRNG 32-byte agent key for HMAC-SHA256 auth.
func GenerateAgentKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating agent key: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// HashAgentKey computes SHA-256 hash of the agent key for storage.
func HashAgentKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(h[:])
}

// VerifyAgentKey verifies an agent key against a stored hash.
func VerifyAgentKey(key, storedHash string) bool {
	return HashAgentKey(key) == storedHash
}

// SignHMAC produces an HMAC-SHA256 signature of the given data using the key.
func SignHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// VerifyHMAC checks an HMAC-SHA256 signature using constant-time comparison.
func VerifyHMAC(key, data, sig []byte) bool {
	expected := SignHMAC(key, data)
	return hmac.Equal(expected, sig)
}
