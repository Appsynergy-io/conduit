package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// newTestDB creates an in-memory SQLite database with migrations applied.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

// seedTenant creates a test tenant and returns its ID.
func seedTenant(t *testing.T, d *db.DB) string {
	t.Helper()
	id := uuid.NewString()
	err := d.CreateTenant(context.Background(), id, "Test Org")
	require.NoError(t, err)
	return id
}

// seedUser creates a test user in the given tenant and returns the User.
func seedUser(t *testing.T, d *db.DB, tenantID, email, role string) *db.User {
	t.Helper()
	u := &db.User{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Email:    email,
		FirstName: "Test",
		LastName:  "User",
		Role:     role,
		Status:   "active",
	}
	err := d.CreateUser(context.Background(), u)
	require.NoError(t, err)
	return u
}

// ptr returns a pointer to the given string.
func ptr(s string) *string {
	return &s
}

// intPtr returns a pointer to the given int.
func intPtr(i int) *int {
	return &i
}
