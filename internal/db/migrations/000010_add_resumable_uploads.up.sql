-- Add resumable_uploads table for TUS protocol support
CREATE TABLE IF NOT EXISTS resumable_uploads (
    id TEXT PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    total_size BIGINT NOT NULL,
    current_size BIGINT NOT NULL DEFAULT 0,
    upload_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_modified TIMESTAMP NOT NULL DEFAULT NOW(),
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'failed')),
    
    CONSTRAINT resumable_uploads_size_check CHECK (current_size >= 0 AND current_size <= total_size)
);

CREATE INDEX IF NOT EXISTS idx_resumable_uploads_file_id ON resumable_uploads(file_id);
CREATE INDEX IF NOT EXISTS idx_resumable_uploads_status ON resumable_uploads(status);
CREATE INDEX IF NOT EXISTS idx_resumable_uploads_created_at ON resumable_uploads(created_at);
