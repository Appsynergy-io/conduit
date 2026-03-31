package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_InMemory(t *testing.T) {
	ctx := context.Background()

	d, err := db.New(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	assert.Empty(t, d.TenantID(), "TenantID should be empty before setup")
}

func TestMigrations_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "idempotent.db")
	ctx := context.Background()

	// First open applies migrations.
	d1, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, d1.Close())

	// Second open on the same file must not fail (migrations already applied).
	d2, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d2.Close() })
}

func TestTenantID_AfterSetup(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	assert.Empty(t, d.TenantID(), "TenantID should be empty before tenant creation")

	tenantID := seedTenant(t, d)

	assert.Equal(t, tenantID, d.TenantID(), "TenantID should match after CreateTenant")

	// Verify persistence by opening a file-based DB, creating a tenant, closing, and reopening.
	dbPath := filepath.Join(t.TempDir(), "tenant-persist.db")

	d2, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	assert.Empty(t, d2.TenantID())

	err = d2.CreateTenant(ctx, tenantID, "Persistent Org")
	require.NoError(t, err)
	require.NoError(t, d2.Close())

	d3, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d3.Close() })

	assert.Equal(t, tenantID, d3.TenantID(), "TenantID should be loaded from DB on startup")
}

func TestCreateTenant_Duplicate(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	tenantID := seedTenant(t, d)

	err := d.CreateTenant(ctx, tenantID, "Duplicate Org")
	assert.Error(t, err, "creating a tenant with the same ID twice should fail")
}
