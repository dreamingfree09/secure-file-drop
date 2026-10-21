-- Add password reset support
ALTER TABLE users ADD COLUMN reset_token VARCHAR(64);
ALTER TABLE users ADD COLUMN reset_token_expires TIMESTAMPTZ;

CREATE INDEX idx_users_reset_token ON users(reset_token);
