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
- `created_by` (TEXT) — admin username or user id
- `user_id` (UUID, FK) — reference to users table (nullable for backward compatibility)
- `status` (TEXT) — one of `pending`, `stored`, `hashed`, `ready`, `failed`
- `created_at` (TIMESTAMPTZ)

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

Indexing:
- `idx_users_email` (email)
- `idx_users_username` (username)

## Migrations

- `schema.sql` — the initial schema to create `files` and indexes (applied via `psql` for local dev).
- `alter_001.sql` — migration that adds `sha256_bytes`, `created_by`, and ensures `status` exists with a check constraint.
- `000003_add_users_table.up.sql` — creates users table, adds user_id column to files, and sets up foreign key relationship
- `000003_add_users_table.down.sql` — rollback migration for users table

## Applying migrations (local/dev)

Example using `psql`:

psql -h <host> -U <user> -d <db> -f internal/db/schema.sql
psql -h <host> -U <user> -d <db> -f internal/db/alter_001.sql
psql -h <host> -U <user> -d <db> -f internal/db/migrations/000003_add_users_table.up.sql

## Notes

- Migrations are intentionally simple and applied manually for now. If you prefer, we can add a small migration runner or adopt tools like `golang-migrate`.
- The system expects the `status` lifecycle; other components assume a file is downloadable only when `status` is `hashed` or `ready`.
- User authentication supports both database users (bcrypt) and legacy admin credentials for backward compatibility.
