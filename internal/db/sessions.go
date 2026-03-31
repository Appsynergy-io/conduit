package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Session represents a row from the sessions table.
type Session struct {
	ID               string
	TenantID         string
	UserID           string
	Type             string  // "web", "cli", "ci"
	SourceIP         *string
	UserAgent        *string
	RefreshTokenHash *string
	ExpiresAt        string
	CreatedAt        string
	LastActiveAt     string
}

// CreateSession inserts a new session.
func (d *DB) CreateSession(ctx context.Context, session *Session) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO sessions (id, tenant_id, user_id, type, source_ip, user_agent, refresh_token_hash, expires_at, created_at, last_active_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		d.tenantID,
		session.UserID,
		session.Type,
		session.SourceIP,
		session.UserAgent,
		session.RefreshTokenHash,
		session.ExpiresAt,
		Now(),
		Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// GetSessionByID returns a session by its ID.
func (d *DB) GetSessionByID(ctx context.Context, id string) (*Session, error) {
	var s Session
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, user_id, type, source_ip, user_agent, refresh_token_hash, expires_at, created_at, last_active_at
		 FROM sessions WHERE id = ?`, id,
	).Scan(
		&s.ID,
		&s.TenantID,
		&s.UserID,
		&s.Type,
		&s.SourceIP,
		&s.UserAgent,
		&s.RefreshTokenHash,
		&s.ExpiresAt,
		&s.CreatedAt,
		&s.LastActiveAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying session by id: %w", err)
	}
	return &s, nil
}

// ListSessionsByUser returns all sessions for a user, ordered by most recently active first.
func (d *DB) ListSessionsByUser(ctx context.Context, userID string) ([]Session, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, user_id, type, source_ip, user_agent, refresh_token_hash, expires_at, created_at, last_active_at
		 FROM sessions WHERE user_id = ?
		 ORDER BY last_active_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions by user: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.ID,
			&s.TenantID,
			&s.UserID,
			&s.Type,
			&s.SourceIP,
			&s.UserAgent,
			&s.RefreshTokenHash,
			&s.ExpiresAt,
			&s.CreatedAt,
			&s.LastActiveAt,
		); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// UpdateSessionActivity sets last_active_at to the current time.
func (d *DB) UpdateSessionActivity(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx,
		"UPDATE sessions SET last_active_at = ? WHERE id = ?",
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("updating session activity: %w", err)
	}
	return nil
}

// UpdateRefreshTokenHash updates the refresh_token_hash for a session.
func (d *DB) UpdateRefreshTokenHash(ctx context.Context, id, hash string) error {
	_, err := d.conn.ExecContext(ctx,
		"UPDATE sessions SET refresh_token_hash = ? WHERE id = ?",
		hash, id,
	)
	if err != nil {
		return fmt.Errorf("updating refresh token hash: %w", err)
	}
	return nil
}

// DeleteSession removes a session by ID (revoke).
func (d *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx,
		"DELETE FROM sessions WHERE id = ?", id,
	)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteSessionsByUser removes all sessions for a user (revoke all).
func (d *DB) DeleteSessionsByUser(ctx context.Context, userID string) error {
	_, err := d.conn.ExecContext(ctx,
		"DELETE FROM sessions WHERE user_id = ?", userID,
	)
	if err != nil {
		return fmt.Errorf("deleting sessions by user: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all sessions past their expiry and returns the count removed.
func (d *DB) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := d.conn.ExecContext(ctx,
		"DELETE FROM sessions WHERE expires_at < ?", Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}
	return affected, nil
}
