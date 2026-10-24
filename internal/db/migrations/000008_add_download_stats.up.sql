-- Add download tracking columns to files table
ALTER TABLE files ADD COLUMN download_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE files ADD COLUMN last_downloaded_at TIMESTAMP;

-- Add index for sorting by download count
CREATE INDEX idx_files_download_count ON files(download_count DESC);
