-- Add optional password protection for download links
ALTER TABLE files ADD COLUMN link_password TEXT;
