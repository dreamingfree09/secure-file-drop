-- Migration: Add resumable_uploads table for TUS protocol support
-- This enables resumable file uploads with progress tracking

CREATE TABLE IF NOT EXISTS resumable_uploads (
    id TEXT PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    total_size BIGINT NOT NULL,
    current_size BIGINT NOT NULL DEFAULT 0,
    upload_id TEXT NOT NULL, -- MinIO multipart upload ID
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_modified TIMESTAMP NOT NULL DEFAULT NOW(),
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'failed')),
    
    CONSTRAINT resumable_uploads_size_check CHECK (current_size >= 0 AND current_size <= total_size)
);

-- Index for querying by file_id
CREATE INDEX IF NOT EXISTS idx_resumable_uploads_file_id ON resumable_uploads(file_id);

-- Index for querying active uploads
CREATE INDEX IF NOT EXISTS idx_resumable_uploads_status ON resumable_uploads(status);

-- Index for cleanup queries (find old uploads)
CREATE INDEX IF NOT EXISTS idx_resumable_uploads_created_at ON resumable_uploads(created_at);

-- Cleanup old failed/completed uploads after 7 days
CREATE OR REPLACE FUNCTION cleanup_old_resumable_uploads()
RETURNS void AS $$
BEGIN
    DELETE FROM resumable_uploads 
    WHERE status IN ('completed', 'failed') 
      AND last_modified < NOW() - INTERVAL '7 days';
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE resumable_uploads IS 'Tracks resumable file upload sessions using TUS protocol';
COMMENT ON COLUMN resumable_uploads.upload_id IS 'MinIO multipart upload identifier';
COMMENT ON COLUMN resumable_uploads.current_size IS 'Number of bytes uploaded so far';
COMMENT ON COLUMN resumable_uploads.total_size IS 'Total file size in bytes';
