BEGIN;

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS sha256_bytes BIGINT CHECK (sha256_bytes IS NULL OR sha256_bytes >= 0);

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT 'admin';

ALTER TABLE files
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','stored','hashed','ready','failed'));

CREATE INDEX IF NOT EXISTS idx_files_status ON files (status);

COMMIT;
