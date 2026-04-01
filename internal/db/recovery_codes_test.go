package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedRecoveryCodes(t *testing.T, d *db.DB, userID, tenantID string, count int) []db.RecoveryCode {
	t.Helper()
	codes := make([]db.RecoveryCode, count)
	now := db.Now()
	for i := 0; i < count; i++ {
		codes[i] = db.RecoveryCode{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			UserID:    userID,
			CodeHash:  "$argon2id$fake-hash-" + uuid.NewString()[:8],
			CreatedAt: now,
		}
	}
	err := d.CreateRecoveryCodes(context.Background(), userID, codes)
	require.NoError(t, err)
	return codes
}

func TestCreateRecoveryCodes_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "rc@test.com", "org_member")

	codes := seedRecoveryCodes(t, d, user.ID, tenantID, 10)
	assert.Len(t, codes, 10)

	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Len(t, unused, 10)
}

func TestCreateRecoveryCodes_ReplacesExisting(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "replace@test.com", "org_member")

	// Create first batch
	seedRecoveryCodes(t, d, user.ID, tenantID, 5)
	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Len(t, unused, 5)

	// Create second batch — replaces first
	seedRecoveryCodes(t, d, user.ID, tenantID, 10)
	unused, err = d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Len(t, unused, 10, "new codes should replace old ones")
}

func TestGetUnusedRecoveryCodesByUser_Empty(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "empty@test.com", "org_member")

	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Empty(t, unused)
}

func TestGetUnusedRecoveryCodesByUser_ExcludesUsed(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "used@test.com", "org_member")

	codes := seedRecoveryCodes(t, d, user.ID, tenantID, 3)

	// Mark one as used
	err := d.MarkRecoveryCodeUsed(context.Background(), codes[0].ID)
	require.NoError(t, err)

	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Len(t, unused, 2)
}

func TestMarkRecoveryCodeUsed(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "mark@test.com", "org_member")

	codes := seedRecoveryCodes(t, d, user.ID, tenantID, 1)

	err := d.MarkRecoveryCodeUsed(context.Background(), codes[0].ID)
	require.NoError(t, err)

	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Empty(t, unused)
}

func TestMarkRecoveryCodeUsed_NotFound(t *testing.T) {
	d := newTestDB(t)
	err := d.MarkRecoveryCodeUsed(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestDeleteRecoveryCodesByUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "del@test.com", "org_member")

	seedRecoveryCodes(t, d, user.ID, tenantID, 5)

	err := d.DeleteRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)

	unused, err := d.GetUnusedRecoveryCodesByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Empty(t, unused)
}

func TestCountUnusedRecoveryCodes(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "count@test.com", "org_member")

	seedRecoveryCodes(t, d, user.ID, tenantID, 10)

	count, err := d.CountUnusedRecoveryCodes(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 10, count)
}

func TestCountUnusedRecoveryCodes_AfterUsing(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "count2@test.com", "org_member")

	codes := seedRecoveryCodes(t, d, user.ID, tenantID, 10)

	// Use 3 codes
	for i := 0; i < 3; i++ {
		err := d.MarkRecoveryCodeUsed(context.Background(), codes[i].ID)
		require.NoError(t, err)
	}

	count, err := d.CountUnusedRecoveryCodes(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, count)
}

func TestCountUnusedRecoveryCodes_NoUser(t *testing.T) {
	d := newTestDB(t)
	count, err := d.CountUnusedRecoveryCodes(context.Background(), "nonexistent-user")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestRecoveryCodes_UserIsolation(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user1 := seedUser(t, d, tenantID, "u1@test.com", "org_member")
	user2 := seedUser(t, d, tenantID, "u2@test.com", "org_member")

	seedRecoveryCodes(t, d, user1.ID, tenantID, 10)
	seedRecoveryCodes(t, d, user2.ID, tenantID, 5)

	count1, err := d.CountUnusedRecoveryCodes(context.Background(), user1.ID)
	require.NoError(t, err)
	assert.Equal(t, 10, count1)

	count2, err := d.CountUnusedRecoveryCodes(context.Background(), user2.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, count2)

	// Deleting user1's codes doesn't affect user2
	err = d.DeleteRecoveryCodesByUser(context.Background(), user1.ID)
	require.NoError(t, err)

	count2, err = d.CountUnusedRecoveryCodes(context.Background(), user2.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, count2)
}
