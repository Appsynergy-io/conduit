-- 002_setup_state.sql
-- Tracks setup wizard state so it cannot be re-triggered after completion.

CREATE TABLE setup_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- Only one row ever
    setup_token_hash TEXT NOT NULL,          -- SHA-256 hash of the setup token
    current_step TEXT NOT NULL DEFAULT 'domain_config',
    completed_steps TEXT NOT NULL DEFAULT '[]',  -- JSON array
    completed INTEGER NOT NULL DEFAULT 0,
    completed_at TEXT,
    created_at TEXT NOT NULL
);
