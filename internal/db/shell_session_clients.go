package db

import (
	"context"
	"fmt"
)

// ShellSessionClient represents a row from the shell_session_clients table.
type ShellSessionClient struct {
	ID             string
	TenantID       string
	SessionID      string
	UserID         string
	Role           string
	ConnectedAt    string
	DisconnectedAt *string
}

// InsertShellSessionClient records a client connection for audit.
func (d *DB) InsertShellSessionClient(ctx context.Context, c *ShellSessionClient) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO shell_session_clients (id, tenant_id, session_id, user_id, role, connected_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.TenantID, c.SessionID, c.UserID, c.Role, c.ConnectedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting shell session client: %w", err)
	}
	return nil
}

// DisconnectShellSessionClient sets the disconnected_at timestamp.
func (d *DB) DisconnectShellSessionClient(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE shell_session_clients SET disconnected_at = ? WHERE id = ?`,
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("disconnecting shell session client: %w", err)
	}
	return nil
}
