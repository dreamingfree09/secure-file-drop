-- Add expiration support to files table
ALTER TABLE files ADD COLUMN expires_at TIMESTAMPTZ;
ALTER TABLE files ADD COLUMN auto_delete BOOLEAN DEFAULT false;

-- Index for efficient expiration queries
CREATE INDEX idx_files_expires_at ON files(expires_at) WHERE expires_at IS NOT NULL;

COMMENT ON COLUMN files.expires_at IS 'When the file should be automatically deleted (NULL = never expires)';
COMMENT ON COLUMN files.auto_delete IS 'Whether the file should be auto-deleted when expired';
