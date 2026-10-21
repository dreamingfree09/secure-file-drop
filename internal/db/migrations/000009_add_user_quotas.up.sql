-- Add storage quota to users table (in bytes, NULL = unlimited)
ALTER TABLE users ADD COLUMN storage_quota_bytes BIGINT;

-- Set default quota to 10GB for existing users (can be modified per user)
UPDATE users SET storage_quota_bytes = 10737418240 WHERE storage_quota_bytes IS NULL;
