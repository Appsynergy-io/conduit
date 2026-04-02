package db

import (
	"context"
	"database/sql"
	"fmt"
)

// DeviceCode represents a row from the device_codes table.
type DeviceCode struct {
	ID          string
	TenantID    string
	DeviceCode  string
	UserCode    string
	ClientID    string
	ProfileName string
	Authorized  bool
	UserID      *string
	ExpiresAt   string
	CreatedAt   string
}

// CreateDeviceCode inserts a new device authorization code.
func (d *DB) CreateDeviceCode(ctx context.Context, dc *DeviceCode) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO device_codes (id, tenant_id, device_code, user_code, client_id, profile_name, authorized, user_id, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dc.ID,
		d.tenantID,
		dc.DeviceCode,
		dc.UserCode,
		dc.ClientID,
		dc.ProfileName,
		boolToInt(dc.Authorized),
		dc.UserID,
		dc.ExpiresAt,
		Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting device code: %w", err)
	}
	return nil
}

// GetDeviceCodeByDeviceCode looks up a device code by its device_code value.
func (d *DB) GetDeviceCodeByDeviceCode(ctx context.Context, deviceCode string) (*DeviceCode, error) {
	var dc DeviceCode
	var authorized int
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, device_code, user_code, client_id, profile_name, authorized, user_id, expires_at, created_at
		 FROM device_codes WHERE device_code = ?`, deviceCode,
	).Scan(
		&dc.ID, &dc.TenantID, &dc.DeviceCode, &dc.UserCode,
		&dc.ClientID, &dc.ProfileName, &authorized, &dc.UserID,
		&dc.ExpiresAt, &dc.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying device code: %w", err)
	}
	dc.Authorized = authorized != 0
	return &dc, nil
}

// GetDeviceCodeByUserCode looks up a device code by its user_code value.
func (d *DB) GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*DeviceCode, error) {
	var dc DeviceCode
	var authorized int
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, tenant_id, device_code, user_code, client_id, profile_name, authorized, user_id, expires_at, created_at
		 FROM device_codes WHERE user_code = ?`, userCode,
	).Scan(
		&dc.ID, &dc.TenantID, &dc.DeviceCode, &dc.UserCode,
		&dc.ClientID, &dc.ProfileName, &authorized, &dc.UserID,
		&dc.ExpiresAt, &dc.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying device code by user code: %w", err)
	}
	dc.Authorized = authorized != 0
	return &dc, nil
}

// AuthorizeDeviceCode marks a device code as authorized by a user.
func (d *DB) AuthorizeDeviceCode(ctx context.Context, id, userID string) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE device_codes SET authorized = 1, user_id = ? WHERE id = ?`,
		userID, id,
	)
	if err != nil {
		return fmt.Errorf("authorizing device code: %w", err)
	}
	return nil
}

// DeleteDeviceCode removes a device code (after use or expiry).
func (d *DB) DeleteDeviceCode(ctx context.Context, id string) error {
	_, err := d.conn.ExecContext(ctx,
		"DELETE FROM device_codes WHERE id = ?", id,
	)
	if err != nil {
		return fmt.Errorf("deleting device code: %w", err)
	}
	return nil
}

// DeleteExpiredDeviceCodes removes all expired device codes.
func (d *DB) DeleteExpiredDeviceCodes(ctx context.Context) (int64, error) {
	result, err := d.conn.ExecContext(ctx,
		"DELETE FROM device_codes WHERE expires_at < ?", Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("deleting expired device codes: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting rows affected: %w", err)
	}
	return affected, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
