package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSession_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "sess@test.com", "org_member")
	ctx := context.Background()

	s := &db.Session{
		ID:               uuid.NewString(),
		TenantID:         tenantID,
		UserID:           user.ID,
		Type:             "web",
		SourceIP:         ptr("192.0.2.1"),
		UserAgent:        ptr("TestAgent/1.0"),
		RefreshTokenHash: ptr("rthash123"),
		ExpiresAt:        "2099-01-01T00:00:00Z",
	}
	err := d.CreateSession(ctx, s)
	require.NoError(t, err)

	got, err := d.GetSessionByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, user.ID, got.UserID)
	assert.Equal(t, "web", got.Type)
	assert.Equal(t, "192.0.2.1", *got.SourceIP)
	assert.Equal(t, "TestAgent/1.0", *got.UserAgent)
	assert.Equal(t, "rthash123", *got.RefreshTokenHash)
	assert.Equal(t, "2099-01-01T00:00:00Z", got.ExpiresAt)
	assert.NotEmpty(t, got.CreatedAt)
	assert.NotEmpty(t, got.LastActiveAt)
}

func TestListSessionsByUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "list@test.com", "org_member")
	ctx := context.Background()

	s1 := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "web",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}
	s2 := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "cli",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}
	require.NoError(t, d.CreateSession(ctx, s1))
	require.NoError(t, d.CreateSession(ctx, s2))

	sessions, err := d.ListSessionsByUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}

func TestUpdateSessionActivity(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "activity@test.com", "org_member")
	ctx := context.Background()

	s := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "web",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}
	require.NoError(t, d.CreateSession(ctx, s))

	before, err := d.GetSessionByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, before)

	err = d.UpdateSessionActivity(ctx, s.ID)
	require.NoError(t, err)

	after, err := d.GetSessionByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.NotEmpty(t, after.LastActiveAt)
}

func TestDeleteSession(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "del@test.com", "org_member")
	ctx := context.Background()

	s := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "web",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}
	require.NoError(t, d.CreateSession(ctx, s))

	err := d.DeleteSession(ctx, s.ID)
	require.NoError(t, err)

	got, err := d.GetSessionByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDeleteSessionsByUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "delall@test.com", "org_member")
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		s := &db.Session{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			UserID:    user.ID,
			Type:      "web",
			ExpiresAt: "2099-01-01T00:00:00Z",
		}
		require.NoError(t, d.CreateSession(ctx, s))
	}

	err := d.DeleteSessionsByUser(ctx, user.ID)
	require.NoError(t, err)

	sessions, err := d.ListSessionsByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestDeleteExpiredSessions(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	user := seedUser(t, d, tenantID, "expire@test.com", "org_member")
	ctx := context.Background()

	expired := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "web",
		ExpiresAt: "2000-01-01T00:00:00Z",
	}
	valid := &db.Session{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		UserID:    user.ID,
		Type:      "cli",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}
	require.NoError(t, d.CreateSession(ctx, expired))
	require.NoError(t, d.CreateSession(ctx, valid))

	count, err := d.DeleteExpiredSessions(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	sessions, err := d.ListSessionsByUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, valid.ID, sessions[0].ID)
}

func TestGetSessionByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	_ = seedTenant(t, d)
	ctx := context.Background()

	got, err := d.GetSessionByID(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}
