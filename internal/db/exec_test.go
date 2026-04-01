package db_test

import (
	"context"
	"testing"

	"github.com/appsynergy-io/conduit/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeExecJob(tenantID string) *db.BulkExecJob {
	tc := 3
	return &db.BulkExecJob{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		Command:     "uptime",
		Status:      "pending",
		TargetCount: &tc,
	}
}

func TestCreateBulkExecJob(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	err := d.CreateBulkExecJob(ctx, job)
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, tid, got.TenantID)
	assert.Equal(t, "uptime", got.Command)
	assert.Equal(t, "pending", got.Status)
	assert.Equal(t, 3, *got.TargetCount)
	assert.NotEmpty(t, got.CreatedAt)
}

func TestGetBulkExecJob_NotFound(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	got, err := d.GetBulkExecJob(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdateBulkExecJobStatus(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	require.NoError(t, d.CreateBulkExecJob(ctx, job))

	err := d.UpdateBulkExecJobStatus(ctx, job.ID, "running")
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", got.Status)
}

func TestUpdateBulkExecJobResults(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	require.NoError(t, d.CreateBulkExecJob(ctx, job))

	results := `[{"agentId":"a1","status":"completed","exitCode":0}]`
	err := d.UpdateBulkExecJobResults(ctx, job.ID, 1, 0, results, false)
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.CompletedCount)
	assert.Equal(t, 0, got.FailedCount)
	require.NotNil(t, got.Results)
	assert.Contains(t, *got.Results, "a1")
}

func TestUpdateBulkExecJobResults_Complete(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	job.Status = "running"
	require.NoError(t, d.CreateBulkExecJob(ctx, job))

	results := `[{"agentId":"a1","status":"completed"},{"agentId":"a2","status":"completed"}]`
	err := d.UpdateBulkExecJobResults(ctx, job.ID, 2, 0, results, true)
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestCancelBulkExecJob(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	job.Status = "running"
	require.NoError(t, d.CreateBulkExecJob(ctx, job))

	err := d.CancelBulkExecJob(ctx, job.ID)
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestCancelBulkExecJob_AlreadyCompleted(t *testing.T) {
	d := newTestDB(t)
	tid := seedTenant(t, d)
	ctx := context.Background()

	job := makeExecJob(tid)
	job.Status = "completed"
	require.NoError(t, d.CreateBulkExecJob(ctx, job))

	// Cancel should not change status since it's already completed
	err := d.CancelBulkExecJob(ctx, job.ID)
	require.NoError(t, err)

	got, err := d.GetBulkExecJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status, "already-completed job should not change")
}
