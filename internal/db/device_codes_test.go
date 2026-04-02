package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func TestDeviceCodeCRUD(t *testing.T) {
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	ctx := context.Background()
	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	// Create
	dc := &db.DeviceCode{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		DeviceCode:  "test-device-code-123",
		UserCode:    "ABCD-1234",
		ClientID:    "conduit-cli",
		ProfileName: "default",
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
	}
	require.NoError(t, database.CreateDeviceCode(ctx, dc))

	// Get by device code
	found, err := database.GetDeviceCodeByDeviceCode(ctx, "test-device-code-123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, dc.ID, found.ID)
	assert.Equal(t, "ABCD-1234", found.UserCode)
	assert.Equal(t, "conduit-cli", found.ClientID)
	assert.False(t, found.Authorized)
	assert.Nil(t, found.UserID)

	// Get by user code
	found2, err := database.GetDeviceCodeByUserCode(ctx, "ABCD-1234")
	require.NoError(t, err)
	require.NotNil(t, found2)
	assert.Equal(t, dc.ID, found2.ID)

	// Authorize
	userID := uuid.NewString()
	require.NoError(t, database.AuthorizeDeviceCode(ctx, dc.ID, userID))

	// Verify authorized
	authorized, err := database.GetDeviceCodeByDeviceCode(ctx, "test-device-code-123")
	require.NoError(t, err)
	require.NotNil(t, authorized)
	assert.True(t, authorized.Authorized)
	assert.Equal(t, userID, *authorized.UserID)

	// Delete
	require.NoError(t, database.DeleteDeviceCode(ctx, dc.ID))
	gone, err := database.GetDeviceCodeByDeviceCode(ctx, "test-device-code-123")
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func TestDeviceCodeNotFound(t *testing.T) {
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	ctx := context.Background()

	found, err := database.GetDeviceCodeByDeviceCode(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, found)

	found, err = database.GetDeviceCodeByUserCode(ctx, "NOPE-0000")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestDeleteExpiredDeviceCodes(t *testing.T) {
	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	ctx := context.Background()
	tenantID := uuid.NewString()
	require.NoError(t, database.CreateTenant(ctx, tenantID, "Test"))

	// Create an expired device code
	dc := &db.DeviceCode{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		DeviceCode: "expired-code",
		UserCode:   "EXPD-0001",
		ExpiresAt:  time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
	}
	require.NoError(t, database.CreateDeviceCode(ctx, dc))

	// Delete expired
	count, err := database.DeleteExpiredDeviceCodes(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify gone
	found, err := database.GetDeviceCodeByDeviceCode(ctx, "expired-code")
	require.NoError(t, err)
	assert.Nil(t, found)
}
