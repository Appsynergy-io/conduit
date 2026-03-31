-- 001_initial.sql
-- Full CE schema — NIST SP 800-53 Rev. 5, OWASP ASVS v4.0.3

-- Tenant (CE generates one UUID at setup — NIST IA-4)
-- All tables reference this for SaaS transferability
CREATE TABLE tenants (
    id TEXT PRIMARY KEY,              -- UUID v4, generated at setup
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Service registry (CE ships with one: 'remote-access')
CREATE TABLE services (
    id TEXT PRIMARY KEY,              -- UUID v4
    slug TEXT UNIQUE NOT NULL,        -- 'remote-access'
    name TEXT NOT NULL,               -- 'Conduit Remote Access'
    description TEXT,
    created_at TEXT NOT NULL
);

-- Tenant <> Service junction (which services a tenant has access to)
CREATE TABLE tenant_services (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    service_id TEXT NOT NULL REFERENCES services(id),
    enabled_at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, service_id)
);

-- Users (single tenant, multiple users with RBAC)
CREATE TABLE users (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    email TEXT UNIQUE NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    display_name TEXT,
    role TEXT NOT NULL DEFAULT 'org_member',
    status TEXT NOT NULL DEFAULT 'active',
    password_hash TEXT,               -- Argon2id, dev mode only
    created_by TEXT REFERENCES users(id),
    last_login_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Groups
CREATE TABLE groups (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Group membership (users)
CREATE TABLE group_members (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    group_id TEXT NOT NULL REFERENCES groups(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

-- Group membership (agents)
CREATE TABLE agent_group_members (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    group_id TEXT NOT NULL REFERENCES groups(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, agent_id)
);

-- WebAuthn credentials
CREATE TABLE passkeys (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    credential_id BLOB NOT NULL,
    public_key BLOB NOT NULL,
    algorithm TEXT,
    algorithm_warning TEXT,
    authenticator_type TEXT,
    sign_count INTEGER NOT NULL DEFAULT 0,
    display_name TEXT,
    created_at TEXT NOT NULL,
    last_used_at TEXT
);

-- SSO providers (SAML/OIDC)
CREATE TABLE sso_providers (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    protocol TEXT NOT NULL,
    name TEXT NOT NULL,
    config TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

-- Registered agents
CREATE TABLE agents (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    hostname TEXT NOT NULL,
    display_name TEXT,
    os TEXT,
    arch TEXT,
    labels TEXT,                      -- JSON
    ip TEXT,
    agent_key_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline',
    transport TEXT,
    version TEXT,
    last_seen_at TEXT,
    connected_at TEXT,
    created_at TEXT NOT NULL
);

-- Join tokens
CREATE TABLE join_tokens (
    id TEXT PRIMARY KEY,              -- UUID v4 (also the token JTI)
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    labels TEXT,                      -- JSON
    token_hash TEXT NOT NULL,
    used_count INTEGER DEFAULT 0,
    max_uses INTEGER,
    expires_at TEXT,
    revoked INTEGER DEFAULT 0,
    revoked_at TEXT,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Active sessions (visibility/revocation)
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,
    source_ip TEXT,
    user_agent TEXT,
    refresh_token_hash TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_active_at TEXT NOT NULL
);

-- Shell sessions (active + closed)
CREATE TABLE shell_sessions (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'active',
    shell TEXT,
    cols INTEGER,
    rows INTEGER,
    recording INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    closed_at TEXT
);

-- Shell recordings (asciicast v2)
CREATE TABLE shell_recordings (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    session_id TEXT NOT NULL REFERENCES shell_sessions(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    agent_hostname TEXT,
    user_email TEXT,
    duration INTEGER,
    size_bytes INTEGER,
    format TEXT NOT NULL DEFAULT 'asciicast-v2',
    created_at TEXT NOT NULL
);

-- RBAC role assignments
CREATE TABLE role_assignments (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    role TEXT NOT NULL,
    user_id TEXT REFERENCES users(id),
    group_id TEXT REFERENCES groups(id),
    scope TEXT,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    CHECK (
        (user_id IS NOT NULL AND group_id IS NULL) OR
        (user_id IS NULL AND group_id IS NOT NULL)
    )
);

-- Audit log (append-only, immutable)
CREATE TABLE audit_log (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    event_type TEXT NOT NULL,
    user_id TEXT,
    user_email TEXT,
    agent_id TEXT,
    agent_hostname TEXT,
    source_ip TEXT,
    user_agent TEXT,
    details TEXT,                     -- JSON
    outcome TEXT NOT NULL,
    algorithm_used TEXT,
    algorithm_warning TEXT,
    timestamp TEXT NOT NULL
);

-- Webhook subscriptions
CREATE TABLE webhook_subscriptions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    url TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    events TEXT NOT NULL,             -- JSON
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Webhook delivery history
CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    subscription_id TEXT NOT NULL REFERENCES webhook_subscriptions(id),
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    http_status INTEGER,
    response_time INTEGER,
    attempt_number INTEGER NOT NULL DEFAULT 1,
    next_retry_at TEXT,
    delivered_at TEXT,
    attempted_at TEXT NOT NULL
);

-- Bulk exec jobs
CREATE TABLE bulk_exec_jobs (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    command TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    target_count INTEGER,
    completed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    results TEXT,                      -- JSON
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    completed_at TEXT
);

-- Binary deploy jobs
CREATE TABLE deploy_jobs (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    status TEXT NOT NULL DEFAULT 'uploading',
    target_count INTEGER,
    completed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    destination_path TEXT,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- CI tokens
CREATE TABLE ci_tokens (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scopes TEXT NOT NULL DEFAULT '[]',
    expires_at TEXT,
    last_used_at TEXT,
    created_at TEXT NOT NULL
);

-- Recovery codes (one-time, Argon2id hashed)
CREATE TABLE recovery_codes (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    code_hash TEXT NOT NULL,
    used INTEGER NOT NULL DEFAULT 0,
    used_at TEXT,
    created_at TEXT NOT NULL
);

-- ── Indexes ──────────────────────────────────────────

CREATE INDEX idx_users_tenant_id ON users(tenant_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_agents_tenant_id ON agents(tenant_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_join_tokens_tenant_id ON join_tokens(tenant_id);
CREATE INDEX idx_join_tokens_revoked ON join_tokens(revoked);
CREATE INDEX idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_shell_sessions_tenant_id ON shell_sessions(tenant_id);
CREATE INDEX idx_shell_sessions_agent_id ON shell_sessions(agent_id);
CREATE INDEX idx_shell_sessions_user_id ON shell_sessions(user_id);
CREATE INDEX idx_shell_sessions_status ON shell_sessions(status);
CREATE INDEX idx_shell_recordings_tenant_id ON shell_recordings(tenant_id);
CREATE INDEX idx_shell_recordings_session_id ON shell_recordings(session_id);
CREATE INDEX idx_shell_recordings_agent_id ON shell_recordings(agent_id);
CREATE INDEX idx_shell_recordings_user_id ON shell_recordings(user_id);
CREATE INDEX idx_role_assignments_tenant_id ON role_assignments(tenant_id);
CREATE INDEX idx_role_assignments_user_id ON role_assignments(user_id);
CREATE INDEX idx_role_assignments_group_id ON role_assignments(group_id);
CREATE INDEX idx_audit_log_tenant_id ON audit_log(tenant_id);
CREATE INDEX idx_audit_log_event_type ON audit_log(event_type);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_webhook_subscriptions_tenant_id ON webhook_subscriptions(tenant_id);
CREATE INDEX idx_webhook_deliveries_tenant_id ON webhook_deliveries(tenant_id);
CREATE INDEX idx_webhook_deliveries_subscription_id ON webhook_deliveries(subscription_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status);
CREATE INDEX idx_passkeys_tenant_id ON passkeys(tenant_id);
CREATE INDEX idx_group_members_tenant_id ON group_members(tenant_id);
CREATE INDEX idx_agent_group_members_tenant_id ON agent_group_members(tenant_id);
CREATE INDEX idx_sso_providers_tenant_id ON sso_providers(tenant_id);
CREATE INDEX idx_tenant_services_tenant_id ON tenant_services(tenant_id);
CREATE INDEX idx_bulk_exec_jobs_tenant_id ON bulk_exec_jobs(tenant_id);
CREATE INDEX idx_bulk_exec_jobs_status ON bulk_exec_jobs(status);
CREATE INDEX idx_bulk_exec_jobs_created_by ON bulk_exec_jobs(created_by);
CREATE INDEX idx_deploy_jobs_tenant_id ON deploy_jobs(tenant_id);
CREATE INDEX idx_deploy_jobs_status ON deploy_jobs(status);
CREATE INDEX idx_ci_tokens_tenant_id ON ci_tokens(tenant_id);
CREATE INDEX idx_ci_tokens_created_by ON ci_tokens(created_by);
CREATE INDEX idx_recovery_codes_user_id ON recovery_codes(user_id);
CREATE INDEX idx_recovery_codes_tenant_id ON recovery_codes(tenant_id);
