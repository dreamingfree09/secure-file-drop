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
  - DATABASE_URL - PostgreSQL connection string
  - SFD_S3_ENDPOINT, SFD_S3_ACCESS_KEY, SFD_S3_SECRET_KEY, SFD_BUCKET
  - SFD_SMTP_HOST, SFD_SMTP_PORT, SFD_SMTP_USER, SFD_SMTP_PASS - Email notifications (optional)
  - SFD_SMTP_FROM - Sender email address
  - SFD_PUBLIC_BASE_URL - Base URL for public links (e.g., https://files.example.com)
- Rotate secrets periodically and keep a secure audit trail for changes.
- See `docs/EMAIL_NOTIFICATIONS.md` for SMTP setup details.

## Production considerations

### Configuration Validation
- All configuration is validated at startup with detailed error messages
- Required: `DATABASE_URL`, `SFD_SESSION_SECRET`, `SFD_ADMIN_PASS` (bcrypt hash)
- The server will refuse to start with invalid or missing critical configuration
- See [docs/PRODUCTION_ENHANCEMENTS.md](PRODUCTION_ENHANCEMENTS.md) for details

### Security & Rate Limiting
- Make MinIO and Postgres accessible only to the backend service (private network)
- Per-endpoint rate limiting automatically protects against abuse:
  - Auth endpoints: 10 req/min (brute-force protection)
  - Uploads: 20/hour, Downloads: 100/hour
  - Admin: 50 req/min, API: 300 req/min
- Account lockout after 5 failed login attempts (15min lock)
- CSRF protection and security headers enabled by default

### Monitoring & Observability
- Use logging collection and monitoring; ensure `/health` and `/ready` are wired into your orchestrator
- Prometheus metrics available at `/metrics/prometheus` for scraping
- Structured JSON logging enabled with `SFD_LOG_FORMAT=json`
- Request tracing with correlation IDs for debugging
- Circuit breakers protect against cascading failures (database, MinIO, SMTP)

### Storage & Backups
- Tune `SFD_MAX_UPLOAD_BYTES` to control allowed file sizes (default: 50GB)
- Configure automated database backups with `SFD_BACKUP_ENABLED=true`
- Monitor storage quotas - default is 10GB per user (configurable via admin panel)
- Set up automatic file cleanup for expired files
- Resumable uploads supported via TUS protocol at `/upload/resumable`

### Email Notifications
- Configure SMTP for email notifications (registration verification, password resets, file notifications)
- Security event notifications: failed logins, account lockouts, password changes
- See [docs/EMAIL_NOTIFICATIONS.md](EMAIL_NOTIFICATIONS.md) for SMTP setup details

### Performance Optimization
- Database connection pooling optimized (25 max, 5 idle, 5min lifetime)
- HTTP response compression (gzip) enabled automatically
- Streaming uploads/downloads for memory efficiency

## Rolling updates & backups

- Back up Postgres regularly. Files are stored in MinIO; consider object storage replication or snapshot strategies depending on your provider.
- For upgrades, drain traffic from the instance, perform a rolling deploy, and verify `/ready` before reintroducing traffic.

## Reverse proxy examples

### Caddy

```
files.example.com {
  encode zstd gzip
  reverse_proxy localhost:8080
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    X-Content-Type-Options "nosniff"
    X-Frame-Options "DENY"
    Referrer-Policy "no-referrer"
  }
}
```

Set `SFD_PUBLIC_BASE_URL=https://files.example.com`.

### Nginx

```
server {
  listen 443 ssl;
  server_name files.example.com;

  ssl_certificate     /etc/ssl/certs/fullchain.pem;
  ssl_certificate_key /etc/ssl/private/privkey.pem;

  add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
  add_header X-Content-Type-Options nosniff;
  add_header X-Frame-Options DENY;
  add_header Referrer-Policy no-referrer;

  location / {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}
```

Set `SFD_PUBLIC_BASE_URL=https://files.example.com`.

### Traefik (Docker Compose labels)

The following example shows Traefik terminating TLS, forwarding traffic to the backend, and applying security headers and rate limits via middlewares. Adjust names as needed.

```
services:
  traefik:
    image: traefik:3.0
    command:
      - --api.dashboard=true
      - --providers.docker=true
      - --entrypoints.web.address=:80
      - --entrypoints.websecure.address=:443
      - --certificatesresolvers.le.acme.tlschallenge=true
      - --certificatesresolvers.le.acme.email=admin@example.com
      - --certificatesresolvers.le.acme.storage=/letsencrypt/acme.json
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - traefik_letsencrypt:/letsencrypt

  backend:
    image: your-registry/sfd-backend:latest
    labels:
      - traefik.enable=true
      - traefik.http.routers.sfd.rule=Host(`files.example.com`)
      - traefik.http.routers.sfd.entrypoints=websecure
      - traefik.http.routers.sfd.tls.certresolver=le
      - traefik.http.services.sfd.loadbalancer.server.port=8080

      # Security headers
      - traefik.http.middlewares.sfd-headers.headers.stsSeconds=31536000
      - traefik.http.middlewares.sfd-headers.headers.stsIncludeSubdomains=true
      - traefik.http.middlewares.sfd-headers.headers.stsPreload=true
      - traefik.http.middlewares.sfd-headers.headers.browserXssFilter=true
      - traefik.http.middlewares.sfd-headers.headers.contentTypeNosniff=true
      - traefik.http.middlewares.sfd-headers.headers.frameDeny=true
      - traefik.http.middlewares.sfd-headers.headers.referrerPolicy=no-referrer

      # Global rate limit (average requests per second)
      - traefik.http.middlewares.sfd-ratelimit.ratelimit.average=100
      - traefik.http.middlewares.sfd-ratelimit.ratelimit.burst=50

      # Apply middlewares
      - traefik.http.routers.sfd.middlewares=sfd-headers,sfd-ratelimit

      # Optional: stricter limits for upload endpoints
      - traefik.http.routers.sfd-upload.rule=Host(`files.example.com`) && PathPrefix(`/upload`)
      - traefik.http.routers.sfd-upload.entrypoints=websecure
      - traefik.http.routers.sfd-upload.tls.certresolver=le
      - traefik.http.services.sfd-upload.loadbalancer.server.port=8080
      - traefik.http.routers.sfd-upload.middlewares=sfd-headers,sfd-upload-ratelimit
      - traefik.http.middlewares.sfd-upload-ratelimit.ratelimit.average=30
      - traefik.http.middlewares.sfd-upload-ratelimit.ratelimit.burst=15

      # Optional: stricter limits for auth endpoints
      - traefik.http.routers.sfd-login.rule=Host(`files.example.com`) && Path(`/login`)
      - traefik.http.routers.sfd-login.entrypoints=websecure
      - traefik.http.routers.sfd-login.tls.certresolver=le
      - traefik.http.services.sfd-login.loadbalancer.server.port=8080
      - traefik.http.routers.sfd-login.middlewares=sfd-headers,sfd-login-ratelimit
      - traefik.http.middlewares.sfd-login-ratelimit.ratelimit.average=20
      - traefik.http.middlewares.sfd-login-ratelimit.ratelimit.burst=10

      - traefik.http.routers.sfd-register.rule=Host(`files.example.com`) && Path(`/register`)
      - traefik.http.routers.sfd-register.entrypoints=websecure
      - traefik.http.routers.sfd-register.tls.certresolver=le
      - traefik.http.services.sfd-register.loadbalancer.server.port=8080
      - traefik.http.routers.sfd-register.middlewares=sfd-headers,sfd-register-ratelimit
      - traefik.http.middlewares.sfd-register-ratelimit.ratelimit.average=10
      - traefik.http.middlewares.sfd-register-ratelimit.ratelimit.burst=5

      # Optional: lighter limits for quota endpoint
      - traefik.http.routers.sfd-quota.rule=Host(`files.example.com`) && Path(`/quota`)
      - traefik.http.routers.sfd-quota.entrypoints=websecure
      - traefik.http.routers.sfd-quota.tls.certresolver=le
      - traefik.http.services.sfd-quota.loadbalancer.server.port=8080
      - traefik.http.middlewares.sfd-quota-ratelimit.ratelimit.average=10
      - traefik.http.middlewares.sfd-quota-ratelimit.ratelimit.burst=10
      - traefik.http.routers.sfd-quota.middlewares=sfd-headers,sfd-quota-ratelimit

      # Optional: stricter limits for password reset endpoints
      - traefik.http.routers.sfd-reset-request.rule=Host(`files.example.com`) && Path(`/reset-password-request`)
      - traefik.http.routers.sfd-reset-request.entrypoints=websecure
      - traefik.http.routers.sfd-reset-request.tls.certresolver=le
      - traefik.http.services.sfd-reset-request.loadbalancer.server.port=8080
      - traefik.http.routers.sfd-reset-request.middlewares=sfd-headers,sfd-resetreq-ratelimit
      - traefik.http.middlewares.sfd-resetreq-ratelimit.ratelimit.average=10
      - traefik.http.middlewares.sfd-resetreq-ratelimit.ratelimit.burst=5

      - traefik.http.routers.sfd-reset.rule=Host(`files.example.com`) && Path(`/reset-password`)
      - traefik.http.routers.sfd-reset.entrypoints=websecure
      - traefik.http.routers.sfd-reset.tls.certresolver=le
      - traefik.http.services.sfd-reset.loadbalancer.server.port=8080
      - traefik.http.routers.sfd-reset.middlewares=sfd-headers,sfd-reset-ratelimit
      - traefik.http.middlewares.sfd-reset-ratelimit.ratelimit.average=10
      - traefik.http.middlewares.sfd-reset-ratelimit.ratelimit.burst=5

volumes:
  traefik_letsencrypt:
```

Notes:
- Traefik forwards `X-Forwarded-Proto`/`Host` automatically; ensure `SFD_PUBLIC_BASE_URL=https://files.example.com` so absolute links are correct.
- Tune `ratelimit.average`/`burst` per your traffic profile; you can define additional routers for other sensitive paths (e.g., `/login`, `/register`).
 - The `/quota` endpoint is polled by the UI; apply lighter limits than uploads but enough to prevent hammering.
 - Ensure security headers (HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy) are enabled at the proxy; see [docs/SECURITY.md](SECURITY.md).

### Nginx rate limiting (example)

Use `limit_req_zone` and `limit_req` to apply simple per-IP rate limits. Place these in your Nginx config and adjust limits for sensitive endpoints.

```
http {
  # Define a shared memory zone keyed by client IP
  limit_req_zone $binary_remote_addr zone=sfd_zone:10m rate=100r/s;
  # Quota endpoint can use a lighter rate to avoid hammering
  limit_req_zone $binary_remote_addr zone=sfd_quota_zone:10m rate=10r/s;

  server {
    listen 443 ssl;
    server_name files.example.com;

    # ... SSL cert config and headers ...

    location /upload {
      limit_req zone=sfd_zone burst=15 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    location = /login {
      limit_req zone=sfd_zone burst=10 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    location = /register {
      limit_req zone=sfd_zone burst=5 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Password reset request
    location = /reset-password-request {
      limit_req zone=sfd_zone burst=5 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Password reset
    location = /reset-password {
      limit_req zone=sfd_zone burst=5 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Quota endpoint
    location = /quota {
      limit_req zone=sfd_quota_zone burst=10 nodelay;
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Default location
    location / {
      proxy_pass http://localhost:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
    }
  }
}
```

### Quota endpoint & frontend banner

- The UI polls `/quota` to display storage usage and limits. If the endpoint is unreachable or rate-limited, a non-blocking banner appears with a brief message and an ℹ️ tooltip.
- Users can dismiss the banner; this dismissal is persisted locally and the banner stays hidden until a successful quota load occurs (then the dismissal resets).
- Recommendation: keep `/quota` accessible with modest rate limits (e.g., 10 r/s per IP) and avoid aggressive caching that could leak user-specific data.