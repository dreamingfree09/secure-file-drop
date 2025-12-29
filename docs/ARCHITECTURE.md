# Architecture

This document describes the high-level components and responsibilities of Secure File Drop.

## Components

- Reverse proxy (recommended: Caddy)
  - Terminates TLS, enforces global rate limits, and provides an externally reachable hostname.

- Backend API (Go)
  - Auth: User registration with email verification, login with session cookies, password reset flow
  - File metadata management: PostgreSQL stores file records and lifecycle state
  - Upload handling: POST /upload streams multipart file parts into MinIO with quota enforcement
  - Hashing: after upload, files are hashed (SHA-256) and metadata persisted
  - Signed download: short-lived signed tokens for direct download via /download
  - Email notifications: SMTP integration for verification, password reset, and file activity alerts
  - Rate limiting: Per-IP and per-user protection against abuse
  - Admin panel: User management, quota settings, system statistics

- Object storage (MinIO)
  - Stores file blobs under internal keys (no user-provided paths)
  - Must be private (not exposed to the public internet)

- Database (Postgres)
  - `files` table tracks id, status, size, content type, sha256, expiration, download stats, password protection
  - `users` table tracks authentication, email verification, password reset tokens, storage quotas
  - 9 migrations manage schema evolution (see `docs/MIGRATIONS.md`)

- Email Service (SMTP)
  - Sends verification emails for new registrations
  - Sends password reset tokens with 1-hour expiry
  - Sends upload/download notifications
  - Supports Gmail, SendGrid, AWS SES, custom SMTP (see `docs/EMAIL_NOTIFICATIONS.md`)

- Native hashing utility (C)
  - Computes SHA-256 over streamed objects and emits machine-readable outputs
  - Useful for reproducible integrity checks and educational value

## Data flow

### User Registration & Verification
1. User submits registration form (POST /register)
2. System creates unverified user account with verification token
3. Email sent with verification link (token valid for 24 hours)
4. User clicks link (GET /verify?token=xxx) to activate account
5. User can now login and upload files

### File Upload & Download
1. Authenticated user creates a file record (POST /files). A server-generated UUID and object key are returned.
2. User uploads the file via POST /upload?id=<uuid> as multipart form with field `file`.
3. System checks user quota before accepting upload
4. Server streams the file into MinIO. On success, it computes SHA-256 and stores results in the database.
5. Optional: User can set file expiration (TTL) or password protection
6. User requests a download link (POST /links with file id and TTL). Server signs a download token and returns a URL.
7. Anyone with the link can GET /download?token=<token> until the token expires.
8. Download count and last download timestamp tracked for analytics

### Password Reset
1. User requests reset (POST /password-reset/request with email)
2. System generates reset token (1-hour expiry)
3. Email sent with reset link
4. User submits new password (POST /password-reset/confirm)
5. Token validated and password updated

## Security notes

- Reverse proxy must enforce HTTPS and recommended security headers.
- Keep MinIO and Postgres private to the backend network.
- Secrets (session secret, download secret, SMTP password) must be provided through environment variables or secret management systems.
- Passwords hashed with bcrypt (cost 10)
- Email verification prevents unauthorized registrations
- Rate limiting protects against brute force and abuse
- Download tokens use HMAC signatures with expiry
- Optional per-file password protection with bcrypt

## Operational notes

- Reverse proxy health checks can use /health (process liveness) and /ready (dependency readiness).
- Monitor DB connectivity and MinIO availability.
- Storage quotas default to 10GB per user (configurable via admin panel)
- Email notifications require SMTP configuration (see `docs/EMAIL_NOTIFICATIONS.md`)
- Automatic file cleanup based on expiration settings

For more details about the API, see `docs/API.md` and `docs/USAGE.md`.