package db

import (
	"context"
	"fmt"
)

// RecoveryCode represents a row from the recovery_codes table.
type RecoveryCode struct {
	ID        string
	TenantID  string
	UserID    string
	CodeHash  string
	Used      bool
	UsedAt    *string
	CreatedAt string
}

// CreateRecoveryCodes replaces all existing recovery codes for a user with new ones.
// Deletes existing codes first, then bulk inserts the new hashes in a transaction.
func (d *DB) CreateRecoveryCodes(ctx context.Context, userID string, codes []RecoveryCode) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing codes for this user
	if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("deleting existing recovery codes: %w", err)
	}

	// Insert new codes
	for _, c := range codes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recovery_codes (id, tenant_id, user_id, code_hash, used, created_at)
			VALUES (?, ?, ?, ?, 0, ?)`,
			c.ID, c.TenantID, c.UserID, c.CodeHash, c.CreatedAt,
		); err != nil {
			return fmt.Errorf("inserting recovery code: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing recovery codes: %w", err)
	}
	return nil
}

// GetUnusedRecoveryCodesByUser returns all unused recovery codes for a user.
func (d *DB) GetUnusedRecoveryCodesByUser(ctx context.Context, userID string) ([]RecoveryCode, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, user_id, code_hash, used, used_at, created_at
		FROM recovery_codes
		WHERE user_id = ? AND used = 0
		ORDER BY created_at`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing unused recovery codes: %w", err)
	}
	defer rows.Close()

	var codes []RecoveryCode
	for rows.Next() {
		var c RecoveryCode
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.UserID, &c.CodeHash, &c.Used, &c.UsedAt, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning recovery code: %w", err)
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

// MarkRecoveryCodeUsed sets used=1 and used_at=NOW() for the given code ID.
func (d *DB) MarkRecoveryCodeUsed(ctx context.Context, id string) error {
	now := Now()
	result, err := d.conn.ExecContext(ctx, `
		UPDATE recovery_codes SET used = 1, used_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("marking recovery code used: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("marking recovery code used: no matching code")
	}
	return nil
}

// DeleteRecoveryCodesByUser deletes all recovery codes for a user.
func (d *DB) DeleteRecoveryCodesByUser(ctx context.Context, userID string) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM recovery_codes WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting recovery codes for user: %w", err)
	}
	return nil
}

// CountUnusedRecoveryCodes returns the number of unused recovery codes for a user.
func (d *DB) CountUnusedRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var count int
	err := d.conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recovery_codes WHERE user_id = ? AND used = 0`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unused recovery codes: %w", err)
	}
	return count, nil
}

// DeletePasskeysByUser deletes all passkeys for a user.
func (d *DB) DeletePasskeysByUser(ctx context.Context, userID string) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("deleting passkeys for user: %w", err)
	}
	return nil
}
