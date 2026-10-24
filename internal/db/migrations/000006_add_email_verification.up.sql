-- Add email verification support
ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN verification_token VARCHAR(64);
ALTER TABLE users ADD COLUMN verification_sent_at TIMESTAMPTZ;

CREATE INDEX idx_users_verification_token ON users(verification_token);
