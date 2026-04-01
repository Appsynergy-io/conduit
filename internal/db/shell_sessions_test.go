package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAgent(t *testing.T, d *db.DB, tenantID, hostname string) *db.Agent {
	t.Helper()
	a := &db.Agent{
		ID:           uuid.NewString(),
		TenantID:     tenantID,
		Hostname:     hostname,
		OS:           ptr("linux"),
		Arch:         ptr("amd64"),
		AgentKeyHash: "hash-" + uuid.NewString()[:8],
		Status:       "online",
	}
	err := d.CreateAgent(context.Background(), a)
	require.NoError(t, err)
	return a
}

func seedShellSession(t *testing.T, d *db.DB, tenantID, agentID, userID string) *db.ShellSession {
	t.Helper()
	s := &db.ShellSession{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		AgentID:   agentID,
		UserID:    userID,
		Status:    "active",
		Shell:     ptr("/bin/bash"),
		Cols:      intPtr(80),
		Rows:      intPtr(24),
		Recording: 1,
	}
	err := d.CreateShellSession(context.Background(), s)
	require.NoError(t, err)
	return s
}

// ---------------------------------------------------------------------------
// Shell Sessions
// ---------------------------------------------------------------------------

func TestCreateShellSession_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "shell@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "web-01")

	s := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, agent.ID, got.AgentID)
	assert.Equal(t, user.ID, got.UserID)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "/bin/bash", *got.Shell)
	assert.Equal(t, 80, *got.Cols)
	assert.Equal(t, 24, *got.Rows)
	assert.Equal(t, 1, got.Recording)
	assert.NotEmpty(t, got.CreatedAt)
	assert.Nil(t, got.ClosedAt)
}

func TestGetShellSessionByID_NotFound(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetShellSessionByID(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestCloseShellSession(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "close@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "web-02")
	s := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	err := d.CloseShellSession(context.Background(), s.ID)
	require.NoError(t, err)

	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "closed", got.Status)
	assert.NotNil(t, got.ClosedAt)
}

func TestListActiveShellSessions(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "active@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "web-03")

	s1 := seedShellSession(t, d, tenantID, agent.ID, user.ID)
	seedShellSession(t, d, tenantID, agent.ID, user.ID)

	// Close first session
	err := d.CloseShellSession(context.Background(), s1.ID)
	require.NoError(t, err)

	active, err := d.ListActiveShellSessions(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Len(t, active, 1, "only the unclosed session should be returned")
}

func TestListActiveShellSessions_Empty(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)

	active, err := d.ListActiveShellSessions(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Empty(t, active)
}

// ---------------------------------------------------------------------------
// Shell Recordings
// ---------------------------------------------------------------------------

func seedShellRecording(t *testing.T, d *db.DB, tenantID, sessionID, agentID, userID string) *db.ShellRecording {
	t.Helper()
	rec := &db.ShellRecording{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		SessionID:     sessionID,
		AgentID:       agentID,
		UserID:        userID,
		AgentHostname: "web-01",
		UserEmail:     "user@test.com",
		Duration:      120,
		SizeBytes:     4096,
		Format:        "asciicast-v2",
		Data:          []byte(`{"version":2,"width":80,"height":24,"timestamp":1234567890}`),
	}
	err := d.CreateShellRecording(context.Background(), rec)
	require.NoError(t, err)
	return rec
}

func TestCreateShellRecording_Success(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "rec@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "rec-01")
	session := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	rec := seedShellRecording(t, d, tenantID, session.ID, agent.ID, user.ID)

	got, err := d.GetShellRecording(context.Background(), rec.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, rec.ID, got.ID)
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, session.ID, got.SessionID)
	assert.Equal(t, agent.ID, got.AgentID)
	assert.Equal(t, user.ID, got.UserID)
	assert.Equal(t, "web-01", got.AgentHostname)
	assert.Equal(t, 120, got.Duration)
	assert.Equal(t, 4096, got.SizeBytes)
	assert.Equal(t, "asciicast-v2", got.Format)
	assert.NotEmpty(t, got.Data)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestGetShellRecording_NotFound(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)

	got, err := d.GetShellRecording(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetShellRecording_TenantIsolation(t *testing.T) {
	d := newTestDB(t)
	tenantID1 := seedTenant(t, d)
	tenantID2 := seedTenant(t, d)

	// Create recording under tenant1
	d.SetTenantID(tenantID1)
	user := seedUser(t, d, tenantID1, "iso@test.com", "org_member")
	agent := seedAgent(t, d, tenantID1, "iso-01")
	session := seedShellSession(t, d, tenantID1, agent.ID, user.ID)
	rec := seedShellRecording(t, d, tenantID1, session.ID, agent.ID, user.ID)

	// Switch to tenant2 — should not find the recording
	d.SetTenantID(tenantID2)
	got, err := d.GetShellRecording(context.Background(), rec.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "recording should not be visible to different tenant")
}

func TestListShellRecordings(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "list@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-01")

	for i := 0; i < 5; i++ {
		session := seedShellSession(t, d, tenantID, agent.ID, user.ID)
		seedShellRecording(t, d, tenantID, session.ID, agent.ID, user.ID)
	}

	recordings, total, err := d.ListShellRecordings(context.Background(), "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, recordings, 5)
}

func TestListShellRecordings_FilterByAgent(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "filter@test.com", "org_member")
	agent1 := seedAgent(t, d, tenantID, "filter-01")
	agent2 := seedAgent(t, d, tenantID, "filter-02")

	for i := 0; i < 3; i++ {
		session := seedShellSession(t, d, tenantID, agent1.ID, user.ID)
		seedShellRecording(t, d, tenantID, session.ID, agent1.ID, user.ID)
	}
	session := seedShellSession(t, d, tenantID, agent2.ID, user.ID)
	seedShellRecording(t, d, tenantID, session.ID, agent2.ID, user.ID)

	recordings, total, err := d.ListShellRecordings(context.Background(), agent1.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, recordings, 3)

	recordings, total, err = d.ListShellRecordings(context.Background(), agent2.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, recordings, 1)
}

func TestListShellRecordings_Pagination(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "page@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "page-01")

	for i := 0; i < 5; i++ {
		session := seedShellSession(t, d, tenantID, agent.ID, user.ID)
		seedShellRecording(t, d, tenantID, session.ID, agent.ID, user.ID)
	}

	page1, total, err := d.ListShellRecordings(context.Background(), "", 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page1, 2)

	page2, total, err := d.ListShellRecordings(context.Background(), "", 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page2, 2)
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestListShellRecordings_NoDataField(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "nodata@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "nodata-01")
	session := seedShellSession(t, d, tenantID, agent.ID, user.ID)
	seedShellRecording(t, d, tenantID, session.ID, agent.ID, user.ID)

	recordings, _, err := d.ListShellRecordings(context.Background(), "", 10, 0)
	require.NoError(t, err)
	require.Len(t, recordings, 1)
	// List should not include data (performance optimization)
	assert.Empty(t, recordings[0].Data)
}
