package db

import (
	"context"
	"database/sql"
	"fmt"
)

// BulkExecJob represents a row from the bulk_exec_jobs table.
type BulkExecJob struct {
	ID             string
	TenantID       string
	Command        string
	Status         string // pending, running, completed, cancelled, failed
	TargetCount    *int
	CompletedCount int
	FailedCount    int
	Results        *string // JSON array of per-agent results
	CreatedBy      *string
	CreatedAt      string
	CompletedAt    *string
}

// CreateBulkExecJob inserts a new bulk exec job.
func (d *DB) CreateBulkExecJob(ctx context.Context, job *BulkExecJob) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO bulk_exec_jobs (
			id, tenant_id, command, status, target_count,
			completed_count, failed_count, results, created_by, created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.TenantID, job.Command, job.Status, job.TargetCount,
		job.CompletedCount, job.FailedCount, job.Results, job.CreatedBy,
		Now(), job.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting bulk exec job: %w", err)
	}
	return nil
}

// GetBulkExecJob returns a bulk exec job by ID.
func (d *DB) GetBulkExecJob(ctx context.Context, id string) (*BulkExecJob, error) {
	var j BulkExecJob
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, command, status, target_count,
		       completed_count, failed_count, results, created_by, created_at, completed_at
		FROM bulk_exec_jobs WHERE id = ?`, id,
	).Scan(
		&j.ID, &j.TenantID, &j.Command, &j.Status, &j.TargetCount,
		&j.CompletedCount, &j.FailedCount, &j.Results, &j.CreatedBy,
		&j.CreatedAt, &j.CompletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying bulk exec job: %w", err)
	}
	return &j, nil
}

// UpdateBulkExecJobStatus updates the status of a bulk exec job.
func (d *DB) UpdateBulkExecJobStatus(ctx context.Context, id, status string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE bulk_exec_jobs SET status = ? WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("updating bulk exec job status: %w", err)
	}
	return nil
}

// UpdateBulkExecJobResults updates the results, counts, and optionally completes the job.
func (d *DB) UpdateBulkExecJobResults(ctx context.Context, id string, completedCount, failedCount int, results string, completed bool) error {
	if completed {
		status := "completed"
		if failedCount > 0 && completedCount == 0 {
			status = "failed"
		}
		_, err := d.conn.ExecContext(ctx, `
			UPDATE bulk_exec_jobs
			SET completed_count = ?, failed_count = ?, results = ?, status = ?, completed_at = ?
			WHERE id = ?`,
			completedCount, failedCount, results, status, Now(), id,
		)
		if err != nil {
			return fmt.Errorf("completing bulk exec job: %w", err)
		}
		return nil
	}

	_, err := d.conn.ExecContext(ctx, `
		UPDATE bulk_exec_jobs
		SET completed_count = ?, failed_count = ?, results = ?
		WHERE id = ?`,
		completedCount, failedCount, results, id,
	)
	if err != nil {
		return fmt.Errorf("updating bulk exec job results: %w", err)
	}
	return nil
}

// CancelBulkExecJob sets a job to cancelled status.
func (d *DB) CancelBulkExecJob(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE bulk_exec_jobs SET status = 'cancelled', completed_at = ? WHERE id = ? AND status IN ('pending', 'running')`,
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("cancelling bulk exec job: %w", err)
	}
	return nil
}
