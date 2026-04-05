-- Migration 007: Multi-client session support (watch and take control)
-- NIST AC-3: Role-based session access control
-- NIST AU-2: Client connection events auditable

-- Track who connected to each session and in what role (audit trail)
CREATE TABLE IF NOT EXISTS shell_session_clients (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    session_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id),
    role TEXT NOT NULL DEFAULT 'watcher',
    connected_at TEXT NOT NULL,
    disconnected_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_ssc_session ON shell_session_clients(session_id);
CREATE INDEX IF NOT EXISTS idx_ssc_user ON shell_session_clients(user_id);
