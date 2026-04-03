package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/appsynergy-io/conduit/internal/auth"
)

// ValidateCIToken implements middleware.CITokenValidator.
// It hashes the plaintext token, looks it up in the DB, checks expiry,
// updates last_used_at, and returns synthetic JWT claims (NIST IA-5, OWASP API2).
func (s *Server) ValidateCIToken(ctx context.Context, token string) (*auth.Claims, error) {
	// Hash the token to look up in DB
	h := sha256.Sum256([]byte(token))
	tokenHash := base64.RawURLEncoding.EncodeToString(h[:])

	ciToken, err := s.db.GetCITokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("looking up ci token: %w", err)
	}
	if ciToken == nil {
		return nil, fmt.Errorf("ci token not found")
	}

	// Check expiry (NIST IA-5)
	if ciToken.ExpiresAt != nil {
		exp, err := time.Parse(time.RFC3339, *ciToken.ExpiresAt)
		if err == nil && time.Now().UTC().After(exp) {
			return nil, fmt.Errorf("ci token expired")
		}
	}

	// Update last_used_at (fire-and-forget — don't block the request)
	go func() {
		_ = s.db.UpdateCITokenLastUsed(context.Background(), ciToken.ID)
	}()

	// Look up the creating user to get their role for claims
	user, err := s.db.GetUserByID(ctx, ciToken.CreatedBy)
	if err != nil || user == nil {
		return nil, fmt.Errorf("ci token owner not found")
	}

	// Parse scopes from JSON array into Permissions field
	var scopes []string
	if err := json.Unmarshal([]byte(ciToken.Scopes), &scopes); err != nil {
		scopes = nil
	}

	// Build synthetic claims scoped by the CI token's permissions
	claims := &auth.Claims{
		TenantID:    ciToken.TenantID,
		Roles:       []string{user.Role},
		Permissions: scopes,
		Services:    []string{"remote-access"},
	}
	claims.Subject = ciToken.CreatedBy
	claims.Issuer = "conduit-ci-token"

	return claims, nil
}

// hasPermission checks if claims include a specific permission scope.
func hasPermission(claims *auth.Claims, scope string) bool {
	for _, p := range claims.Permissions {
		if p == scope {
			return true
		}
	}
	return false
}
