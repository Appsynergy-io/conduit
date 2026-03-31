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
