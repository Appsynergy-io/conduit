package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Group represents a row from the groups table.
type Group struct {
	ID          string
	TenantID    string
	Name        string
	Description *string
	CreatedAt   string
	UpdatedAt   string
}

// CreateGroup inserts a new group.
func (d *DB) CreateGroup(ctx context.Context, group *Group) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO groups (id, tenant_id, name, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		group.ID, group.TenantID, group.Name, group.Description, Now(), Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting group: %w", err)
	}
	return nil
}

// GetGroupByID returns a group by its ID.
func (d *DB) GetGroupByID(ctx context.Context, id string) (*Group, error) {
	var g Group
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, description, created_at, updated_at
		 FROM groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying group by id: %w", err)
	}
	return &g, nil
}

// ListGroups returns all groups for a tenant, ordered by name.
func (d *DB) ListGroups(ctx context.Context, tenantID string) ([]Group, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, name, description, created_at, updated_at
		 FROM groups WHERE tenant_id = ? ORDER BY name`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// UpdateGroup updates a group's name, description, and updated_at.
func (d *DB) UpdateGroup(ctx context.Context, id, name string, description *string) error {
	result, err := d.conn.ExecContext(ctx,
		`UPDATE groups SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		name, description, Now(), id,
	)
	if err != nil {
		return fmt.Errorf("updating group: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteGroup deletes a group by its ID.
func (d *DB) DeleteGroup(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		`DELETE FROM groups WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting group: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AddUserToGroup inserts a user into a group.
func (d *DB) AddUserToGroup(ctx context.Context, tenantID, groupID, userID string) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO group_members (tenant_id, group_id, user_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		tenantID, groupID, userID, Now(),
	)
	if err != nil {
		return fmt.Errorf("adding user to group: %w", err)
	}
	return nil
}

// RemoveUserFromGroup removes a user from a group.
func (d *DB) RemoveUserFromGroup(ctx context.Context, groupID, userID string) error {
	result, err := d.conn.ExecContext(ctx,
		`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`,
		groupID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing user from group: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListGroupMembers returns the user IDs belonging to a group.
func (d *DB) ListGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT user_id FROM group_members WHERE group_id = ?`, groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, rows.Err()
}

// AddAgentToGroup inserts an agent into a group.
func (d *DB) AddAgentToGroup(ctx context.Context, tenantID, groupID, agentID string) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO agent_group_members (tenant_id, group_id, agent_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		tenantID, groupID, agentID, Now(),
	)
	if err != nil {
		return fmt.Errorf("adding agent to group: %w", err)
	}
	return nil
}

// RemoveAgentFromGroup removes an agent from a group.
func (d *DB) RemoveAgentFromGroup(ctx context.Context, groupID, agentID string) error {
	result, err := d.conn.ExecContext(ctx,
		`DELETE FROM agent_group_members WHERE group_id = ? AND agent_id = ?`,
		groupID, agentID,
	)
	if err != nil {
		return fmt.Errorf("removing agent from group: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListAgentGroupMembers returns the agent IDs belonging to a group.
func (d *DB) ListAgentGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT agent_id FROM agent_group_members WHERE group_id = ?`, groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing agent group members: %w", err)
	}
	defer rows.Close()

	var agentIDs []string
	for rows.Next() {
		var aid string
		if err := rows.Scan(&aid); err != nil {
			return nil, fmt.Errorf("scanning agent group member: %w", err)
		}
		agentIDs = append(agentIDs, aid)
	}
	return agentIDs, rows.Err()
}

// ListGroupsForUser returns all groups a user belongs to, ordered by name.
func (d *DB) ListGroupsForUser(ctx context.Context, userID string) ([]Group, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT g.id, g.tenant_id, g.name, g.description, g.created_at, g.updated_at
		 FROM groups g
		 INNER JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = ?
		 ORDER BY g.name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing groups for user: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning group for user: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GroupListParams holds filters for paginated group listing.
type GroupListParams struct {
	TenantID string
	CursorAt string
	CursorID string
	Limit    int
	Search   string
}

// ListGroupsPaginated returns groups with cursor-based pagination and member counts.
func (d *DB) ListGroupsPaginated(ctx context.Context, p GroupListParams) ([]Group, []int, error) {
	query := `SELECT g.id, g.tenant_id, g.name, g.description, g.created_at, g.updated_at,
	           (SELECT COUNT(*) FROM group_members WHERE group_id = g.id) AS member_count
	           FROM groups g WHERE g.tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.Search != "" {
		query += ` AND g.name LIKE ?`
		args = append(args, "%"+p.Search+"%")
	}
	if p.CursorAt != "" {
		query += ` AND (g.created_at < ? OR (g.created_at = ? AND g.id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY g.created_at DESC, g.id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("listing groups paginated: %w", err)
	}
	defer rows.Close()

	var groups []Group
	var counts []int
	for rows.Next() {
		var g Group
		var count int
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt, &count); err != nil {
			return nil, nil, fmt.Errorf("scanning group: %w", err)
		}
		groups = append(groups, g)
		counts = append(counts, count)
	}
	return groups, counts, rows.Err()
}

// CountGroupMembersByID returns the member count for a single group.
func (d *DB) CountGroupMembersByID(ctx context.Context, groupID string) (int, error) {
	var count int
	err := d.conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_members WHERE group_id = ?`, groupID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting group members: %w", err)
	}
	return count, nil
}

// ListGroupsForAgent returns all groups an agent belongs to, ordered by name.
func (d *DB) ListGroupsForAgent(ctx context.Context, agentID string) ([]Group, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT g.id, g.tenant_id, g.name, g.description, g.created_at, g.updated_at
		 FROM groups g
		 INNER JOIN agent_group_members agm ON agm.group_id = g.id
		 WHERE agm.agent_id = ?
		 ORDER BY g.name`, agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing groups for agent: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning group for agent: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}
