package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateJoinToken_SingleUse(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "single_use",
		Name:      "one-time-token",
		Labels:    ptr(`{"env":"staging"}`),
		TokenHash: "sha256-single-use-hash",
		UsedCount: 0,
		MaxUses:   intPtr(1),
		ExpiresAt: ptr(time.Now().Add(time.Hour).UTC().Format(time.RFC3339)),
		Revoked:   0,
		CreatedBy: &user.ID,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, "single_use", got.Type)
	assert.Equal(t, "one-time-token", got.Name)
	assert.Equal(t, `{"env":"staging"}`, *got.Labels)
	assert.Equal(t, "sha256-single-use-hash", got.TokenHash)
	assert.Equal(t, 0, got.UsedCount)
	require.NotNil(t, got.MaxUses)
	assert.Equal(t, 1, *got.MaxUses)
	require.NotNil(t, got.ExpiresAt)
	assert.Equal(t, 0, got.Revoked)
	assert.Nil(t, got.RevokedAt)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, user.ID, *got.CreatedBy)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestCreateJoinToken_Persistent(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "persistent",
		Name:      "fleet-enrollment",
		Labels:    ptr(`{"env":"production","role":"web"}`),
		TokenHash: "sha256-persistent-hash",
		UsedCount: 0,
		MaxUses:   intPtr(100),
		ExpiresAt: nil,
		Revoked:   0,
		CreatedBy: &user.ID,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "persistent", got.Type)
	assert.Equal(t, "fleet-enrollment", got.Name)
	require.NotNil(t, got.MaxUses)
	assert.Equal(t, 100, *got.MaxUses)
	assert.Nil(t, got.ExpiresAt)
}

func TestGetJoinTokenByHash(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)

	hash := "sha256-lookup-by-hash-value"

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "single_use",
		Name:      "hash-lookup-token",
		TokenHash: hash,
		UsedCount: 0,
		Revoked:   0,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByHash(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, hash, got.TokenHash)

	// Non-existent hash returns nil, nil.
	got, err = d.GetJoinTokenByHash(ctx, "no-such-hash")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListJoinTokens_OrderedDesc(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)

	// Use explicit timestamps so ordering is deterministic regardless of clock granularity.
	timestamps := []string{
		"2025-01-01T00:00:00Z",
		"2025-01-02T00:00:00Z",
		"2025-01-03T00:00:00Z",
	}

	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		ids[i] = uuid.NewString()
		token := &db.JoinToken{
			ID:        ids[i],
			TenantID:  tenantID,
			Type:      "persistent",
			Name:      "token-" + ids[i][:8],
			TokenHash: "hash-" + ids[i],
			UsedCount: 0,
			Revoked:   0,
		}
		err := d.CreateJoinToken(ctx, token)
		require.NoError(t, err)
		// Override created_at to a known value so ordering is reliable.
		_, err = d.Conn().ExecContext(ctx,
			"UPDATE join_tokens SET created_at = ? WHERE id = ?", timestamps[i], ids[i])
		require.NoError(t, err)
	}

	tokens, err := d.ListJoinTokens(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, tokens, 3)

	// Newest first (DESC): 2025-01-03, 2025-01-02, 2025-01-01.
	assert.Equal(t, ids[2], tokens[0].ID)
	assert.Equal(t, ids[1], tokens[1].ID)
	assert.Equal(t, ids[0], tokens[2].ID)
}

func TestIncrementTokenUsage(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "persistent",
		Name:      "increment-test",
		TokenHash: "sha256-increment-hash",
		UsedCount: 0,
		Revoked:   0,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	err = d.IncrementTokenUsage(ctx, token.ID)
	require.NoError(t, err)

	err = d.IncrementTokenUsage(ctx, token.ID)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2, got.UsedCount)
}

func TestRevokeJoinToken(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "persistent",
		Name:      "revoke-test",
		TokenHash: "sha256-revoke-hash",
		UsedCount: 0,
		Revoked:   0,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	err = d.RevokeJoinToken(ctx, token.ID)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Revoked)
	require.NotNil(t, got.RevokedAt)
	assert.NotEmpty(t, *got.RevokedAt)
}

func TestDeleteJoinToken(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)

	token := &db.JoinToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Type:      "single_use",
		Name:      "delete-test",
		TokenHash: "sha256-delete-hash",
		UsedCount: 0,
		Revoked:   0,
	}

	err := d.CreateJoinToken(ctx, token)
	require.NoError(t, err)

	err = d.DeleteJoinToken(ctx, token.ID)
	require.NoError(t, err)

	got, err := d.GetJoinTokenByID(ctx, token.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetJoinTokenByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	got, err := d.GetJoinTokenByID(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}
