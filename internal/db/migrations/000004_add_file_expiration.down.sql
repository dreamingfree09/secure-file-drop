-- Rollback file expiration
DROP INDEX IF EXISTS idx_files_expires_at;
ALTER TABLE files DROP COLUMN IF EXISTS auto_delete;
ALTER TABLE files DROP COLUMN IF EXISTS expires_at;
