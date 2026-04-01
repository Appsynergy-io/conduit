package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeAgent(tenantID string) *db.Agent {
	return &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Hostname:     "host-" + uuid.NewString()[:8],
		AgentKeyHash: "hash-" + uuid.NewString(),
		Status:       "offline",
	}
}

func TestCreateAgent_Success(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tid,
		Hostname:     "web-01",
		DisplayName:  ptr("Web Server 1"),
		OS:           ptr("linux"),
		Arch:         ptr("amd64"),
		Labels:       ptr(`{"env":"production","role":"web"}`),
		IP:           ptr("10.0.0.5"),
		AgentKeyHash: "argon2id$hash",
		Status:       "online",
		Transport:    ptr("quic"),
		Version:      ptr("0.1.0"),
	}

	err := d.CreateAgent(ctx, a)
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, tid, got.TenantID)
	assert.Equal(t, "web-01", got.Hostname)
	assert.Equal(t, "Web Server 1", *got.DisplayName)
	assert.Equal(t, "linux", *got.OS)
	assert.Equal(t, "amd64", *got.Arch)
	assert.Equal(t, `{"env":"production","role":"web"}`, *got.Labels)
	assert.Equal(t, "10.0.0.5", *got.IP)
	assert.Equal(t, "argon2id$hash", got.AgentKeyHash)
	assert.Equal(t, "online", got.Status)
	assert.Equal(t, "quic", *got.Transport)
	assert.Equal(t, "0.1.0", *got.Version)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestListAgents_OrderedByHostname(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	names := []string{"charlie", "alpha", "bravo"}
	for _, name := range names {
		a := makeAgent(tid)
		a.Hostname = name
		require.NoError(t, d.CreateAgent(ctx, a))
	}

	agents, err := d.ListAgents(ctx, tid)
	require.NoError(t, err)
	require.Len(t, agents, 3)

	assert.Equal(t, "alpha", agents[0].Hostname)
	assert.Equal(t, "bravo", agents[1].Hostname)
	assert.Equal(t, "charlie", agents[2].Hostname)
}

func TestUpdateAgentStatus(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	a.Status = "offline"
	require.NoError(t, d.CreateAgent(ctx, a))

	err := d.UpdateAgentStatus(ctx, a.ID, "online")
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "online", got.Status)
	assert.NotNil(t, got.LastSeenAt)
}

func TestUpdateAgentConnection(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a))

	err := d.UpdateAgentConnection(ctx, a.ID, "online", "quic", "192.168.1.10")
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "online", got.Status)
	assert.Equal(t, "quic", *got.Transport)
	assert.Equal(t, "192.168.1.10", *got.IP)
	assert.NotNil(t, got.ConnectedAt)
	assert.NotNil(t, got.LastSeenAt)
}

func TestUpdateAgentLabels(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a))

	labels := `{"env":"staging","dc":"eu-west-1"}`
	err := d.UpdateAgentLabels(ctx, a.ID, labels)
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Labels)

	assert.Equal(t, labels, *got.Labels)
}

func TestDeleteAgent(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a))

	err := d.DeleteAgent(ctx, a.ID)
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCountAgentsByStatus(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		a := makeAgent(tid)
		a.Status = "online"
		require.NoError(t, d.CreateAgent(ctx, a))
	}

	offline := makeAgent(tid)
	offline.Status = "offline"
	require.NoError(t, d.CreateAgent(ctx, offline))

	onlineCount, err := d.CountAgentsByStatus(ctx, tid, "online")
	require.NoError(t, err)
	assert.Equal(t, 2, onlineCount)

	offlineCount, err := d.CountAgentsByStatus(ctx, tid, "offline")
	require.NoError(t, err)
	assert.Equal(t, 1, offlineCount)
}

func TestGetAgentByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	got, err := d.GetAgentByID(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdateAgentMetadata(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a))

	dn := "My Server"
	labels := `{"env":"staging","region":"eu"}`
	err := d.UpdateAgentMetadata(ctx, a.ID, &dn, &labels)
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.DisplayName)
	assert.Equal(t, "My Server", *got.DisplayName)
	require.NotNil(t, got.Labels)
	assert.Equal(t, labels, *got.Labels)
}

func TestUpdateAgentMetadata_ClearFields(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a := makeAgent(tid)
	dn := "Initial Name"
	labels := `{"env":"prod"}`
	a.DisplayName = &dn
	a.Labels = &labels
	require.NoError(t, d.CreateAgent(ctx, a))

	// Clear both fields
	err := d.UpdateAgentMetadata(ctx, a.ID, nil, nil)
	require.NoError(t, err)

	got, err := d.GetAgentByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, got.DisplayName)
	assert.Nil(t, got.Labels)
}

func TestListLabels(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	// Create agents with various labels
	a1 := makeAgent(tid)
	a1.Labels = ptr(`{"env":"production","role":"web"}`)
	require.NoError(t, d.CreateAgent(ctx, a1))

	a2 := makeAgent(tid)
	a2.Labels = ptr(`{"env":"staging","role":"api"}`)
	require.NoError(t, d.CreateAgent(ctx, a2))

	a3 := makeAgent(tid)
	a3.Labels = ptr(`{"env":"production","dc":"us-east-1"}`)
	require.NoError(t, d.CreateAgent(ctx, a3))

	// Agent with no labels
	a4 := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a4))

	labels, err := d.ListLabels(ctx, tid)
	require.NoError(t, err)
	require.Len(t, labels, 3) // env, role, dc

	// Results sorted by key
	assert.Equal(t, "dc", labels[0].Key)
	assert.Equal(t, []string{"us-east-1"}, labels[0].Values)
	assert.Equal(t, 1, labels[0].Count)

	assert.Equal(t, "env", labels[1].Key)
	assert.Equal(t, []string{"production", "staging"}, labels[1].Values)
	assert.Equal(t, 3, labels[1].Count)

	assert.Equal(t, "role", labels[2].Key)
	assert.Equal(t, []string{"api", "web"}, labels[2].Values)
	assert.Equal(t, 2, labels[2].Count)
}

func TestListLabels_EmptyTenant(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	labels, err := d.ListLabels(ctx, tid)
	require.NoError(t, err)
	assert.Empty(t, labels)
}

func TestListAgentsPaginated_LabelFilter(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	a1 := makeAgent(tid)
	a1.Labels = ptr(`{"env":"production","role":"web"}`)
	require.NoError(t, d.CreateAgent(ctx, a1))

	a2 := makeAgent(tid)
	a2.Labels = ptr(`{"env":"staging","role":"api"}`)
	require.NoError(t, d.CreateAgent(ctx, a2))

	a3 := makeAgent(tid)
	a3.Labels = ptr(`{"env":"production","role":"api"}`)
	require.NoError(t, d.CreateAgent(ctx, a3))

	a4 := makeAgent(tid)
	require.NoError(t, d.CreateAgent(ctx, a4))

	// Exact match
	agents, err := d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels:   []db.LabelFilter{{Key: "env", Values: []string{"production"}}},
	})
	require.NoError(t, err)
	assert.Len(t, agents, 2)

	// OR match
	agents, err = d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels:   []db.LabelFilter{{Key: "role", Values: []string{"web", "api"}}},
	})
	require.NoError(t, err)
	assert.Len(t, agents, 3)

	// AND match (multiple filters)
	agents, err = d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels: []db.LabelFilter{
			{Key: "env", Values: []string{"production"}},
			{Key: "role", Values: []string{"api"}},
		},
	})
	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, a3.ID, agents[0].ID)

	// Existence check
	agents, err = d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels:   []db.LabelFilter{{Key: "role"}},
	})
	require.NoError(t, err)
	assert.Len(t, agents, 3)

	// Negate
	agents, err = d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels:   []db.LabelFilter{{Key: "env", Values: []string{"staging"}, Negate: true}},
	})
	require.NoError(t, err)
	// a1 (production), a3 (production), a4 (no labels)
	assert.Len(t, agents, 3)

	// Negate existence
	agents, err = d.ListAgentsPaginated(ctx, db.AgentListParams{
		TenantID: tid,
		Limit:    100,
		Labels:   []db.LabelFilter{{Key: "role", Negate: true}},
	})
	require.NoError(t, err)
	// Only a4 has no role label, but a3 also has no... wait a3 has role:api
	// Only a4 has no labels at all
	assert.Len(t, agents, 1)
	assert.Equal(t, a4.ID, agents[0].ID)
}
