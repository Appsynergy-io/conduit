package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// JoinToken represents a row from the join_tokens table.
type JoinToken struct {
	ID        string
	TenantID  string
	Type      string  // "single_use" or "persistent"
	Name      string
	Labels    *string // JSON, nullable
	TokenHash string
	UsedCount int
	MaxUses   *int    // nullable (NULL = unlimited for persistent)
	ExpiresAt *string // nullable
	Revoked   int
	RevokedAt *string // nullable
	CreatedBy *string // nullable
	CreatedAt string
}

// CreateJoinToken inserts a new join token.
func (d *DB) CreateJoinToken(ctx context.Context, token *JoinToken) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO join_tokens (id, tenant_id, type, name, labels, token_hash, used_count, max_uses, expires_at, revoked, revoked_at, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID,
		token.TenantID,
		token.Type,
		token.Name,
		token.Labels,
		token.TokenHash,
		token.UsedCount,
		token.MaxUses,
		token.ExpiresAt,
		token.Revoked,
		token.RevokedAt,
		token.CreatedBy,
		Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting join token: %w", err)
	}
	return nil
}

// GetJoinTokenByID returns a join token by its primary key.
func (d *DB) GetJoinTokenByID(ctx context.Context, id string) (*JoinToken, error) {
	t := &JoinToken{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, name, labels, token_hash, used_count, max_uses, expires_at, revoked, revoked_at, created_by, created_at
		 FROM join_tokens WHERE id = ?`,
		id,
	).Scan(
		&t.ID, &t.TenantID, &t.Type, &t.Name, &t.Labels, &t.TokenHash,
		&t.UsedCount, &t.MaxUses, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
		&t.CreatedBy, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying join token by id: %w", err)
	}
	return t, nil
}

// GetJoinTokenByHash returns a join token by its token_hash.
func (d *DB) GetJoinTokenByHash(ctx context.Context, hash string) (*JoinToken, error) {
	t := &JoinToken{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, name, labels, token_hash, used_count, max_uses, expires_at, revoked, revoked_at, created_by, created_at
		 FROM join_tokens WHERE token_hash = ?`,
		hash,
	).Scan(
		&t.ID, &t.TenantID, &t.Type, &t.Name, &t.Labels, &t.TokenHash,
		&t.UsedCount, &t.MaxUses, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
		&t.CreatedBy, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying join token by hash: %w", err)
	}
	return t, nil
}

// ListJoinTokens returns all join tokens for the given tenant, ordered by created_at DESC.
func (d *DB) ListJoinTokens(ctx context.Context, tenantID string) ([]JoinToken, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, tenant_id, type, name, labels, token_hash, used_count, max_uses, expires_at, revoked, revoked_at, created_by, created_at
		 FROM join_tokens WHERE tenant_id = ?
		 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing join tokens: %w", err)
	}
	defer rows.Close()

	var tokens []JoinToken
	for rows.Next() {
		var t JoinToken
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Type, &t.Name, &t.Labels, &t.TokenHash,
			&t.UsedCount, &t.MaxUses, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
			&t.CreatedBy, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning join token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// IncrementTokenUsage atomically increments used_count by 1 for the given token.
func (d *DB) IncrementTokenUsage(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		"UPDATE join_tokens SET used_count = used_count + 1 WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("incrementing token usage: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("join token not found: %s", id)
	}
	return nil
}

// RevokeJoinToken marks a join token as revoked.
func (d *DB) RevokeJoinToken(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		"UPDATE join_tokens SET revoked = 1, revoked_at = ? WHERE id = ?",
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("revoking join token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("join token not found: %s", id)
	}
	return nil
}

// TokenListParams holds filters for paginated join token listing.
type TokenListParams struct {
	TenantID  string
	CursorAt  string
	CursorID  string
	Limit     int
	Type      string // "single_use" or "persistent"
	Revoked   string // "true" or "false"
	Search    string
}

// ListJoinTokensPaginated returns join tokens with cursor-based pagination and filters.
func (d *DB) ListJoinTokensPaginated(ctx context.Context, p TokenListParams) ([]JoinToken, error) {
	query := `SELECT id, tenant_id, type, name, labels, token_hash, used_count, max_uses,
	           expires_at, revoked, revoked_at, created_by, created_at
	           FROM join_tokens WHERE tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.Type != "" {
		query += ` AND type = ?`
		args = append(args, p.Type)
	}
	if p.Revoked == "true" {
		query += ` AND revoked = 1`
	} else if p.Revoked == "false" {
		query += ` AND revoked = 0`
	}
	if p.Search != "" {
		query += ` AND name LIKE ?`
		term := "%" + strings.ReplaceAll(p.Search, "%", "") + "%"
		args = append(args, term)
	}
	if p.CursorAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing join tokens paginated: %w", err)
	}
	defer rows.Close()

	var tokens []JoinToken
	for rows.Next() {
		var t JoinToken
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Type, &t.Name, &t.Labels, &t.TokenHash,
			&t.UsedCount, &t.MaxUses, &t.ExpiresAt, &t.Revoked, &t.RevokedAt,
			&t.CreatedBy, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning join token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// DeleteJoinToken permanently removes a join token.
func (d *DB) DeleteJoinToken(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		"DELETE FROM join_tokens WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting join token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("join token not found: %s", id)
	}
	return nil
}
