package db

import (
	"context"
	"fmt"
)

// CreateTenant inserts the single CE tenant.
func (d *DB) CreateTenant(ctx context.Context, id, name string) error {
	_, err := d.conn.ExecContext(ctx,
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		id, name, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting tenant: %w", err)
	}
	d.tenantID = id
	return nil
}

// GetTenant returns the tenant name for the given ID.
func (d *DB) GetTenant(ctx context.Context, id string) (string, error) {
	var name string
	err := d.conn.QueryRowContext(ctx,
		"SELECT name FROM tenants WHERE id = ?", id,
	).Scan(&name)
	if err != nil {
		return "", fmt.Errorf("querying tenant: %w", err)
	}
	return name, nil
}

// CreateService inserts a service into the registry.
func (d *DB) CreateService(ctx context.Context, id, slug, name, description string) error {
	_, err := d.conn.ExecContext(ctx,
		"INSERT INTO services (id, slug, name, description, created_at) VALUES (?, ?, ?, ?, ?)",
		id, slug, name, description, Now(),
	)
	if err != nil {
		return fmt.Errorf("inserting service: %w", err)
	}
	return nil
}

// EnableServiceForTenant links a service to the tenant.
func (d *DB) EnableServiceForTenant(ctx context.Context, tenantID, serviceID string) error {
	_, err := d.conn.ExecContext(ctx,
		"INSERT INTO tenant_services (tenant_id, service_id, enabled_at) VALUES (?, ?, ?)",
		tenantID, serviceID, Now(),
	)
	if err != nil {
		return fmt.Errorf("enabling service for tenant: %w", err)
	}
	return nil
}

// Service represents a row from the services table joined with tenant enablement.
type Service struct {
	ID          string
	Slug        string
	Name        string
	Description string
	CreatedAt   string
	EnabledAt   string // Non-empty if enabled for the tenant
}

// ListServices returns all services, with enablement status for the given tenant.
func (d *DB) ListServices(ctx context.Context, tenantID string) ([]Service, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT s.id, s.slug, s.name, s.description, s.created_at,
		       COALESCE(ts.enabled_at, '') AS enabled_at
		FROM services s
		LEFT JOIN tenant_services ts ON ts.service_id = s.id AND ts.tenant_id = ?
		ORDER BY s.slug
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.CreatedAt, &s.EnabledAt); err != nil {
			return nil, fmt.Errorf("scanning service: %w", err)
		}
		services = append(services, s)
	}
	return services, rows.Err()
}
