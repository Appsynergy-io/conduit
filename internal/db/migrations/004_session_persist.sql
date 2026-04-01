-- Migration 004: Persistent/resumable shell sessions
-- Adds pinned mode, idle timeout, and detached state to shell sessions.
-- NIST AC-12: session termination via idle timeout
-- NIST AU-2: detach/attach events auditable

ALTER TABLE shell_sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE shell_sessions ADD COLUMN idle_timeout INTEGER NOT NULL DEFAULT 3600;
ALTER TABLE shell_sessions ADD COLUMN detached_at TEXT;

-- Update status check: active, detached, closed
-- SQLite doesn't support ALTER CHECK, but the application layer enforces this.

CREATE INDEX idx_shell_sessions_pinned ON shell_sessions(pinned);
CREATE INDEX idx_shell_sessions_detached_at ON shell_sessions(detached_at);
