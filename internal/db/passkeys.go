package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Passkey represents a row from the passkeys table.
type Passkey struct {
	ID                string
	TenantID          string
	UserID            string
	CredentialID      []byte
	PublicKey         []byte
	Algorithm         string
	AlgorithmWarning  *string
	AuthenticatorType string
	SignCount         uint32
	DisplayName       *string
	CreatedAt         string
	LastUsedAt        *string
}

// CreatePasskey inserts a new passkey credential.
func (d *DB) CreatePasskey(ctx context.Context, p *Passkey) error {
	if p.CreatedAt == "" {
		p.CreatedAt = Now()
	}
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO passkeys (
			id, tenant_id, user_id, credential_id, public_key,
			algorithm, algorithm_warning, authenticator_type,
			sign_count, display_name, created_at, last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.TenantID, p.UserID, p.CredentialID, p.PublicKey,
		p.Algorithm, p.AlgorithmWarning, p.AuthenticatorType,
		p.SignCount, p.DisplayName, p.CreatedAt, p.LastUsedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting passkey: %w", err)
	}
	return nil
}

// GetPasskeysByUserID returns all passkeys for a user.
func (d *DB) GetPasskeysByUserID(ctx context.Context, userID string) ([]Passkey, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, tenant_id, user_id, credential_id, public_key,
		       algorithm, algorithm_warning, authenticator_type,
		       sign_count, display_name, created_at, last_used_at
		FROM passkeys WHERE user_id = ?
		ORDER BY created_at`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing passkeys by user: %w", err)
	}
	defer rows.Close()

	var passkeys []Passkey
	for rows.Next() {
		var p Passkey
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.UserID, &p.CredentialID, &p.PublicKey,
			&p.Algorithm, &p.AlgorithmWarning, &p.AuthenticatorType,
			&p.SignCount, &p.DisplayName, &p.CreatedAt, &p.LastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning passkey: %w", err)
		}
		passkeys = append(passkeys, p)
	}
	return passkeys, rows.Err()
}

// GetPasskeyByCredentialID looks up a passkey by its WebAuthn credential ID.
func (d *DB) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*Passkey, error) {
	p := &Passkey{}
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, credential_id, public_key,
		       algorithm, algorithm_warning, authenticator_type,
		       sign_count, display_name, created_at, last_used_at
		FROM passkeys WHERE credential_id = ?`, credentialID,
	).Scan(
		&p.ID, &p.TenantID, &p.UserID, &p.CredentialID, &p.PublicKey,
		&p.Algorithm, &p.AlgorithmWarning, &p.AuthenticatorType,
		&p.SignCount, &p.DisplayName, &p.CreatedAt, &p.LastUsedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying passkey by credential id: %w", err)
	}
	return p, nil
}

// UpdatePasskeySignCount updates the sign count after a successful assertion.
func (d *DB) UpdatePasskeySignCount(ctx context.Context, id string, signCount uint32) error {
	now := Now()
	result, err := d.conn.ExecContext(ctx, `
		UPDATE passkeys SET sign_count = ?, last_used_at = ? WHERE id = ?`,
		signCount, now, id,
	)
	if err != nil {
		return fmt.Errorf("updating passkey sign count: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("updating passkey sign count: %w", sql.ErrNoRows)
	}
	return nil
}

// DeletePasskey removes a passkey by ID.
func (d *DB) DeletePasskey(ctx context.Context, id string) error {
	result, err := d.conn.ExecContext(ctx, `DELETE FROM passkeys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting passkey: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("deleting passkey: %w", sql.ErrNoRows)
	}
	return nil
}

// GetPasskeyByID returns a single passkey by primary key.
func (d *DB) GetPasskeyByID(ctx context.Context, id string) (*Passkey, error) {
	p := &Passkey{}
	err := d.conn.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, credential_id, public_key,
		       algorithm, algorithm_warning, authenticator_type,
		       sign_count, display_name, created_at, last_used_at
		FROM passkeys WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.TenantID, &p.UserID, &p.CredentialID, &p.PublicKey,
		&p.Algorithm, &p.AlgorithmWarning, &p.AuthenticatorType,
		&p.SignCount, &p.DisplayName, &p.CreatedAt, &p.LastUsedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying passkey by id: %w", err)
	}
	return p, nil
}
