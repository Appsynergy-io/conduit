package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSetupState(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.CreateSetupState(ctx, "abc123hash")
	require.NoError(t, err)

	state, err := d.GetSetupState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, "abc123hash", state.SetupTokenHash)
	assert.Equal(t, "domain_config", state.CurrentStep)
	assert.Equal(t, "[]", state.CompletedSteps)
	assert.False(t, state.Completed)
	assert.Nil(t, state.CompletedAt)
	assert.NotEmpty(t, state.CreatedAt)
}

func TestGetSetupState_Empty(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	state, err := d.GetSetupState(ctx)
	require.NoError(t, err)
	assert.Nil(t, state, "should return nil when no setup state exists")
}

func TestUpdateSetupStep(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.CreateSetupState(ctx, "hash")
	require.NoError(t, err)

	err = d.UpdateSetupStep(ctx, "passkey_registration", `["domain_config","admin_account"]`)
	require.NoError(t, err)

	state, err := d.GetSetupState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "passkey_registration", state.CurrentStep)
	assert.Equal(t, `["domain_config","admin_account"]`, state.CompletedSteps)
	assert.False(t, state.Completed, "should not be marked complete yet")
}

func TestCompleteSetup(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.CreateSetupState(ctx, "hash")
	require.NoError(t, err)

	err = d.CompleteSetup(ctx)
	require.NoError(t, err)

	state, err := d.GetSetupState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.True(t, state.Completed)
	assert.NotNil(t, state.CompletedAt)
	assert.Equal(t, "complete", state.CurrentStep)
}

func TestDeleteSetupToken(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.CreateSetupState(ctx, "secret-hash")
	require.NoError(t, err)

	err = d.DeleteSetupToken(ctx)
	require.NoError(t, err)

	state, err := d.GetSetupState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "", state.SetupTokenHash, "token hash should be cleared")
}

func TestIsSetupComplete(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// No state yet — not complete
	complete, err := d.IsSetupComplete(ctx)
	require.NoError(t, err)
	assert.False(t, complete)

	// Create state — still not complete
	err = d.CreateSetupState(ctx, "hash")
	require.NoError(t, err)

	complete, err = d.IsSetupComplete(ctx)
	require.NoError(t, err)
	assert.False(t, complete)

	// Complete setup
	err = d.CompleteSetup(ctx)
	require.NoError(t, err)

	complete, err = d.IsSetupComplete(ctx)
	require.NoError(t, err)
	assert.True(t, complete)
}

func TestCreateSetupState_Duplicate(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.CreateSetupState(ctx, "hash1")
	require.NoError(t, err)

	// Should fail — id=1 singleton constraint
	err = d.CreateSetupState(ctx, "hash2")
	assert.Error(t, err, "duplicate setup state should be rejected")
}
