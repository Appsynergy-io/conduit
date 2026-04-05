package db

import (
	"context"
	"database/sql"
	"fmt"
)

// ServerKeyJWTEd25519 is the key_type used for the Ed25519 JWT signing key.
const ServerKeyJWTEd25519 = "jwt_ed25519"

// GetServerKey returns the persisted private key bytes for the given key type,
// or (nil, nil) if no key exists yet.
func (d *DB) GetServerKey(ctx context.Context, keyType string) ([]byte, error) {
	var key []byte
	err := d.conn.QueryRowContext(ctx,
		`SELECT private_key FROM server_keys WHERE key_type = ?`, keyType,
	).Scan(&key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying server key %s: %w", keyType, err)
	}
	return key, nil
}

// SaveServerKey persists the private key bytes for the given key type.
// Returns an error if a key already exists for that type.
func (d *DB) SaveServerKey(ctx context.Context, keyType string, privateKey []byte) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO server_keys (key_type, private_key, created_at) VALUES (?, ?, ?)`,
		keyType, privateKey, Now(),
	)
	if err != nil {
		return fmt.Errorf("saving server key %s: %w", keyType, err)
	}
	return nil
}
