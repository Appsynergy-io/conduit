package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CIToken represents a row from the ci_tokens table.
type CIToken struct {
	ID         string
	TenantID   string
	CreatedBy  string
	Name       string
	TokenHash  string
	Scopes     string  // JSON array
	ExpiresAt  *string // nullable
	LastUsedAt *string // nullable
	CreatedAt  string
}

// CreateCIToken inserts a new CI token.
func (d *DB) CreateCIToken(ctx context.Context, token *CIToken) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO ci_tokens (id, tenant_id, created_by, name, token_hash, scopes, expires_at, last_used_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID,
		token.TenantID,
		token.CreatedBy,
		token.Name,
		token.TokenHash,
		token.Scopes,
		token.ExpiresAt,
		token.LastUsedAt,
		Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting ci token: %w", err)
	}
	return nil
}

// GetCITokenByID returns a CI token by its primary key.
func (d *DB) GetCITokenByID(ctx context.Context, id string) (*CIToken, error) {
	t := &CIToken{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, created_by, name, token_hash, scopes, expires_at, last_used_at, created_at
		 FROM ci_tokens WHERE id = ?`,
		id,
	).Scan(
		&t.ID, &t.TenantID, &t.CreatedBy, &t.Name, &t.TokenHash,
		&t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying ci token by id: %w", err)
	}
	return t, nil
}

// GetCITokenByHash returns a CI token by its token_hash.
func (d *DB) GetCITokenByHash(ctx context.Context, hash string) (*CIToken, error) {
	t := &CIToken{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, created_by, name, token_hash, scopes, expires_at, last_used_at, created_at
		 FROM ci_tokens WHERE token_hash = ?`,
		hash,
	).Scan(
		&t.ID, &t.TenantID, &t.CreatedBy, &t.Name, &t.TokenHash,
		&t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying ci token by hash: %w", err)
	}
	return t, nil
}

// UpdateCITokenLastUsed updates the last_used_at timestamp for a CI token.
func (d *DB) UpdateCITokenLastUsed(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx,
		"UPDATE ci_tokens SET last_used_at = ? WHERE id = ?",
		Now(), id,
	)
	if err != nil {
		return fmt.Errorf("updating ci token last_used_at: %w", err)
	}
	return nil
}

// DeleteCIToken permanently removes a CI token.
func (d *DB) DeleteCIToken(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx,
		"DELETE FROM ci_tokens WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting ci token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("ci token not found: %s", id)
	}
	return nil
}

// CITokenListParams holds filters for paginated CI token listing.
type CITokenListParams struct {
	TenantID string
	CursorAt string
	CursorID string
	Limit    int
}

// ListCITokensPaginated returns CI tokens with cursor-based pagination.
func (d *DB) ListCITokensPaginated(ctx context.Context, p CITokenListParams) ([]CIToken, error) {
	query := `SELECT id, tenant_id, created_by, name, token_hash, scopes, expires_at, last_used_at, created_at
	           FROM ci_tokens WHERE tenant_id = ?`
	args := []interface{}{p.TenantID}

	if p.CursorAt != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, p.CursorAt, p.CursorAt, p.CursorID)
	}

	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing ci tokens paginated: %w", err)
	}
	defer rows.Close()

	var tokens []CIToken
	for rows.Next() {
		var t CIToken
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.CreatedBy, &t.Name, &t.TokenHash,
			&t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning ci token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// ValidCITokenScopes defines the allowed scope values for CI tokens.
var ValidCITokenScopes = map[string]bool{
	"agents:read":   true,
	"agents:write":  true,
	"shell:execute": true,
	"shell:watch":   true,
	"shell:control": true,
	"files:read":    true,
	"files:write":   true,
	"exec:run":      true,
	"audit:read":    true,
	"users:read":    true,
	"users:write":   true,
	"config:read":   true,
	"config:write":  true,
}

// ValidateCITokenScopes checks that all scopes are valid and returns an error message if not.
func ValidateCITokenScopes(scopes []string) string {
	if len(scopes) == 0 {
		return "at least one scope is required"
	}
	if len(scopes) > 20 {
		return "maximum 20 scopes allowed"
	}
	seen := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		if !ValidCITokenScopes[s] {
			return fmt.Sprintf("invalid scope: %s", s)
		}
		if seen[s] {
			return fmt.Sprintf("duplicate scope: %s", s)
		}
		seen[s] = true
	}
	return ""
}

// ScopesContains checks if a JSON scopes string contains the given scope.
func ScopesContains(scopesJSON string, scope string) bool {
	// Fast path: avoid full JSON parse
	return strings.Contains(scopesJSON, `"`+scope+`"`)
}
