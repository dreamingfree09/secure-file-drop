-- Migration 011: Remove redundant created_by column
-- The user_id (UUID FK) is sufficient and more reliable than text username

BEGIN;

-- Remove created_by column as user_id is the proper foreign key
ALTER TABLE files DROP COLUMN IF EXISTS created_by;

COMMIT;
