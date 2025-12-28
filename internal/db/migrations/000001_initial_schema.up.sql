-- Initial schema for Secure File Drop
-- Migration: 000001_initial_schema
-- Applied: 2025-12-25

BEGIN;

-- Files uploaded into the system.
-- Lifecycle:
--   pending  -> metadata created, object not yet finalised
--   stored   -> object stored successfully in MinIO
--   hashed   -> sha256 computed and recorded
--   ready    -> downloadable (signed links will be based on this state)
--   failed   -> upload or processing failed (kept for audit/troubleshooting)
CREATE TABLE IF NOT EXISTS files (
    id           UUID PRIMARY KEY,

    -- Key used in object storage (MinIO). Must be unique.
    object_key   TEXT NOT NULL UNIQUE,

    -- Original client-provided name.
    orig_name    TEXT NOT NULL,

    -- Declared content-type (best-effort; we will not trust blindly later).
    content_type TEXT NOT NULL,

    -- Size in bytes (as known by server / upload handler).
    size_bytes   BIGINT NOT NULL CHECK (size_bytes >= 0),

    -- Integrity: SHA-256 of the stored file, lowercase hex.
    sha256_hex   CHAR(64),
    sha256_bytes BIGINT CHECK (sha256_bytes IS NULL OR sha256_bytes >= 0),

    -- Who performed the upload (for now: admin username; later could be user id).
    created_by   TEXT NOT NULL DEFAULT 'admin',

    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','stored','hashed','ready','failed')),

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_files_created_at ON files (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_files_status ON files (status);

COMMIT;
