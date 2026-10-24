-- Legacy schemas may already have this manually applied role column.
-- Preserve every existing role and user; new schemas default to ordinary users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE auth_sessions (
 token_hash TEXT PRIMARY KEY CHECK (length(token_hash) = 64),
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX auth_sessions_expiry ON auth_sessions(expires_at);
