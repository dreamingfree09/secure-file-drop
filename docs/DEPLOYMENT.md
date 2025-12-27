# Deployment

This document outlines simple deployment options and recommendations for running Secure File Drop in production.

## Quick Docker Compose deployment

The repository contains `docker-compose.yml` intended for a simple deployment with Postgres and MinIO. Basic steps:

1. Ensure required environment variables are set (see `docs/USAGE.md`).
2. Start services:

   docker compose up -d

3. Apply DB migrations:

   psql -h postgres -U postgres -d sfd -f internal/db/schema.sql
   psql -h postgres -U postgres -d sfd -f internal/db/alter_001.sql

4. Confirm readiness: `GET http://<host>:8080/ready` should return `{"status":"ok"}`.

## Reverse proxy & TLS

- Use a reverse proxy to terminate TLS and provide a stable `SFD_PUBLIC_BASE_URL`.
- Recommended: Caddy for automatic HTTPS or Traefik/Nginx if you prefer fine-grained control.
- Enforce HTTPS only and set proxy headers (X-Forwarded-Proto, X-Forwarded-Host) so the server generates correct public links.

## Secrets & configuration

- Use environment variables or a secrets manager to provide credentials and secrets:
  - SFD_ADMIN_USER / SFD_ADMIN_PASS
  - SFD_SESSION_SECRET
  - SFD_DOWNLOAD_SECRET
  - MinIO and Postgres credentials
- Rotate secrets periodically and keep a secure audit trail for changes.

## Production considerations

- Make MinIO and Postgres accessible only to the backend service (private network).
- Use logging collection and monitoring; ensure `/health` and `/ready` are wired into your orchestrator.
- Consider rate limiting at the proxy to protect against abuse.
- Tune `SFD_MAX_UPLOAD_BYTES` to control allowed file sizes.

## Rolling updates & backups

- Back up Postgres regularly. Files are stored in MinIO; consider object storage replication or snapshot strategies depending on your provider.
- For upgrades, drain traffic from the instance, perform a rolling deploy, and verify `/ready` before reintroducing traffic.

If you'd like, I can add a sample `caddy` or `nginx` configuration snippet and a systemd unit for running the service directly on a VM.