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
	assert.Equal(t, 0, got.Pinned)
	assert.Equal(t, 0, got.IdleTimeout)
	assert.NotEmpty(t, got.CreatedAt)
	assert.Nil(t, got.DetachedAt)
	assert.Nil(t, got.ClosedAt)
}

func TestCreateShellSession_WithPinnedAndTimeout(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "pinned@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "pin-01")

	s := &db.ShellSession{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		Status:      "active",
		Shell:       ptr("/bin/zsh"),
		Cols:        intPtr(120),
		Rows:        intPtr(40),
		Recording:   1,
		Pinned:      1,
		IdleTimeout: 7200,
	}
	err := d.CreateShellSession(context.Background(), s)
	require.NoError(t, err)

	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Pinned)
	assert.Equal(t, 7200, got.IdleTimeout)
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

func TestDetachShellSession(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "detach@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "detach-01")
	s := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	err := d.DetachShellSession(context.Background(), s.ID)
	require.NoError(t, err)

	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "detached", got.Status)
	assert.NotNil(t, got.DetachedAt)
	assert.Nil(t, got.ClosedAt)
}

func TestUpdateShellSessionStatus_ActiveClearsDetachedAt(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "reattach@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "reattach-01")
	s := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	// Detach first
	err := d.DetachShellSession(context.Background(), s.ID)
	require.NoError(t, err)

	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, "detached", got.Status)
	assert.NotNil(t, got.DetachedAt)

	// Re-attach (set active)
	err = d.UpdateShellSessionStatus(context.Background(), s.ID, "active")
	require.NoError(t, err)

	got, err = d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", got.Status)
	assert.Nil(t, got.DetachedAt, "detached_at should be cleared on re-attach")
}

func TestUpdateShellSessionSettings(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "settings@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "settings-01")
	s := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	// Verify defaults
	got, err := d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Pinned)
	assert.Equal(t, 0, got.IdleTimeout)

	// Update settings
	err = d.UpdateShellSessionSettings(context.Background(), s.ID, 1, 3600)
	require.NoError(t, err)

	got, err = d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Pinned)
	assert.Equal(t, 3600, got.IdleTimeout)

	// Update back
	err = d.UpdateShellSessionSettings(context.Background(), s.ID, 0, 1800)
	require.NoError(t, err)

	got, err = d.GetShellSessionByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Pinned)
	assert.Equal(t, 1800, got.IdleTimeout)
}

func TestListShellSessions_DefaultFilter(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "listdefault@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-default-01")

	s1 := seedShellSession(t, d, tenantID, agent.ID, user.ID) // active
	seedShellSession(t, d, tenantID, agent.ID, user.ID)        // active

	// Detach one
	err := d.DetachShellSession(context.Background(), s1.ID)
	require.NoError(t, err)

	// Default: active + detached
	sessions, err := d.ListShellSessions(context.Background(), tenantID, "", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestListShellSessions_FilterByStatus(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "liststatus@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-status-01")

	s1 := seedShellSession(t, d, tenantID, agent.ID, user.ID)
	s2 := seedShellSession(t, d, tenantID, agent.ID, user.ID)
	s3 := seedShellSession(t, d, tenantID, agent.ID, user.ID)

	// Detach s1
	require.NoError(t, d.DetachShellSession(context.Background(), s1.ID))
	// Close s2
	require.NoError(t, d.CloseShellSession(context.Background(), s2.ID))

	// Only active
	active, err := d.ListShellSessions(context.Background(), tenantID, "active", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, s3.ID, active[0].ID)

	// Only detached
	detached, err := d.ListShellSessions(context.Background(), tenantID, "detached", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, detached, 1)
	assert.Equal(t, s1.ID, detached[0].ID)

	// Only closed
	closed, err := d.ListShellSessions(context.Background(), tenantID, "closed", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, closed, 1)
	assert.Equal(t, s2.ID, closed[0].ID)

	// All
	all, err := d.ListShellSessions(context.Background(), tenantID, "all", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestListShellSessions_FilterByAgent(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "listagent@test.com", "org_member")
	agent1 := seedAgent(t, d, tenantID, "list-agent-01")
	agent2 := seedAgent(t, d, tenantID, "list-agent-02")

	seedShellSession(t, d, tenantID, agent1.ID, user.ID)
	seedShellSession(t, d, tenantID, agent1.ID, user.ID)
	seedShellSession(t, d, tenantID, agent2.ID, user.ID)

	sessions, err := d.ListShellSessions(context.Background(), tenantID, "", agent1.ID, "", nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	sessions, err = d.ListShellSessions(context.Background(), tenantID, "", agent2.ID, "", nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestListShellSessions_FilterByUser(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user1 := seedUser(t, d, tenantID, "listuser1@test.com", "org_member")
	user2 := seedUser(t, d, tenantID, "listuser2@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-user-01")

	seedShellSession(t, d, tenantID, agent.ID, user1.ID)
	seedShellSession(t, d, tenantID, agent.ID, user2.ID)
	seedShellSession(t, d, tenantID, agent.ID, user2.ID)

	sessions, err := d.ListShellSessions(context.Background(), tenantID, "", "", user1.ID, nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	sessions, err = d.ListShellSessions(context.Background(), tenantID, "", "", user2.ID, nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestListShellSessions_FilterByPinned(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "listpinned@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-pinned-01")

	// Create one pinned session
	pinned := &db.ShellSession{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		AgentID:     agent.ID,
		UserID:      user.ID,
		Status:      "active",
		Recording:   1,
		Pinned:      1,
		IdleTimeout: 0,
	}
	require.NoError(t, d.CreateShellSession(context.Background(), pinned))

	// Create one unpinned session
	seedShellSession(t, d, tenantID, agent.ID, user.ID)

	pinnedTrue := true
	sessions, err := d.ListShellSessions(context.Background(), tenantID, "", "", "", &pinnedTrue)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, pinned.ID, sessions[0].ID)

	pinnedFalse := false
	sessions, err = d.ListShellSessions(context.Background(), tenantID, "", "", "", &pinnedFalse)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, 0, sessions[0].Pinned)
}

func TestListShellSessions_ReturnsAll(t *testing.T) {
	d := newTestDB(t)
	tenantID := seedTenant(t, d)
	d.SetTenantID(tenantID)
	user := seedUser(t, d, tenantID, "listorder@test.com", "org_member")
	agent := seedAgent(t, d, tenantID, "list-order-01")

	seedShellSession(t, d, tenantID, agent.ID, user.ID)
	seedShellSession(t, d, tenantID, agent.ID, user.ID)
	seedShellSession(t, d, tenantID, agent.ID, user.ID)

	sessions, err := d.ListShellSessions(context.Background(), tenantID, "", "", "", nil)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
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
