# Architecture

This document describes the high-level components and responsibilities of Secure File Drop.

## Components

- Reverse proxy (recommended: Caddy)
  - Terminates TLS, enforces global rate limits, and provides an externally reachable hostname.

- Backend API (Go)
  - Auth: simple admin username/password -> HMAC-signed session cookie
  - File metadata management: PostgreSQL stores file records and lifecycle state
  - Upload handling: POST /upload streams multipart file parts into MinIO
  - Hashing: after upload, files are hashed (SHA-256) and metadata persisted
  - Signed download: short-lived signed tokens for direct download via /download

- Object storage (MinIO)
  - Stores file blobs under internal keys (no user-provided paths)
  - Must be private (not exposed to the public internet)

- Database (Postgres)
  - `files` table tracks id, status (pending/stored/hashed/ready/failed), size, content type, sha256 metadata
  - Migrations live in `internal/db/` (see `schema.sql` and `alter_001.sql`)

- Native hashing utility (C)
  - Computes SHA-256 over streamed objects and emits machine-readable outputs
  - Useful for reproducible integrity checks and educational value

## Data flow

1. Authenticated user creates a file record (POST /files). A server-generated UUID and object key are returned.
2. User uploads the file via POST /upload?id=<uuid> as multipart form with field `file`.
3. Server streams the file into MinIO. On success, it computes SHA-256 (using MinIO get + local hashing helper) and stores results in the database.
4. User requests a download link (POST /links with file id and TTL). Server signs a download token and returns a URL.
5. Anyone with the link can GET /download?token=<token> until the token expires.

## Security notes

- Reverse proxy must enforce HTTPS and recommended security headers.
- Keep MinIO and Postgres private to the backend network.
- Secrets (admin credentials, session secret, download secret) must be provided through environment variables or secret management systems.

## Operational notes

- Reverse proxy health checks can use /health (process liveness) and /ready (dependency readiness).
- Monitor DB connectivity and MinIO availability.

For more details about the API, see `docs/API.md` and `docs/USAGE.md`.