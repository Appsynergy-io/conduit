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

func seedCIToken(t *testing.T, d *db.DB, tenantID, userID, name, hash string) *db.CIToken {
	t.Helper()
	token := &db.CIToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		CreatedBy: userID,
		Name:      name,
		TokenHash: hash,
		Scopes:    `["agents:read","shell:execute"]`,
	}
	err := d.CreateCIToken(context.Background(), token)
	require.NoError(t, err)
	return token
}

func TestCreateCIToken(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := &db.CIToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		CreatedBy: user.ID,
		Name:      "GitHub Actions deploy",
		TokenHash: "sha256-ci-hash-1",
		Scopes:    `["agents:read","shell:execute"]`,
		ExpiresAt: ptr(time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)),
	}

	err := d.CreateCIToken(ctx, token)
	require.NoError(t, err)

	got, err := d.GetCITokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, user.ID, got.CreatedBy)
	assert.Equal(t, "GitHub Actions deploy", got.Name)
	assert.Equal(t, "sha256-ci-hash-1", got.TokenHash)
	assert.Equal(t, `["agents:read","shell:execute"]`, got.Scopes)
	require.NotNil(t, got.ExpiresAt)
	assert.Nil(t, got.LastUsedAt)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestCreateCIToken_NoExpiry(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := &db.CIToken{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		CreatedBy: user.ID,
		Name:      "Permanent token",
		TokenHash: "sha256-ci-hash-noexp",
		Scopes:    `["audit:read"]`,
	}

	err := d.CreateCIToken(ctx, token)
	require.NoError(t, err)

	got, err := d.GetCITokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ExpiresAt)
}

func TestGetCITokenByHash(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	hash := "sha256-ci-hash-lookup"
	token := seedCIToken(t, d, tenantID, user.ID, "hash-lookup", hash)

	got, err := d.GetCITokenByHash(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, hash, got.TokenHash)

	// Non-existent hash returns nil, nil
	got, err = d.GetCITokenByHash(ctx, "no-such-hash")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetCITokenByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	got, err := d.GetCITokenByID(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdateCITokenLastUsed(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := seedCIToken(t, d, tenantID, user.ID, "last-used-test", "sha256-ci-hash-lu")

	// Initially nil
	got, err := d.GetCITokenByID(ctx, token.ID)
	require.NoError(t, err)
	assert.Nil(t, got.LastUsedAt)

	// Update
	err = d.UpdateCITokenLastUsed(ctx, token.ID)
	require.NoError(t, err)

	got, err = d.GetCITokenByID(ctx, token.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.NotEmpty(t, *got.LastUsedAt)
}

func TestDeleteCIToken(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	token := seedCIToken(t, d, tenantID, user.ID, "delete-test", "sha256-ci-hash-del")

	err := d.DeleteCIToken(ctx, token.ID)
	require.NoError(t, err)

	got, err := d.GetCITokenByID(ctx, token.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteCIToken_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.DeleteCIToken(ctx, uuid.NewString())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ci token not found")
}

func TestListCITokensPaginated(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "admin@test.com", "platform_owner")

	// Create 3 tokens with known timestamps
	timestamps := []string{
		"2025-01-01T00:00:00Z",
		"2025-01-02T00:00:00Z",
		"2025-01-03T00:00:00Z",
	}

	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		ids[i] = uuid.NewString()
		token := &db.CIToken{
			ID:        ids[i],
			TenantID:  tenantID,
			CreatedBy: user.ID,
			Name:      "token-" + ids[i][:8],
			TokenHash: "hash-ci-" + ids[i],
			Scopes:    `["agents:read"]`,
		}
		err := d.CreateCIToken(ctx, token)
		require.NoError(t, err)
		_, err = d.Conn().ExecContext(ctx,
			"UPDATE ci_tokens SET created_at = ? WHERE id = ?", timestamps[i], ids[i])
		require.NoError(t, err)
	}

	// List all (limit 10)
	tokens, err := d.ListCITokensPaginated(ctx, db.CITokenListParams{
		TenantID: tenantID,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, tokens, 3)

	// Newest first
	assert.Equal(t, ids[2], tokens[0].ID)
	assert.Equal(t, ids[1], tokens[1].ID)
	assert.Equal(t, ids[0], tokens[2].ID)

	// Paginate: limit 1
	tokens, err = d.ListCITokensPaginated(ctx, db.CITokenListParams{
		TenantID: tenantID,
		Limit:    1,
	})
	require.NoError(t, err)
	require.Len(t, tokens, 2) // limit+1
	assert.Equal(t, ids[2], tokens[0].ID)

	// Second page using cursor
	tokens, err = d.ListCITokensPaginated(ctx, db.CITokenListParams{
		TenantID: tenantID,
		Limit:    10,
		CursorAt: timestamps[2],
		CursorID: ids[2],
	})
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	assert.Equal(t, ids[1], tokens[0].ID)
	assert.Equal(t, ids[0], tokens[1].ID)
}

func TestListCITokensPaginated_TenantIsolation(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenant1 := seedTenant(t, d)
	user1 := seedUser(t, d, tenant1, "admin1@test.com", "platform_owner")
	seedCIToken(t, d, tenant1, user1.ID, "tenant1-token", "hash-t1")

	tenant2 := seedTenant(t, d)
	user2 := seedUser(t, d, tenant2, "admin2@test.com", "platform_owner")
	seedCIToken(t, d, tenant2, user2.ID, "tenant2-token", "hash-t2")

	// Tenant 1 should only see their token
	tokens, err := d.ListCITokensPaginated(ctx, db.CITokenListParams{
		TenantID: tenant1,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "tenant1-token", tokens[0].Name)

	// Tenant 2 should only see their token
	tokens, err = d.ListCITokensPaginated(ctx, db.CITokenListParams{
		TenantID: tenant2,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "tenant2-token", tokens[0].Name)
}

func TestValidateCITokenScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		errMsg string
	}{
		{"valid single", []string{"agents:read"}, ""},
		{"valid multiple", []string{"agents:read", "shell:execute", "files:read"}, ""},
		{"empty", []string{}, "at least one scope is required"},
		{"nil", nil, "at least one scope is required"},
		{"invalid scope", []string{"agents:read", "invalid:scope"}, "invalid scope: invalid:scope"},
		{"duplicate", []string{"agents:read", "agents:read"}, "duplicate scope: agents:read"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := db.ValidateCITokenScopes(tt.scopes)
			assert.Equal(t, tt.errMsg, msg)
		})
	}
}
