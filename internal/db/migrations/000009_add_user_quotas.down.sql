-- Remove storage quota column
ALTER TABLE users DROP COLUMN IF EXISTS storage_quota_bytes;
