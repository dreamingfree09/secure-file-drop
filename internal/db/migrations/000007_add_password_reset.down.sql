-- Rollback password reset
DROP INDEX IF EXISTS idx_users_reset_token;
ALTER TABLE users DROP COLUMN IF EXISTS reset_token_expires;
ALTER TABLE users DROP COLUMN IF EXISTS reset_token;
