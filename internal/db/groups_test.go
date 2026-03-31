package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGroup_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	g := &db.Group{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Name:        "engineering",
		Description: ptr("Engineering team"),
	}
	err := d.CreateGroup(ctx, g)
	require.NoError(t, err)

	got, err := d.GetGroupByID(ctx, g.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, g.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, "engineering", got.Name)
	assert.Equal(t, "Engineering team", *got.Description)
	assert.NotEmpty(t, got.CreatedAt)
	assert.NotEmpty(t, got.UpdatedAt)
}

func TestListGroups_OrderedByName(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	names := []string{"zulu", "alpha", "mike"}
	for _, name := range names {
		err := d.CreateGroup(ctx, &db.Group{
			ID:       uuid.NewString(),
			TenantID: tenantID,
			Name:     name,
		})
		require.NoError(t, err)
	}

	groups, err := d.ListGroups(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, groups, 3)
	assert.Equal(t, "alpha", groups[0].Name)
	assert.Equal(t, "mike", groups[1].Name)
	assert.Equal(t, "zulu", groups[2].Name)
}

func TestUpdateGroup(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	g := &db.Group{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Name:        "old-name",
		Description: ptr("old desc"),
	}
	require.NoError(t, d.CreateGroup(ctx, g))

	err := d.UpdateGroup(ctx, g.ID, "new-name", ptr("new desc"))
	require.NoError(t, err)

	got, err := d.GetGroupByID(ctx, g.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "new-name", got.Name)
	assert.Equal(t, "new desc", *got.Description)
}

func TestDeleteGroup(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	g := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     "to-delete",
	}
	require.NoError(t, d.CreateGroup(ctx, g))

	err := d.DeleteGroup(ctx, g.ID)
	require.NoError(t, err)

	got, err := d.GetGroupByID(ctx, g.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAddRemoveUserFromGroup(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	user := seedUser(t, d, tenantID, "member@test.com", "org_member")
	g := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     "team",
	}
	require.NoError(t, d.CreateGroup(ctx, g))

	err := d.AddUserToGroup(ctx, tenantID, g.ID, user.ID)
	require.NoError(t, err)

	members, err := d.ListGroupMembers(ctx, g.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, user.ID, members[0])

	err = d.RemoveUserFromGroup(ctx, g.ID, user.ID)
	require.NoError(t, err)

	members, err = d.ListGroupMembers(ctx, g.ID)
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestAddRemoveAgentFromGroup(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	agent := &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Hostname:     "test-host",
		AgentKeyHash: "hash",
		Status:       "online",
	}
	require.NoError(t, d.CreateAgent(ctx, agent))

	g := &db.Group{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     "agent-group",
	}
	require.NoError(t, d.CreateGroup(ctx, g))

	err := d.AddAgentToGroup(ctx, tenantID, g.ID, agent.ID)
	require.NoError(t, err)

	agentIDs, err := d.ListAgentGroupMembers(ctx, g.ID)
	require.NoError(t, err)
	require.Len(t, agentIDs, 1)
	assert.Equal(t, agent.ID, agentIDs[0])

	err = d.RemoveAgentFromGroup(ctx, g.ID, agent.ID)
	require.NoError(t, err)

	agentIDs, err = d.ListAgentGroupMembers(ctx, g.ID)
	require.NoError(t, err)
	assert.Empty(t, agentIDs)
}

func TestListGroupsForUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	ctx := context.Background()

	user := seedUser(t, d, tenantID, "multi@test.com", "org_member")

	g1 := &db.Group{ID: uuid.NewString(), TenantID: tenantID, Name: "beta"}
	g2 := &db.Group{ID: uuid.NewString(), TenantID: tenantID, Name: "alpha"}
	require.NoError(t, d.CreateGroup(ctx, g1))
	require.NoError(t, d.CreateGroup(ctx, g2))

	require.NoError(t, d.AddUserToGroup(ctx, tenantID, g1.ID, user.ID))
	require.NoError(t, d.AddUserToGroup(ctx, tenantID, g2.ID, user.ID))

	groups, err := d.ListGroupsForUser(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "alpha", groups[0].Name)
	assert.Equal(t, "beta", groups[1].Name)
}

func TestGetGroupByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	_ = seedTenant(t, d)
	ctx := context.Background()

	got, err := d.GetGroupByID(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}
