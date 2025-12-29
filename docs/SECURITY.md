# Security Overview

This document summarizes Secure File Drop's security features and operational best practices.

## Session Security
- **Cookie**: `sfd_session` (HttpOnly, SameSite=Lax, Secure in production)
- **Model**: Stateless HMAC-signed payload; server validates signature, no server-side session store
- **TTL**: 12 hours; sessions do not auto-renew
- **Logout**: `POST /logout` clears cookie

## Authentication & Account Safety
- **Email Verification**: Mandatory before login; `GET /verify?token=...`
- **Password Policy**: Minimum 8 chars; includes upper, lower, and number
- **Password Hashing**: Bcrypt cost 12
- **Password Reset**: `POST /reset-password-request` then `POST /reset-password` with 1 hour token expiry

## Download Link Integrity
- **Signed Tokens**: HMAC with expiry; tokens include file id and expiration
- **Public Base URL**: Links generated using `SFD_PUBLIC_BASE_URL` when set
- **Optional Password**: Per-link password protection; validated server-side

## Storage & Data Handling
- **Object Storage**: MinIO (S3-compatible), private to backend network
- **Object Keys**: Internal UUID-based under `uploads/` with no user-provided paths
- **Lifecycle States**: `pending → stored → hashed → ready/failed`
- **Cleanup**: Background and manual cleanup of `pending/failed` and expired files

## Rate Limiting & Abuse Protection
- **Global Limit**: 100 requests/min per IP (token bucket)
- **Protected Endpoints**: All user/admin routes require authentication
- **Logging**: Structured request logs with request ID

## Configuration Secrets
- **Required**: `SFD_ADMIN_PASS`, `SFD_SESSION_SECRET`, `SFD_DOWNLOAD_SECRET`
- **Email**: `SFD_SMTP_HOST`, `SFD_SMTP_PORT`, `SFD_SMTP_USER`, `SFD_SMTP_PASS`, `SFD_SMTP_FROM`
- **Base URL**: Prefer `SFD_PUBLIC_BASE_URL` (fallback to `SFD_BASE_URL`)
- **Storage/DB**: MinIO credentials and `DATABASE_URL`

## Deployment Best Practices
- **TLS**: Terminate TLS at reverse proxy (Traefik/Caddy)
- **Headers**: Security headers (HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy)
- **Isolation**: Keep MinIO and Postgres on private network; only expose backend via proxy
- **Resource Limits**: Configure `SFD_MAX_UPLOAD_BYTES` to prevent oversized uploads
- **Monitoring**: Use `/health`, `/ready`, and `/metrics` for observability

## Admin Protections
- **Admin Routes**: `GET /admin/files`, `DELETE /admin/files/{id}`, `POST /admin/cleanup` protected by `requireAdmin`
- **User Ownership**: `DELETE /user/files/{id}` enforces ownership via `user_id`
- **Notifications**: Best-effort emails on deletions

For detailed API behavior, see [docs/API.md](API.md). For architecture details, see [docs/ARCHITECTURE.md](ARCHITECTURE.md).
