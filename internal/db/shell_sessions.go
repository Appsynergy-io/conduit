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
	ID          string  `json:"id"`
	TenantID    string  `json:"tenantId"`
	AgentID     string  `json:"agentId"`
	UserID      string  `json:"userId"`
	Status      string  `json:"status"`      // "active", "detached", or "closed"
	Shell       *string `json:"shell"`       // e.g. "/bin/bash"
	Cols        *int    `json:"cols"`
	Rows        *int    `json:"rows"`
	Recording   int     `json:"recording"`   // 1 = recording enabled
	Pinned      int     `json:"pinned"`      // 1 = never idle-timeout (NIST AC-12)
	IdleTimeout int     `json:"idleTimeout"` // seconds of detached inactivity before close
	CreatedAt   string  `json:"createdAt"`
	DetachedAt  *string `json:"detachedAt"`
	ClosedAt    *string `json:"closedAt"`
}

// CreateShellSession inserts a new shell session record.
func (d *DB) CreateShellSession(ctx context.Context, s *ShellSession) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO shell_sessions (id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, pinned, idle_timeout, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, d.tenantID, s.AgentID, s.UserID, s.Status, s.Shell, s.Cols, s.Rows, s.Recording, s.Pinned, s.IdleTimeout, Now(),
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
		SELECT id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, pinned, idle_timeout, created_at, detached_at, closed_at
		FROM shell_sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.TenantID, &s.AgentID, &s.UserID, &s.Status, &s.Shell, &s.Cols, &s.Rows, &s.Recording, &s.Pinned, &s.IdleTimeout, &s.CreatedAt, &s.DetachedAt, &s.ClosedAt)
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

// DetachShellSession marks a session as detached with current timestamp.
func (d *DB) DetachShellSession(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE shell_sessions SET status = 'detached', detached_at = ? WHERE id = ?`,
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("detaching shell session: %w", err)
	}
	return nil
}

// UpdateShellSessionStatus sets the session status and clears detached_at when active.
func (d *DB) UpdateShellSessionStatus(ctx context.Context, id, status string) error {
	if status == "active" {
		_, err := d.conn.ExecContext(ctx, `
			UPDATE shell_sessions SET status = 'active', detached_at = NULL WHERE id = ?`, id)
		return err
	}
	_, err := d.conn.ExecContext(ctx, `
		UPDATE shell_sessions SET status = ? WHERE id = ?`, status, id)
	return err
}

// UpdateShellSessionSettings updates the pinned and idle_timeout fields.
func (d *DB) UpdateShellSessionSettings(ctx context.Context, id string, pinned, idleTimeout int) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE shell_sessions SET pinned = ?, idle_timeout = ? WHERE id = ?`,
		pinned, idleTimeout, id,
	)
	if err != nil {
		return fmt.Errorf("updating shell session settings: %w", err)
	}
	return nil
}

// ListShellSessions returns shell sessions filtered by status.
// If status is empty, returns active + detached sessions.
// If agentID is non-empty, filters to that agent.
// If userID is non-empty, filters to that user.
func (d *DB) ListShellSessions(ctx context.Context, tenantID, status, agentID, userID string, pinnedOnly *bool) ([]ShellSession, error) {
	query := `
		SELECT id, tenant_id, agent_id, user_id, status, shell, cols, rows, recording, pinned, idle_timeout, created_at, detached_at, closed_at
		FROM shell_sessions WHERE tenant_id = ?`
	args := []interface{}{tenantID}

	switch status {
	case "active":
		query += ` AND status = 'active'`
	case "detached":
		query += ` AND status = 'detached'`
	case "closed":
		query += ` AND status = 'closed'`
	case "all":
		// no filter
	default:
		// Default: active + detached
		query += ` AND status IN ('active', 'detached')`
	}

	if agentID != "" {
		query += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	if pinnedOnly != nil {
		if *pinnedOnly {
			query += ` AND pinned = 1`
		} else {
			query += ` AND pinned = 0`
		}
	}

	query += ` ORDER BY created_at DESC`

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing shell sessions: %w", err)
	}
	defer rows.Close()

	var sessions []ShellSession
	for rows.Next() {
		var s ShellSession
		if err := rows.Scan(&s.ID, &s.TenantID, &s.AgentID, &s.UserID, &s.Status, &s.Shell, &s.Cols, &s.Rows, &s.Recording, &s.Pinned, &s.IdleTimeout, &s.CreatedAt, &s.DetachedAt, &s.ClosedAt); err != nil {
			return nil, fmt.Errorf("scanning shell session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// ListActiveShellSessions returns all active shell sessions for a tenant.
func (d *DB) ListActiveShellSessions(ctx context.Context, tenantID string) ([]ShellSession, error) {
	return d.ListShellSessions(ctx, tenantID, "active", "", "", nil)
}

// CloseOrphanedShellSessions closes all sessions that are still active or detached.
// Called at server startup to clean up sessions from previous runs whose PTYs are gone.
func (d *DB) CloseOrphanedShellSessions(ctx context.Context) (int64, error) {
	res, err := d.conn.ExecContext(ctx, `
		UPDATE shell_sessions SET status = 'closed', closed_at = ?
		WHERE status IN ('active', 'detached')`, Now())
	if err != nil {
		return 0, fmt.Errorf("closing orphaned shell sessions: %w", err)
	}
	return res.RowsAffected()
}
