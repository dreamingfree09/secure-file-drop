-- Migration 012: Add partial unique index to prevent duplicate uploads
-- This prevents the same user from uploading the same file (name + size) within a short time window
BEGIN;

-- Create a partial unique index on user_id, orig_name, size_bytes for recent files only
-- This prevents race conditions where duplicate requests create multiple records
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS files_duplicate_prevention 
ON files (user_id, orig_name, size_bytes) 
WHERE created_at > NOW() - INTERVAL '30 seconds';

COMMIT;
