# Database schema & migrations

This document summarises the database schema and available migration files in `internal/db/`. Migrations are embedded and auto-applied on backend startup.

## Primary tables

### `files` table

The `files` table tracks file metadata and lifecycle state.

Columns of interest:
- `id` (UUID, PK) — stable identifier used across the system
- `object_key` (TEXT) — the MinIO key (e.g., `uploads/<uuid>`)
- `orig_name` (TEXT) — original client-provided filename
- `content_type` (TEXT) — declared content type
- `size_bytes` (BIGINT) — file size recorded at upload
- `sha256_hex` (CHAR(64)) — lowercase hex SHA-256 of the stored object
- `sha256_bytes` (BIGINT) — the byte count computed during hashing
- `created_by` (TEXT) — username who uploaded the file (legacy; see `user_id`)
- `user_id` (UUID, FK) — reference to users table
- `status` (TEXT) — one of `pending`, `stored`, `hashed`, `ready`, `failed`
- `created_at` (TIMESTAMPTZ) — file creation timestamp
- `expires_at` (TIMESTAMPTZ, nullable) — auto-deletion time for TTL files
- `auto_delete` (BOOLEAN) — whether to auto-delete when expired
- `link_password` (TEXT, nullable) — bcrypt hash for password-protected downloads
- `download_count` (INTEGER, default 0) — number of times file was downloaded
- `last_downloaded_at` (TIMESTAMPTZ, nullable) — timestamp of last download

Indexing:
- `idx_files_created_at` (created_at DESC)
- `idx_files_status` (status)

### `users` table

The `users` table stores registered user accounts with secure password hashing.

Columns:
- `id` (UUID, PK) — unique user identifier
- `email` (TEXT, UNIQUE, NOT NULL) — user's email address
- `username` (TEXT, UNIQUE, NOT NULL) — unique username (3-50 chars, alphanumeric + underscore)
- `password_hash` (TEXT, NOT NULL) — bcrypt hashed password (cost factor 12)
- `created_at` (TIMESTAMPTZ) — account creation timestamp
- `updated_at` (TIMESTAMPTZ) — last update timestamp
- `verification_token` (TEXT, nullable) — email verification token
- `verification_sent_at` (TIMESTAMPTZ, nullable) — when verification email was sent
- `email_verified` (BOOLEAN, default false) — email verification status
- `reset_token` (TEXT, nullable) — password reset token
- `reset_token_expires` (TIMESTAMPTZ, nullable) — reset token expiration time
- `storage_quota_bytes` (BIGINT, nullable) — per-user storage quota (default 10GB)

Indexing:
- `idx_users_email` (email)
- `idx_users_username` (username)

## Migrations

All migrations are in `internal/db/migrations/` and auto-apply on backend startup via golang-migrate:

- `000001_initial_schema.up.sql` — creates initial schema
- `000002_add_lifecycle_fields.up.sql` — adds lifecycle constraints and indexes
- `000003_add_users_table.up.sql` — creates users table
- `000004_add_file_expiration.up.sql` — adds `expires_at` and `auto_delete`
- `000005_add_link_password.up.sql` — adds password protection for downloads
- `000006_add_email_verification.up.sql` — adds verification token fields
- `000007_add_password_reset.up.sql` — adds reset token and expiry
- `000008_add_download_stats.up.sql` — adds download_count and last_downloaded_at
- `000009_add_user_quotas.up.sql` — adds per-user storage quota

Each migration has a corresponding `.down.sql` file for rollback.

### Migration Status

Current version: **000009** (9 migrations applied). Pending migrations are applied automatically on backend start.

## Notes

- Files are downloadable only when `status` is `hashed` or `ready`.
- Legacy schema files (`schema.sql`, `alter_001.sql`) are preserved for reference but not used in production.
