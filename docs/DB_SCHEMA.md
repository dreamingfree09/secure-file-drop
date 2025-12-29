# Database schema & migrations

This document summarises the database schema and available migration files in `internal/db/`.

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
- `created_by` (TEXT) — username who uploaded the file
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
- `password_hash` (TEXT, NOT NULL) — bcrypt hashed password (cost factor 10)
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

All migrations are in `internal/db/migrations/` and auto-apply on backend startup:

- `schema.sql` — initial schema (legacy)
- `alter_001.sql` — adds sha256_bytes, created_by, status constraint (legacy)
- `000001_initial_files_table.up.sql` — creates files table
- `000002_add_user_id_to_files.up.sql` — adds user_id foreign key
- `000003_add_users_table.up.sql` — creates users table
- `000004_add_file_expiration.up.sql` — adds expires_at and auto_delete
- `000005_add_link_password.up.sql` — adds password protection for downloads
- `000006_add_email_verification.up.sql` — adds verification_token and email_verified
- `000007_add_password_reset.up.sql` — adds reset_token and reset_token_expires
- `000008_add_download_stats.up.sql` — adds download_count and last_downloaded_at
- `000009_add_user_quotas.up.sql` — adds storage_quota_bytes to users

Each migration has a corresponding `.down.sql` file for rollback.

### Migration Status

Current version: **000009** (9 migrations applied)

All migrations are idempotent and safe to re-run.

## Applying migrations (local/dev)

Example using `psql`:

psql -h <host> -U <user> -d <db> -f internal/db/schema.sql
psql -h <host> -U <user> -d <db> -f internal/db/alter_001.sql
psql -h <host> -U <user> -d <db> -f internal/db/migrations/000003_add_users_table.up.sql

## Notes

- Migrations are intentionally simple and applied manually for now. If you prefer, we can add a small migration runner or adopt tools like `golang-migrate`.
- The system expects the `status` lifecycle; other components assume a file is downloadable only when `status` is `hashed` or `ready`.
- User authentication supports both database users (bcrypt) and legacy admin credentials for backward compatibility.
