package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ShellRecording represents a row from the shell_recordings table.
type ShellRecording struct {
	ID            string
	TenantID      string
	SessionID     string
	AgentID       string
	UserID        string
	AgentHostname string
	UserEmail     string
	Duration      int    // seconds
	SizeBytes     int
	Format        string // "asciicast-v2"
	Data          []byte // the actual recording data
	CreatedAt     string
}

// CreateShellRecording inserts a new shell recording record.
func (d *DB) CreateShellRecording(ctx context.Context, rec *ShellRecording) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO shell_recordings (id, tenant_id, session_id, agent_id, user_id, agent_hostname, user_email, duration, size_bytes, format, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, d.tenantID, rec.SessionID, rec.AgentID, rec.UserID, rec.AgentHostname, rec.UserEmail, rec.Duration, rec.SizeBytes, rec.Format, rec.Data, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting shell recording: %w", err)
	}
	return nil
}

// GetShellRecording returns a shell recording by ID, including the data.
func (d *DB) GetShellRecording(ctx context.Context, id string) (*ShellRecording, error) {
	var rec ShellRecording
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, session_id, agent_id, user_id, agent_hostname, user_email, duration, size_bytes, format, data, created_at
		FROM shell_recordings WHERE id = ? AND tenant_id = ?`, id, d.tenantID,
	).Scan(&rec.ID, &rec.TenantID, &rec.SessionID, &rec.AgentID, &rec.UserID, &rec.AgentHostname, &rec.UserEmail, &rec.Duration, &rec.SizeBytes, &rec.Format, &rec.Data, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying shell recording: %w", err)
	}
	return &rec, nil
}

// ListShellRecordings returns paginated shell recordings for the tenant.
// If agentID is non-empty, results are filtered to that agent.
// The data field is not populated in list results.
// Returns the recordings and total count.
func (d *DB) ListShellRecordings(ctx context.Context, agentID string, limit, offset int) ([]ShellRecording, int, error) {
	// Count query
	countQuery := `SELECT COUNT(*) FROM shell_recordings WHERE tenant_id = ?`
	countArgs := []interface{}{d.tenantID}
	if agentID != "" {
		countQuery += ` AND agent_id = ?`
		countArgs = append(countArgs, agentID)
	}

	var total int
	if err := d.conn.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting shell recordings: %w", err)
	}

	// List query (no data field)
	listQuery := `
		SELECT id, tenant_id, session_id, agent_id, user_id, agent_hostname, user_email, duration, size_bytes, format, created_at
		FROM shell_recordings WHERE tenant_id = ?`
	listArgs := []interface{}{d.tenantID}
	if agentID != "" {
		listQuery += ` AND agent_id = ?`
		listArgs = append(listArgs, agentID)
	}
	listQuery += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	listArgs = append(listArgs, limit, offset)

	rows, err := d.conn.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing shell recordings: %w", err)
	}
	defer rows.Close()

	var recordings []ShellRecording
	for rows.Next() {
		var rec ShellRecording
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.SessionID, &rec.AgentID, &rec.UserID, &rec.AgentHostname, &rec.UserEmail, &rec.Duration, &rec.SizeBytes, &rec.Format, &rec.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning shell recording: %w", err)
		}
		recordings = append(recordings, rec)
	}
	return recordings, total, rows.Err()
}

// ShellSession represents a row from the shell_sessions table.
type ShellSession struct {
	ID        string
	TenantID  string
	AgentID   string
	UserID    string
	Status    string  // "active" or "closed"
	Shell     *string // e.g. "/bin/bash"
	Cols      *int
	Rows      *int
	Recording int // 1 = recording enabled
	CreatedAt string
	ClosedAt  *string
}

// CreateShellSession inserts a new shell session record.
func (d *DB) CreateShellSession(ctx context.Context, s *ShellSession) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO shell_sessions (id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, d.tenantID, s.AgentID, s.UserID, s.Status, s.Shell, s.Cols, s.Rows, s.Recording, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting shell session: %w", err)
	}
	return nil
}

// GetShellSessionByID returns a shell session by ID.
func (d *DB) GetShellSessionByID(ctx context.Context, id string) (*ShellSession, error) {
	var s ShellSession
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, created_at, closed_at
		FROM shell_sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.TenantID, &s.AgentID, &s.UserID, &s.Status, &s.Shell, &s.Cols, &s.Rows, &s.Recording, &s.CreatedAt, &s.ClosedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying shell session: %w", err)
	}
	return &s, nil
}

// CloseShellSession marks a shell session as closed.
func (d *DB) CloseShellSession(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE shell_sessions SET status = 'closed', closed_at = ? WHERE id = ?`,
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("closing shell session: %w", err)
	}
	return nil
}

// ListActiveShellSessions returns all active shell sessions for a tenant.
func (d *DB) ListActiveShellSessions(ctx context.Context, tenantID string) ([]ShellSession, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, created_at, closed_at
		FROM shell_sessions WHERE tenant_id = ? AND status = 'active'
		ORDER BY created_at DESC`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing active shell sessions: %w", err)
	}
	defer rows.Close()

	var sessions []ShellSession
	for rows.Next() {
		var s ShellSession
		if err := rows.Scan(&s.ID, &s.TenantID, &s.AgentID, &s.UserID, &s.Status, &s.Shell, &s.Cols, &s.Rows, &s.Recording, &s.CreatedAt, &s.ClosedAt); err != nil {
			return nil, fmt.Errorf("scanning shell session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
