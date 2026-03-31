package db

import (
	"context"
	"fmt"
)

// AuditEntry represents a row in the audit_log table.
type AuditEntry struct {
	ID               string
	TenantID         string
	EventType        string
	UserID           *string
	UserEmail        *string
	AgentID          *string
	AgentHostname    *string
	SourceIP         *string
	UserAgent        *string
	Details          *string // JSON
	Outcome          string  // "success" or "failure"
	AlgorithmUsed    *string
	AlgorithmWarning *string
	Timestamp        string
}

// InsertAuditLog inserts an append-only audit log entry.
// If entry.Timestamp is empty, it is set to Now().
func (d *DB) InsertAuditLog(ctx context.Context, entry *AuditEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = Now()
	}

	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO audit_log (
			id, tenant_id, event_type, user_id, user_email,
			agent_id, agent_hostname, source_ip, user_agent,
			details, outcome, algorithm_used, algorithm_warning, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.TenantID,
		entry.EventType,
		entry.UserID,
		entry.UserEmail,
		entry.AgentID,
		entry.AgentHostname,
		entry.SourceIP,
		entry.UserAgent,
		entry.Details,
		entry.Outcome,
		entry.AlgorithmUsed,
		entry.AlgorithmWarning,
		entry.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("inserting audit log: %w", err)
	}
	return nil
}

// ListAuditLogs returns audit log entries for a tenant, ordered by timestamp DESC.
func (d *DB) ListAuditLogs(ctx context.Context, tenantID string, limit, offset int) ([]AuditEntry, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, event_type, user_id, user_email,
		       agent_id, agent_hostname, source_ip, user_agent,
		       details, outcome, algorithm_used, algorithm_warning, timestamp
		FROM audit_log
		WHERE tenant_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// ListAuditLogsByEventType returns audit log entries filtered by event type.
func (d *DB) ListAuditLogsByEventType(ctx context.Context, tenantID, eventType string, limit, offset int) ([]AuditEntry, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, event_type, user_id, user_email,
		       agent_id, agent_hostname, source_ip, user_agent,
		       details, outcome, algorithm_used, algorithm_warning, timestamp
		FROM audit_log
		WHERE tenant_id = ? AND event_type = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		tenantID, eventType, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs by event type: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// ListAuditLogsByUser returns audit log entries filtered by user_id.
func (d *DB) ListAuditLogsByUser(ctx context.Context, tenantID, userID string, limit, offset int) ([]AuditEntry, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, event_type, user_id, user_email,
		       agent_id, agent_hostname, source_ip, user_agent,
		       details, outcome, algorithm_used, algorithm_warning, timestamp
		FROM audit_log
		WHERE tenant_id = ? AND user_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`,
		tenantID, userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs by user: %w", err)
	}
	defer rows.Close()

	return scanAuditEntries(rows)
}

// CountAuditLogs returns the total number of audit log entries for a tenant.
func (d *DB) CountAuditLogs(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := d.conn.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_log WHERE tenant_id = ?",
		tenantID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting audit logs: %w", err)
	}
	return count, nil
}

// scanAuditEntries scans rows into a slice of AuditEntry.
func scanAuditEntries(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]AuditEntry, error) {
	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID,
			&e.TenantID,
			&e.EventType,
			&e.UserID,
			&e.UserEmail,
			&e.AgentID,
			&e.AgentHostname,
			&e.SourceIP,
			&e.UserAgent,
			&e.Details,
			&e.Outcome,
			&e.AlgorithmUsed,
			&e.AlgorithmWarning,
			&e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
