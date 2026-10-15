-- Store only token hashes so logout can revoke signed sessions across instances.
CREATE TABLE sessions (
 token_hash CHAR(64) PRIMARY KEY,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
