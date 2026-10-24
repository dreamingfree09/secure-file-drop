-- Remove download tracking columns
DROP INDEX IF EXISTS idx_files_download_count;
ALTER TABLE files DROP COLUMN IF EXISTS last_downloaded_at;
ALTER TABLE files DROP COLUMN IF EXISTS download_count;
