-- Migration 010: Add is_admin flag to users table
-- This allows proper authorization for admin endpoints

BEGIN;

-- Add is_admin column (default false for existing users)
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT false;

-- Create index for faster admin checks
CREATE INDEX IF NOT EXISTS idx_users_is_admin ON users (is_admin) WHERE is_admin = true;

COMMIT;
