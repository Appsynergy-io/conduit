package db

import (
	"context"
	"database/sql"
	"fmt"
)

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
