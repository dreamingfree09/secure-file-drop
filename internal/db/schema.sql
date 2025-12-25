-- Secure File Drop - initial schema (MVP)
-- Applied manually via psql in local dev.

BEGIN;

CREATE TABLE IF NOT EXISTS files (
    id           UUID PRIMARY KEY,
    object_key   TEXT NOT NULL UNIQUE,
    orig_name    TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes   BIGINT NOT NULL CHECK (size_bytes >= 0),

    sha256_hex   CHAR(64), -- set later when hashing is integrated

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_files_created_at ON files (created_at DESC);

COMMIT;
