-- Add WebAuthn backup eligibility and state flags to passkeys.
-- Required by go-webauthn library to validate login assertions correctly.
ALTER TABLE passkeys ADD COLUMN backup_eligible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE passkeys ADD COLUMN backup_state INTEGER NOT NULL DEFAULT 0;
