-- Migration 005: Device authorization flow for CLI authentication
-- NIST IA-2: Multi-factor authentication via browser passkey delegation
-- OWASP API2: Secure device flow with short-lived codes

CREATE TABLE device_codes (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    device_code TEXT UNIQUE NOT NULL,
    user_code TEXT UNIQUE NOT NULL,
    client_id TEXT NOT NULL DEFAULT '',
    profile_name TEXT NOT NULL DEFAULT '',
    authorized INTEGER NOT NULL DEFAULT 0,
    user_id TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_device_codes_device_code ON device_codes(device_code);
CREATE INDEX idx_device_codes_user_code ON device_codes(user_code);
CREATE INDEX idx_device_codes_expires_at ON device_codes(expires_at);
