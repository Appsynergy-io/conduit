package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims extends jwt.RegisteredClaims with Conduit-specific fields.
type Claims struct {
	jwt.RegisteredClaims
	TenantID    string   `json:"tid"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Services    []string `json:"services,omitempty"`
	SessionID   string   `json:"sid,omitempty"`
}

// JWTManager handles Ed25519 JWT issuance and validation.
type JWTManager struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewJWTManager creates a JWTManager with a fresh Ed25519 keypair.
func NewJWTManager(issuer string, accessTTL, refreshTTL time.Duration) (*JWTManager, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating Ed25519 keypair: %w", err)
	}
	return &JWTManager{
		privateKey: priv,
		publicKey:  pub,
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}, nil
}

// NewJWTManagerFromKey creates a JWTManager with an existing Ed25519 private key.
func NewJWTManagerFromKey(privateKey ed25519.PrivateKey, issuer string, accessTTL, refreshTTL time.Duration) *JWTManager {
	return &JWTManager{
		privateKey: privateKey,
		publicKey:  privateKey.Public().(ed25519.PublicKey),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// IssueAccessToken creates a short-lived access JWT.
func (m *JWTManager) IssueAccessToken(userID, tenantID, sessionID string, roles, services []string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
		TenantID:  tenantID,
		SessionID: sessionID,
		Roles:     roles,
		Services:  services,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing access token: %w", err)
	}
	return signed, nil
}

// IssueRefreshToken creates a longer-lived refresh JWT.
func (m *JWTManager) IssueRefreshToken(userID, tenantID, sessionID string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTTL)),
		},
		TenantID:  tenantID,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing refresh token: %w", err)
	}
	return signed, nil
}

// IssueScopedToken creates a short-lived JWT scoped to specific permissions
// (e.g., for recovery code flow that only allows passkey registration).
func (m *JWTManager) IssueScopedToken(userID, tenantID string, permissions []string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TenantID:    tenantID,
		Permissions: permissions,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing scoped token: %w", err)
	}
	return signed, nil
}

// ValidateToken parses and validates a JWT, returning the claims if valid.
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// PublicKey returns the Ed25519 public key (for external verification).
func (m *JWTManager) PublicKey() ed25519.PublicKey {
	return m.publicKey
}
