package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps the SQLite connection and provides entity-scoped methods.
type DB struct {
	conn     *sql.DB
	tenantID string // Loaded at startup; single CE tenant
}

// New opens a SQLite database at the given path (use ":memory:" for tests),
// applies pragmas, runs migrations, and returns a ready DB.
func New(ctx context.Context, dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite pragmas for safety and performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-64000", // 64MB
	}
	for _, p := range pragmas {
		if _, err := conn.ExecContext(ctx, p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	d := &DB{conn: conn}
	if err := d.migrate(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	// Load tenant ID if one exists (will be empty until setup completes)
	tenantID, err := d.loadTenantID(ctx)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("loading tenant ID: %w", err)
	}
	d.tenantID = tenantID

	return d, nil
}

// TenantID returns the single CE tenant UUID.
func (d *DB) TenantID() string {
	return d.tenantID
}

// SetTenantID sets the tenant ID (used during setup).
func (d *DB) SetTenantID(id string) {
	d.tenantID = id
}

// Conn returns the underlying *sql.DB for direct use in tests or transactions.
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Now returns the current time in UTC RFC 3339 format for consistent timestamps.
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// migrate applies embedded SQL migration files in order.
func (d *DB) migrate(ctx context.Context) error {
	// Create schema_version table if not exists
	if _, err := d.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	err := d.conn.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("reading current schema version: %w", err)
	}

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version number from filename: "001_initial.sql" → 1
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			slog.WarnContext(ctx, "skipping migration file with unparseable name", "file", entry.Name())
			continue
		}

		if version <= currentVersion {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", entry.Name(), err)
		}

		slog.InfoContext(ctx, "applying migration", "version", version, "file", entry.Name())

		tx, err := d.conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", entry.Name(), err)
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
			version, Now(),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration version %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", version, err)
		}
	}

	return nil
}

// loadTenantID reads the single tenant UUID from the tenants table.
func (d *DB) loadTenantID(ctx context.Context) (string, error) {
	var id string
	err := d.conn.QueryRowContext(ctx, "SELECT id FROM tenants LIMIT 1").Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil // No tenant yet — setup not complete
	}
	if err != nil {
		return "", fmt.Errorf("querying tenant: %w", err)
	}
	return id, nil
}
