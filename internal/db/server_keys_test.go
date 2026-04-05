package db_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/appsynergy-io/conduit/internal/db"
)

func TestServerKey_SaveAndGet(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// No key initially
	got, err := d.GetServerKey(ctx, db.ServerKeyJWTEd25519)
	require.NoError(t, err)
	assert.Nil(t, got)

	// Generate and save
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.NoError(t, d.SaveServerKey(ctx, db.ServerKeyJWTEd25519, priv))

	// Retrieve
	got, err = d.GetServerKey(ctx, db.ServerKeyJWTEd25519)
	require.NoError(t, err)
	require.Len(t, got, ed25519.PrivateKeySize)
	assert.True(t, bytes.Equal(got, priv), "retrieved key should match saved key")
}

func TestServerKey_DuplicateInsertFails(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.NoError(t, d.SaveServerKey(ctx, db.ServerKeyJWTEd25519, priv))

	// Second save with the same key_type should fail (primary key conflict)
	err = d.SaveServerKey(ctx, db.ServerKeyJWTEd25519, priv)
	assert.Error(t, err)
}

func TestServerKey_DifferentTypes(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	_, p1, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, p2, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	require.NoError(t, d.SaveServerKey(ctx, "type_a", p1))
	require.NoError(t, d.SaveServerKey(ctx, "type_b", p2))

	got1, err := d.GetServerKey(ctx, "type_a")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got1, p1))

	got2, err := d.GetServerKey(ctx, "type_b")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got2, p2))
}
