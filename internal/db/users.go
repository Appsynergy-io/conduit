package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// User represents a row from the users table.
type User struct {
	ID           string
	TenantID     string
	Email        string
	FirstName    string
	LastName     string
	DisplayName  *string
	Role         string
	Status       string
	PasswordHash *string
	CreatedBy    *string
	LastLoginAt  *string
	CreatedAt    string
	UpdatedAt    string
}

// CreateUser inserts a new user. It sets created_at and updated_at to Now().
func (d *DB) CreateUser(ctx context.Context, user *User) error {
	now := Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO users (
			id, tenant_id, email, first_name, last_name, display_name,
			role, status, password_hash, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.TenantID, user.Email, user.FirstName, user.LastName,
		user.DisplayName, user.Role, user.Status, user.PasswordHash,
		user.CreatedBy, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}
	return nil
}

// GetUserByID returns a user by primary key.
func (d *DB) GetUserByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, first_name, last_name, display_name,
		       role, status, password_hash, created_by, last_login_at,
		       created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(
		&u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName,
		&u.DisplayName, &u.Role, &u.Status, &u.PasswordHash,
		&u.CreatedBy, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}
	return u, nil
}

// GetUserByEmail returns a user by email address.
func (d *DB) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, first_name, last_name, display_name,
		       role, status, password_hash, created_by, last_login_at,
		       created_at, updated_at
		FROM users WHERE email = ?`, email,
	).Scan(
		&u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName,
		&u.DisplayName, &u.Role, &u.Status, &u.PasswordHash,
		&u.CreatedBy, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}
	return u, nil
}

// ListUsers returns all users for a tenant, ordered by email.
func (d *DB) ListUsers(ctx context.Context, tenantID string) ([]User, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, email, first_name, last_name, display_name,
		       role, status, password_hash, created_by, last_login_at,
		       created_at, updated_at
		FROM users WHERE tenant_id = ?
		ORDER BY email`, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName,
			&u.DisplayName, &u.Role, &u.Status, &u.PasswordHash,
			&u.CreatedBy, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser updates a user's mutable fields: name, display_name, role, status,
// and sets updated_at to Now(). Looks up by user.ID.
func (d *DB) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = Now()
	result, err := d.conn.ExecContext(ctx, `
		UPDATE users
		SET first_name = ?, last_name = ?, display_name = ?,
		    role = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		user.FirstName, user.LastName, user.DisplayName,
		user.Role, user.Status, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("updating user: %w", sql.ErrNoRows)
	}
	return nil
}

// UpdateUserLastLogin sets last_login_at to Now() for the given user ID.
func (d *DB) UpdateUserLastLogin(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx, `
		UPDATE users SET last_login_at = ? WHERE id = ?`,
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("updating last login: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("updating last login: %w", sql.ErrNoRows)
	}
	return nil
}

// SetPasswordHash sets the password_hash for the given user ID.
func (d *DB) SetPasswordHash(ctx context.Context, id, hash string) error {
	result, err := d.conn.ExecContext(ctx, `
		UPDATE users SET password_hash = ? WHERE id = ?`,
		hash, id,
	)
	if err != nil {
		return fmt.Errorf("setting password hash: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("setting password hash: %w", sql.ErrNoRows)
	}
	return nil
}

// DeletePasswordHash sets password_hash to NULL for the given user ID.
func (d *DB) DeletePasswordHash(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx, `
		UPDATE users SET password_hash = NULL WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting password hash: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("deleting password hash: %w", sql.ErrNoRows)
	}
	return nil
}

// CountUsers returns the number of users for the given tenant.
func (d *DB) CountUsers(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := d.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users WHERE tenant_id = ?`,
		tenantID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// UserListParams holds filters for paginated user listing.
type UserListParams struct {
	TenantID string
	CursorAt string
	CursorID string
	Limit    int
	Status   string
	Role     string
	Search   string
}

// ListUsersPaginated returns users with cursor-based pagination and optional filters.
// Returns limit+1 items so the caller can detect hasMore.
func (d *DB) ListUsersPaginated(ctx context.Context, p UserListParams) ([]User, error) {
	query := `SELECT id, tenant_id, email, first_name, last_name, display_name,
	           role, status, password_hash, created_by, last_login_at, created_at, updated_at
	           FROM users WHERE tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.Status != "" && p.Status != "all" {
		query += ` AND status = ?`
		args = append(args, p.Status)
	}
	if p.Role != "" {
		query += ` AND role = ?`
		args = append(args, p.Role)
	}
	if p.Search != "" {
		query += ` AND (email LIKE ? OR first_name LIKE ? OR last_name LIKE ?)`
		term := "%" + p.Search + "%"
		args = append(args, term, term, term)
	}
	if p.CursorAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing users paginated: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName,
			&u.DisplayName, &u.Role, &u.Status, &u.PasswordHash,
			&u.CreatedBy, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// CountPasskeysByUsers returns passkey counts keyed by user ID.
func (d *DB) CountPasskeysByUsers(ctx context.Context, userIDs []string) (map[string]int, error) {
	result := make(map[string]int, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	placeholders := strings.Repeat("?,", len(userIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}

	rows, err := d.conn.QueryContext(ctx,
		`SELECT user_id, COUNT(*) FROM passkeys WHERE user_id IN (`+placeholders+`) GROUP BY user_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("counting passkeys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var uid string
		var count int
		if err := rows.Scan(&uid, &count); err != nil {
			return nil, fmt.Errorf("scanning passkey count: %w", err)
		}
		result[uid] = count
	}
	return result, rows.Err()
}

// GetGroupIDsByUsers returns group IDs keyed by user ID.
func (d *DB) GetGroupIDsByUsers(ctx context.Context, userIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	placeholders := strings.Repeat("?,", len(userIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}

	rows, err := d.conn.QueryContext(ctx,
		`SELECT user_id, group_id FROM group_members WHERE user_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("getting group ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var uid, gid string
		if err := rows.Scan(&uid, &gid); err != nil {
			return nil, fmt.Errorf("scanning group member: %w", err)
		}
		result[uid] = append(result[uid], gid)
	}
	return result, rows.Err()
}
