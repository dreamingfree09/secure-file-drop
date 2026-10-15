# Schema and migrations

The embedded migration set in internal/db/migrations is authoritative. Startup runs forward migrations automatically using DATABASE_URL; [migration operations](MIGRATIONS.md) describes safe manual execution.

- files: UUID id; generated unique object_key; orig_name/content_type; actual size_bytes; sha256_hex and sha256_bytes; created_by; nullable user_id ownership; pending/stored/hashed/ready/failed status; created_at.
- users: UUID id, unique email/username, bcrypt password_hash, creation/update/login timestamps and is_active.
- sessions: SHA-256 token_hash primary key and expires_at. No raw session token is stored.

Migration 1 creates files, migration 2 is the historical lifecycle placeholder, migration 3 creates users and adds nullable file ownership, and migration 4 adds revocable sessions. Applied migrations are preserved unchanged. The obsolete standalone schema.sql and alter_001.sql represent an early files-only schema; they are legacy fixtures rather than a complete setup procedure.

Historical ownerless rows cannot be safely assigned to registered users and remain administrator-only. Existing single-admin sessions must log in again after adopting session records. File rows have no updated_at column; list endpoints return created_at only.
