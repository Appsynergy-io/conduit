package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeAuditEntry(tenantID string) *db.AuditEntry {
	return &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		EventType: "auth.login",
		Outcome:   "success",
	}
}

func TestInsertAuditLog(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	entry := &db.AuditEntry{
		ID:               uuid.NewString(),
		TenantID:         tid,
		EventType:        "auth.login",
		UserID:           ptr("user-123"),
		UserEmail:        ptr("admin@example.com"),
		AgentID:          ptr("agent-456"),
		AgentHostname:    ptr("web-01"),
		SourceIP:         ptr("192.168.1.100"),
		UserAgent:        ptr("Mozilla/5.0"),
		Details:          ptr(`{"method":"passkey"}`),
		Outcome:          "success",
		AlgorithmUsed:    ptr("Ed25519"),
		AlgorithmWarning: ptr("classical algorithm"),
		Timestamp:        "2026-03-31T12:00:00Z",
	}

	err := d.InsertAuditLog(ctx, entry)
	require.NoError(t, err)

	logs, err := d.ListAuditLogs(ctx, tid, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	got := logs[0]
	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, tid, got.TenantID)
	assert.Equal(t, "auth.login", got.EventType)
	assert.Equal(t, "user-123", *got.UserID)
	assert.Equal(t, "admin@example.com", *got.UserEmail)
	assert.Equal(t, "agent-456", *got.AgentID)
	assert.Equal(t, "web-01", *got.AgentHostname)
	assert.Equal(t, "192.168.1.100", *got.SourceIP)
	assert.Equal(t, "Mozilla/5.0", *got.UserAgent)
	assert.Equal(t, `{"method":"passkey"}`, *got.Details)
	assert.Equal(t, "success", got.Outcome)
	assert.Equal(t, "Ed25519", *got.AlgorithmUsed)
	assert.Equal(t, "classical algorithm", *got.AlgorithmWarning)
	assert.Equal(t, "2026-03-31T12:00:00Z", got.Timestamp)
}

func TestInsertAuditLog_SetsTimestamp(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	entry := makeAuditEntry(tid)
	entry.Timestamp = "" // leave empty, expect auto-set

	err := d.InsertAuditLog(ctx, entry)
	require.NoError(t, err)

	logs, err := d.ListAuditLogs(ctx, tid, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	assert.NotEmpty(t, logs[0].Timestamp, "Timestamp should be auto-set when left empty")
}

func TestListAuditLogs_Pagination(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	// Insert 5 entries with distinct timestamps so ordering is deterministic.
	for i := 0; i < 5; i++ {
		entry := makeAuditEntry(tid)
		entry.Timestamp = "2026-03-31T12:00:0" + string(rune('0'+i)) + "Z"
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}

	// First page: limit=2, offset=0 (newest first).
	page1, err := d.ListAuditLogs(ctx, tid, 2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "2026-03-31T12:00:04Z", page1[0].Timestamp)
	assert.Equal(t, "2026-03-31T12:00:03Z", page1[1].Timestamp)

	// Second page: limit=2, offset=2.
	page2, err := d.ListAuditLogs(ctx, tid, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "2026-03-31T12:00:02Z", page2[0].Timestamp)
	assert.Equal(t, "2026-03-31T12:00:01Z", page2[1].Timestamp)
}

func TestListAuditLogsByEventType(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	types := []string{"auth.login", "shell.start", "auth.login", "file.download", "auth.login"}
	for _, et := range types {
		entry := makeAuditEntry(tid)
		entry.EventType = et
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}

	logs, err := d.ListAuditLogsByEventType(ctx, tid, "auth.login", 100, 0)
	require.NoError(t, err)
	assert.Len(t, logs, 3)

	for _, l := range logs {
		assert.Equal(t, "auth.login", l.EventType)
	}

	shell, err := d.ListAuditLogsByEventType(ctx, tid, "shell.start", 100, 0)
	require.NoError(t, err)
	assert.Len(t, shell, 1)
}

func TestListAuditLogsByUser(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	userA := uuid.NewString()
	userB := uuid.NewString()

	for i := 0; i < 3; i++ {
		entry := makeAuditEntry(tid)
		entry.UserID = &userA
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}
	for i := 0; i < 2; i++ {
		entry := makeAuditEntry(tid)
		entry.UserID = &userB
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}

	logsA, err := d.ListAuditLogsByUser(ctx, tid, userA, 100, 0)
	require.NoError(t, err)
	assert.Len(t, logsA, 3)

	for _, l := range logsA {
		require.NotNil(t, l.UserID)
		assert.Equal(t, userA, *l.UserID)
	}

	logsB, err := d.ListAuditLogsByUser(ctx, tid, userB, 100, 0)
	require.NoError(t, err)
	assert.Len(t, logsB, 2)
}

func TestCountAuditLogs(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		entry := makeAuditEntry(tid)
		require.NoError(t, d.InsertAuditLog(ctx, entry))
	}

	count, err := d.CountAuditLogs(ctx, tid)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestAuditLog_AppendOnly(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	// Insert with all nullable fields nil.
	entry := &db.AuditEntry{
		ID:        uuid.NewString(),
		TenantID:  tid,
		EventType: "agent.connected",
		Outcome:   "success",
	}

	err := d.InsertAuditLog(ctx, entry)
	require.NoError(t, err)

	logs, err := d.ListAuditLogs(ctx, tid, 10, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	got := logs[0]
	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, "agent.connected", got.EventType)
	assert.Equal(t, "success", got.Outcome)
	assert.Nil(t, got.UserID)
	assert.Nil(t, got.UserEmail)
	assert.Nil(t, got.AgentID)
	assert.Nil(t, got.AgentHostname)
	assert.Nil(t, got.SourceIP)
	assert.Nil(t, got.UserAgent)
	assert.Nil(t, got.Details)
	assert.Nil(t, got.AlgorithmUsed)
	assert.Nil(t, got.AlgorithmWarning)
	assert.NotEmpty(t, got.Timestamp)
}
