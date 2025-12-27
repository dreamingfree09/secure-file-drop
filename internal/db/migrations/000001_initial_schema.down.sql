-- Rollback initial schema
BEGIN;

DROP INDEX IF EXISTS idx_files_status;
DROP INDEX IF EXISTS idx_files_created_at;
DROP TABLE IF EXISTS files;

COMMIT;
