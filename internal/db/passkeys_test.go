package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedPasskey(t *testing.T, d *db.DB, userID, tenantID string) *db.Passkey {
	t.Helper()
	warning := "classical algorithm used: hardware authenticator selected non-PQC algorithm"
	p := &db.Passkey{
		ID:                uuid.NewString(),
		TenantID:          tenantID,
		UserID:            userID,
		CredentialID:      []byte("cred-" + uuid.NewString()[:8]),
		PublicKey:         []byte("pk-" + uuid.NewString()[:8]),
		Algorithm:         "ECDSA-P256",
		AlgorithmWarning:  &warning,
		AuthenticatorType: "platform",
		SignCount:         0,
	}
	err := d.CreatePasskey(context.Background(), p)
	require.NoError(t, err)
	return p
}

func TestCreatePasskey_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "pk@test.com", "org_member")

	p := seedPasskey(t, d, user.ID, tenantID)

	got, err := d.GetPasskeyByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, user.ID, got.UserID)
	assert.Equal(t, p.CredentialID, got.CredentialID)
	assert.Equal(t, p.PublicKey, got.PublicKey)
	assert.Equal(t, "ECDSA-P256", got.Algorithm)
	assert.NotNil(t, got.AlgorithmWarning)
	assert.Equal(t, "platform", got.AuthenticatorType)
	assert.Equal(t, uint32(0), got.SignCount)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestGetPasskeysByUserID_Empty(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "nopk@test.com", "org_member")

	passkeys, err := d.GetPasskeysByUserID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Empty(t, passkeys)
}

func TestGetPasskeysByUserID_Multiple(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "multi@test.com", "org_member")

	seedPasskey(t, d, user.ID, tenantID)
	seedPasskey(t, d, user.ID, tenantID)
	seedPasskey(t, d, user.ID, tenantID)

	passkeys, err := d.GetPasskeysByUserID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Len(t, passkeys, 3)
}

func TestGetPasskeysByUserID_IsolatedPerUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user1 := seedUser(t, d, tenantID, "u1@test.com", "org_member")
	user2 := seedUser(t, d, tenantID, "u2@test.com", "org_member")

	seedPasskey(t, d, user1.ID, tenantID)
	seedPasskey(t, d, user1.ID, tenantID)
	seedPasskey(t, d, user2.ID, tenantID)

	pk1, err := d.GetPasskeysByUserID(context.Background(), user1.ID)
	require.NoError(t, err)
	assert.Len(t, pk1, 2)

	pk2, err := d.GetPasskeysByUserID(context.Background(), user2.ID)
	require.NoError(t, err)
	assert.Len(t, pk2, 1)
}

func TestGetPasskeyByCredentialID(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "cred@test.com", "org_member")

	p := seedPasskey(t, d, user.ID, tenantID)

	got, err := d.GetPasskeyByCredentialID(context.Background(), p.CredentialID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, user.ID, got.UserID)
}

func TestGetPasskeyByCredentialID_NotFound(t *testing.T) {
	d := newTestDB(t)
	_, err := d.GetPasskeyByCredentialID(context.Background(), []byte("nonexistent"))
	assert.Error(t, err)
}

func TestUpdatePasskeySignCount(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "sc@test.com", "org_member")
	p := seedPasskey(t, d, user.ID, tenantID)

	err := d.UpdatePasskeySignCount(context.Background(), p.ID, 42)
	require.NoError(t, err)

	got, err := d.GetPasskeyByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, uint32(42), got.SignCount)
	assert.NotNil(t, got.LastUsedAt)
}

func TestUpdatePasskeySignCount_NotFound(t *testing.T) {
	d := newTestDB(t)
	err := d.UpdatePasskeySignCount(context.Background(), "nonexistent-id", 1)
	assert.Error(t, err)
}

func TestDeletePasskey(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "del@test.com", "org_member")
	p := seedPasskey(t, d, user.ID, tenantID)

	err := d.DeletePasskey(context.Background(), p.ID)
	require.NoError(t, err)

	_, err = d.GetPasskeyByID(context.Background(), p.ID)
	assert.Error(t, err, "passkey should be deleted")
}

func TestDeletePasskey_NotFound(t *testing.T) {
	d := newTestDB(t)
	err := d.DeletePasskey(context.Background(), "nonexistent-id")
	assert.Error(t, err)
}

func TestDeletePasskeysByUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "delall@test.com", "org_member")

	seedPasskey(t, d, user.ID, tenantID)
	seedPasskey(t, d, user.ID, tenantID)

	err := d.DeletePasskeysByUser(context.Background(), user.ID)
	require.NoError(t, err)

	passkeys, err := d.GetPasskeysByUserID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Empty(t, passkeys)
}
