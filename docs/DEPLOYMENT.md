# Deployment

This document outlines simple deployment options and recommendations for running Secure File Drop in production.

## Quick Docker Compose deployment

The repository contains `docker-compose.yml` intended for a simple deployment with Postgres and MinIO. Basic steps:

1. Ensure required environment variables are set (see `docs/USAGE.md`).
2. Start services:

   docker compose up -d

3. Migrations are applied automatically on backend startup (see `docs/MIGRATIONS.md`).

4. Confirm readiness: `GET http://<host>:8080/ready` should return `{"status":"ok"}`.

## Reverse proxy & TLS

- Use a reverse proxy to terminate TLS and provide a stable `SFD_PUBLIC_BASE_URL`.
- Recommended: Caddy for automatic HTTPS or Traefik/Nginx if you prefer fine-grained control.
- Enforce HTTPS only and set proxy headers (X-Forwarded-Proto, X-Forwarded-Host) so the server generates correct public links.

## Secrets & configuration

- Use environment variables or a secrets manager to provide credentials and secrets:
  - SFD_SESSION_SECRET - HMAC signing key for session cookies
  - SFD_DOWNLOAD_SECRET - HMAC signing key for download tokens
  - SFD_DB_DSN - PostgreSQL connection string
  - SFD_MINIO_ENDPOINT, SFD_MINIO_ACCESS_KEY, SFD_MINIO_SECRET_KEY, SFD_MINIO_BUCKET
  - SFD_SMTP_HOST, SFD_SMTP_PORT, SFD_SMTP_USER, SFD_SMTP_PASS - Email notifications (optional)
  - SFD_SMTP_FROM - Sender email address
  - SFD_PUBLIC_BASE_URL - Base URL for email links (e.g., https://files.example.com)
- Rotate secrets periodically and keep a secure audit trail for changes.
- See `docs/EMAIL_NOTIFICATIONS.md` for SMTP setup details.

## Production considerations

- Make MinIO and Postgres accessible only to the backend service (private network).
- Use logging collection and monitoring; ensure `/health` and `/ready` are wired into your orchestrator.
- Rate limiting is built-in at the application level (see `docs/API.md`).
- Tune `SFD_MAX_UPLOAD_BYTES` to control allowed file sizes (default: 50GB).
- Configure SMTP for email notifications (registration verification, password resets, file notifications).
- Monitor storage quotas - default is 10GB per user (configurable via admin panel).
- Set up automatic file cleanup for expired files.

## Rolling updates & backups

- Back up Postgres regularly. Files are stored in MinIO; consider object storage replication or snapshot strategies depending on your provider.
- For upgrades, drain traffic from the instance, perform a rolling deploy, and verify `/ready` before reintroducing traffic.

If you'd like, I can add a sample `caddy` or `nginx` configuration snippet and a systemd unit for running the service directly on a VM.