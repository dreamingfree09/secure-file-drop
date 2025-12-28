-- Rollback users table
DROP INDEX IF EXISTS idx_files_user_id;
ALTER TABLE files DROP COLUMN IF EXISTS user_id;

DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
