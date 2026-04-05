-- Migration 008: Persistent server keys (JWT signing key)
-- NIST SC-12: Cryptographic Key Establishment and Management
-- NIST SC-17: Public Key Infrastructure Certificates
--
-- Before this migration, the server generated a fresh Ed25519 JWT signing
-- keypair on every startup, which invalidated every user's access/refresh
-- tokens after any restart. We now persist the key so tokens survive restarts.

CREATE TABLE IF NOT EXISTS server_keys (
    key_type TEXT PRIMARY KEY,  -- e.g. 'jwt_ed25519'
    private_key BLOB NOT NULL,  -- raw private key bytes
    created_at TEXT NOT NULL
);
