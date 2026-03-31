package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUser_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	u := &db.User{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Email:       "alice@example.com",
		FirstName:   "Alice",
		LastName:    "Smith",
		DisplayName: ptr("A. Smith"),
		Role:        "org_admin",
		Status:      "active",
	}
	err := d.CreateUser(ctx, u)
	require.NoError(t, err)

	got, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)

	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, "Alice", got.FirstName)
	assert.Equal(t, "Smith", got.LastName)
	assert.Equal(t, ptr("A. Smith"), got.DisplayName)
	assert.Equal(t, "org_admin", got.Role)
	assert.Equal(t, "active", got.Status)
	assert.Nil(t, got.PasswordHash)
	assert.Nil(t, got.LastLoginAt)
	assert.NotEmpty(t, got.CreatedAt)
	assert.NotEmpty(t, got.UpdatedAt)
}

func TestGetUserByEmail(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	seeded := seedUser(t, d, tenantID, "bob@example.com", "org_member")

	got, err := d.GetUserByEmail(ctx, "bob@example.com")
	require.NoError(t, err)

	assert.Equal(t, seeded.ID, got.ID)
	assert.Equal(t, "bob@example.com", got.Email)
	assert.Equal(t, "org_member", got.Role)
}

func TestListUsers_OrderedByEmail(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	seedUser(t, d, tenantID, "charlie@example.com", "org_member")
	seedUser(t, d, tenantID, "alice@example.com", "org_admin")
	seedUser(t, d, tenantID, "bob@example.com", "org_member")

	users, err := d.ListUsers(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, users, 3)

	assert.Equal(t, "alice@example.com", users[0].Email)
	assert.Equal(t, "bob@example.com", users[1].Email)
	assert.Equal(t, "charlie@example.com", users[2].Email)
}

func TestUpdateUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	u := seedUser(t, d, tenantID, "update@example.com", "org_member")

	// Verify original values before update.
	assert.Equal(t, "Test", u.FirstName)
	assert.Equal(t, "org_member", u.Role)

	u.FirstName = "Updated"
	u.LastName = "Name"
	u.DisplayName = ptr("U. Name")
	u.Role = "org_admin"

	err := d.UpdateUser(ctx, u)
	require.NoError(t, err)

	got, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)

	assert.Equal(t, "Updated", got.FirstName)
	assert.Equal(t, "Name", got.LastName)
	assert.Equal(t, ptr("U. Name"), got.DisplayName)
	assert.Equal(t, "org_admin", got.Role)
	assert.NotEmpty(t, got.UpdatedAt, "updated_at should be set")
}

func TestUpdateUserLastLogin(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	u := seedUser(t, d, tenantID, "login@example.com", "org_member")

	before, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, before.LastLoginAt)

	err = d.UpdateUserLastLogin(ctx, u.ID)
	require.NoError(t, err)

	after, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, after.LastLoginAt)
	assert.NotEmpty(t, *after.LastLoginAt)
}

func TestSetPasswordHash(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	u := seedUser(t, d, tenantID, "hash@example.com", "org_member")

	err := d.SetPasswordHash(ctx, u.ID, "$argon2id$fake-hash")
	require.NoError(t, err)

	got, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.PasswordHash)
	assert.Equal(t, "$argon2id$fake-hash", *got.PasswordHash)
}

func TestDeletePasswordHash(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	u := seedUser(t, d, tenantID, "delhash@example.com", "org_member")

	err := d.SetPasswordHash(ctx, u.ID, "$argon2id$to-be-deleted")
	require.NoError(t, err)

	set, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, set.PasswordHash)

	err = d.DeletePasswordHash(ctx, u.ID)
	require.NoError(t, err)

	got, err := d.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, got.PasswordHash)
}

func TestCountUsers(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	seedUser(t, d, tenantID, "one@example.com", "org_member")
	seedUser(t, d, tenantID, "two@example.com", "org_admin")

	count, err := d.CountUsers(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGetUserByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	_, err := d.GetUserByID(ctx, uuid.NewString())
	require.Error(t, err)
	assert.True(t, errors.Is(err, sql.ErrNoRows), "expected sql.ErrNoRows, got: %v", err)
}
