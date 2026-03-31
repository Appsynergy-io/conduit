package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/appsynergy-io/conduit/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJWTManager(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)
	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.PublicKey())
	assert.Len(t, mgr.PublicKey(), ed25519.PublicKeySize)
}

func TestIssueAccessToken_RoundTrip(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	userID := "user-001"
	tenantID := "tenant-001"
	sessionID := "session-001"
	roles := []string{"org_admin", "org_member"}
	services := []string{"remote-access"}

	tokenStr, err := mgr.IssueAccessToken(userID, tenantID, sessionID, roles, services)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := mgr.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, userID, claims.Subject)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, sessionID, claims.SessionID)
	assert.Equal(t, roles, claims.Roles)
	assert.Equal(t, services, claims.Services)
	assert.Equal(t, "conduit-test", claims.Issuer)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
}

func TestIssueRefreshToken_RoundTrip(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	userID := "user-002"
	tenantID := "tenant-002"
	sessionID := "session-002"

	tokenStr, err := mgr.IssueRefreshToken(userID, tenantID, sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := mgr.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, userID, claims.Subject)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, sessionID, claims.SessionID)
	assert.Empty(t, claims.Roles)
	assert.Empty(t, claims.Services)
}

func TestIssueScopedToken_RoundTrip(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	userID := "user-003"
	tenantID := "tenant-003"
	permissions := []string{"passkey:register"}

	tokenStr, err := mgr.IssueScopedToken(userID, tenantID, permissions, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := mgr.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, userID, claims.Subject)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, permissions, claims.Permissions)
	assert.Empty(t, claims.Roles)
	assert.Empty(t, claims.Services)
	assert.Empty(t, claims.SessionID)
}

func TestValidateToken_Expired(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 1*time.Millisecond, 1*time.Millisecond)
	require.NoError(t, err)

	tokenStr, err := mgr.IssueAccessToken("user-004", "tenant-004", "session-004", nil, nil)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond)

	claims, err := mgr.ValidateToken(tokenStr)
	assert.Error(t, err)
	assert.Nil(t, claims)
	assert.Contains(t, err.Error(), "token")
}

func TestValidateToken_WrongKey(t *testing.T) {
	mgr1, err := auth.NewJWTManager("conduit-issuer-1", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	mgr2, err := auth.NewJWTManager("conduit-issuer-2", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	tokenStr, err := mgr1.IssueAccessToken("user-005", "tenant-005", "session-005", []string{"org_member"}, []string{"remote-access"})
	require.NoError(t, err)

	claims, err := mgr2.ValidateToken(tokenStr)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestValidateToken_InvalidString(t *testing.T) {
	mgr, err := auth.NewJWTManager("conduit-test", 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	claims, err := mgr.ValidateToken("not-a-valid-jwt")
	assert.Error(t, err)
	assert.Nil(t, claims)

	claims, err = mgr.ValidateToken("")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestNewJWTManagerFromKey(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	mgr := auth.NewJWTManagerFromKey(priv, "conduit-from-key", 10*time.Minute, 12*time.Hour)
	require.NotNil(t, mgr)

	assert.Equal(t, pub, mgr.PublicKey())

	tokenStr, err := mgr.IssueAccessToken("user-006", "tenant-006", "session-006", []string{"org_owner"}, []string{"remote-access"})
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := mgr.ValidateToken(tokenStr)
	require.NoError(t, err)

	assert.Equal(t, "user-006", claims.Subject)
	assert.Equal(t, "tenant-006", claims.TenantID)
	assert.Equal(t, "conduit-from-key", claims.Issuer)
	assert.Equal(t, []string{"org_owner"}, claims.Roles)
	assert.Equal(t, []string{"remote-access"}, claims.Services)
}
