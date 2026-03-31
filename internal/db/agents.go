package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Agent represents a row from the agents table.
type Agent struct {
	ID           string
	TenantID     string
	Hostname     string
	DisplayName  *string
	OS           *string
	Arch         *string
	Labels       *string // JSON string
	IP           *string
	AgentKeyHash string
	Status       string
	Transport    *string
	Version      *string
	LastSeenAt   *string
	ConnectedAt  *string
	CreatedAt    string
}

// CreateAgent inserts a new agent record.
func (d *DB) CreateAgent(ctx context.Context, agent *Agent) error {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO agents (
			id, tenant_id, hostname, display_name, os, arch, labels, ip,
			agent_key_hash, status, transport, version, last_seen_at, connected_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ID, agent.TenantID, agent.Hostname, agent.DisplayName,
		agent.OS, agent.Arch, agent.Labels, agent.IP,
		agent.AgentKeyHash, agent.Status, agent.Transport, agent.Version,
		agent.LastSeenAt, agent.ConnectedAt, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting agent: %w", err)
	}
	return nil
}

// GetAgentByID returns a single agent by its ID.
func (d *DB) GetAgentByID(ctx context.Context, id string) (*Agent, error) {
	var a Agent
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, hostname, display_name, os, arch, labels, ip,
		       agent_key_hash, status, transport, version, last_seen_at, connected_at, created_at
		FROM agents
		WHERE id = ?`, id,
	).Scan(
		&a.ID, &a.TenantID, &a.Hostname, &a.DisplayName,
		&a.OS, &a.Arch, &a.Labels, &a.IP,
		&a.AgentKeyHash, &a.Status, &a.Transport, &a.Version,
		&a.LastSeenAt, &a.ConnectedAt, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent by id: %w", err)
	}
	return &a, nil
}

// ListAgents returns all agents for the given tenant, ordered by hostname.
func (d *DB) ListAgents(ctx context.Context, tenantID string) ([]Agent, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, hostname, display_name, os, arch, labels, ip,
		       agent_key_hash, status, transport, version, last_seen_at, connected_at, created_at
		FROM agents
		WHERE tenant_id = ?
		ORDER BY hostname`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Hostname, &a.DisplayName,
			&a.OS, &a.Arch, &a.Labels, &a.IP,
			&a.AgentKeyHash, &a.Status, &a.Transport, &a.Version,
			&a.LastSeenAt, &a.ConnectedAt, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// UpdateAgentStatus sets the agent's status and updates last_seen_at.
func (d *DB) UpdateAgentStatus(ctx context.Context, id, status string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE agents SET status = ?, last_seen_at = ? WHERE id = ?`,
		status, Now(), id,
	)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}
	return nil
}

// UpdateAgentConnection updates status, transport, ip, connected_at, and last_seen_at.
func (d *DB) UpdateAgentConnection(ctx context.Context, id, status, transport, ip string) error {
	now := Now()
	_, err := d.conn.ExecContext(ctx, `
		UPDATE agents SET status = ?, transport = ?, ip = ?, connected_at = ?, last_seen_at = ?
		WHERE id = ?`,
		status, transport, ip, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("updating agent connection: %w", err)
	}
	return nil
}

// UpdateAgentInfo updates hostname, os, arch, and version for an agent.
func (d *DB) UpdateAgentInfo(ctx context.Context, id, hostname, os, arch, version string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE agents SET hostname = ?, os = ?, arch = ?, version = ? WHERE id = ?`,
		hostname, os, arch, version, id,
	)
	if err != nil {
		return fmt.Errorf("updating agent info: %w", err)
	}
	return nil
}

// UpdateAgentLabels updates the labels JSON string for an agent.
func (d *DB) UpdateAgentLabels(ctx context.Context, id, labels string) error {
	_, err := d.conn.ExecContext(ctx, `
		UPDATE agents SET labels = ? WHERE id = ?`,
		labels, id,
	)
	if err != nil {
		return fmt.Errorf("updating agent labels: %w", err)
	}
	return nil
}

// DeleteAgent removes an agent record by ID.
func (d *DB) DeleteAgent(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx, `
		DELETE FROM agents WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	return nil
}

// AgentListParams holds filters for paginated agent listing.
type AgentListParams struct {
	TenantID  string
	CursorAt  string
	CursorID  string
	Limit     int
	Status    string
	Transport string
	OS        string
	Search    string
}

// ListAgentsPaginated returns agents with cursor-based pagination and filters.
func (d *DB) ListAgentsPaginated(ctx context.Context, p AgentListParams) ([]Agent, error) {
	query := `SELECT id, tenant_id, hostname, display_name, os, arch, labels, ip,
	           agent_key_hash, status, transport, version, last_seen_at, connected_at, created_at
	           FROM agents WHERE tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.Status != "" {
		query += ` AND status = ?`
		args = append(args, p.Status)
	}
	if p.Transport != "" && p.Transport != "all" {
		query += ` AND transport = ?`
		args = append(args, p.Transport)
	}
	if p.OS != "" && p.OS != "all" {
		query += ` AND os = ?`
		args = append(args, p.OS)
	}
	if p.Search != "" {
		query += ` AND (hostname LIKE ? OR ip LIKE ?)`
		term := "%" + p.Search + "%"
		args = append(args, term, term)
	}
	if p.CursorAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing agents paginated: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Hostname, &a.DisplayName,
			&a.OS, &a.Arch, &a.Labels, &a.IP,
			&a.AgentKeyHash, &a.Status, &a.Transport, &a.Version,
			&a.LastSeenAt, &a.ConnectedAt, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// CountAgentsByStatus returns the number of agents with the given status for a tenant.
func (d *DB) CountAgentsByStatus(ctx context.Context, tenantID, status string) (int, error) {
	var count int
	err := d.conn.QueryRowContext(ctx, `
		SELECT COUNT(id) FROM agents WHERE tenant_id = ? AND status = ?`,
		tenantID, status,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting agents by status: %w", err)
	}
	return count, nil
}
