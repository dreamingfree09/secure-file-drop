# Production Enhancements - Environment Variables

This document describes the new environment variables added for production features.

## Configuration Validation

The application now validates all configuration at startup and will exit with detailed error messages if any required variables are missing or invalid.

## Account Lockout

Account lockout is automatically enabled with sensible defaults:
- **Lockout Threshold**: 5 failed login attempts
- **Lockout Duration**: 15 minutes
- **Attempt Window**: 10 minutes (rolling window)

No configuration required - works out of the box.

## Structured JSON Logging

Enable structured JSON logging for production environments:

```bash
# Enable JSON logging (recommended for production)
SFD_LOG_FORMAT=json

# Set log level (debug, info, warn, error)
SFD_LOG_LEVEL=info

# Automatically enabled when SFD_ENV=production
SFD_ENV=production
```

## Prometheus Metrics

Prometheus metrics are automatically exposed at `/metrics/prometheus`:

```bash
# No configuration required
# Scrape endpoint: http://your-server:8080/metrics/prometheus
```

Metrics include:
- `sfd_requests_total` - Total requests by method, path, status
- `sfd_uploads_total` - Total file uploads
- `sfd_downloads_total` - Total file downloads
- `sfd_storage_bytes` - Current storage usage
- `sfd_login_success_total` - Successful logins
- `sfd_login_failed_total` - Failed login attempts
- `sfd_uptime_seconds` - Server uptime

## Automated Database Backups

Configure automated PostgreSQL backups:

```bash
# Enable automated backups
SFD_BACKUP_ENABLED=true

# Backup interval (e.g., 24h for daily, 12h for twice daily)
SFD_BACKUP_INTERVAL=24h

# Retention period in days
SFD_BACKUP_RETENTION_DAYS=7

# Directory to store backup files
SFD_BACKUP_DIR=/var/backups/sfd

# Enable gzip compression (recommended)
SFD_BACKUP_COMPRESSION=true

# Upload backups to S3/MinIO (optional)
SFD_BACKUP_S3_ENABLED=true
SFD_BACKUP_S3_BUCKET=backups
SFD_BACKUP_S3_PREFIX=sfd-backups

# Email notifications
SFD_BACKUP_NOTIFY_FAILURE=true
SFD_BACKUP_NOTIFY_SUCCESS=false
```

## Request Tracing

Request tracing with correlation IDs is automatically enabled:
- Accepts `X-Correlation-ID` or `X-Request-ID` headers from clients
- Generates unique IDs if not provided
- Includes correlation IDs in all log entries
- Returns `X-Correlation-ID` header in responses

No configuration required.

## Resumable Uploads (TUS Protocol)

Resumable upload endpoints are automatically available:
- `POST /upload/resumable` - Create upload session
- `PATCH /upload/resumable/{id}` - Upload chunk
- `HEAD /upload/resumable/{id}` - Check upload status

Database migration `000010_add_resumable_uploads` creates the required table.

No additional configuration required.

## Database Connection Pooling

Connection pool is automatically optimized with these defaults:
- **MaxOpenConns**: 25 (maximum concurrent connections)
- **MaxIdleConns**: 5 (idle connections to keep alive)
- **ConnMaxLifetime**: 5 minutes (max connection age)
- **ConnMaxIdleTime**: 2 minutes (max idle time before closing)

No configuration required - optimized for production use.

## Per-Endpoint Rate Limiting

Rate limiting is automatically applied to all endpoints with specialized limits:

| Endpoint Category | Limit | Window | Purpose |
|------------------|-------|---------|---------|
| Auth (`/login`, `/register`) | 10 | 1 minute | Prevent brute force |
| Upload (`/upload*`) | 20 | 1 hour | Prevent abuse |
| Download (`/download`, `/links`) | 100 | 1 hour | Fair usage |
| Admin (`/admin/*`) | 50 | 1 minute | Protect admin endpoints |
| General API | 300 | 1 minute | Overall protection |

No configuration required.

## HTTP Compression

Gzip compression is automatically enabled for:
- JSON API responses
- HTML pages
- Text-based content

Binary files and downloads are excluded automatically.

No configuration required.

## Circuit Breakers

Circuit breakers are automatically enabled for external dependencies:

| Service | Failure Threshold | Timeout |
|---------|------------------|---------|
| Database | 5 failures | 30 seconds |
| MinIO/S3 | 5 failures | 30 seconds |
| SMTP | 3 failures | 60 seconds |

No configuration required - automatic fail-fast protection.

## Security Event Notifications

Email notifications for security events (requires SMTP configuration):
- Failed login attempts
- Account lockouts
- Password changes
- Multiple authentication failures

Uses existing SMTP configuration:
```bash
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=notifications@example.com
SMTP_PASS=your-password
SMTP_FROM=noreply@example.com
```

## Migration Guide

### Upgrading from Previous Versions

1. **Backup your database** before upgrading:
   ```bash
   pg_dump -h localhost -U postgres secure_file_drop > backup.sql
   ```

2. **Pull the latest version**:
   ```bash
   git pull origin main
   docker compose pull
   ```

3. **Update environment variables**:
   - Add `SFD_LOG_FORMAT=json` for production
   - Add `SFD_BACKUP_ENABLED=true` for automated backups
   - Review new optional variables above

4. **Restart services**:
   ```bash
   docker compose down
   docker compose up -d
   ```

5. **Verify migrations**:
   ```bash
   # Check logs for successful migration
   docker compose logs backend | grep migration
   
   # Verify new tables exist
   docker compose exec postgres psql -U postgres -d secure_file_drop -c "\dt"
   # Should show: resumable_uploads, audit_logs
   ```

6. **Test health check**:
   ```bash
   curl http://localhost:8080/ready
   # Should return status 200 with component health
   ```

### New Database Tables

The following tables are automatically created during migration:

- **`resumable_uploads`** (migration 000010): Tracks TUS protocol upload sessions
- **`audit_logs`** (migration 000011): Comprehensive audit trail with JSONB metadata

### Monitoring Recommendations

For production deployments, configure:

1. **Prometheus scraping**:
   ```yaml
   scrape_configs:
     - job_name: 'secure-file-drop'
       static_configs:
         - targets: ['your-server:8080']
       metrics_path: /metrics/prometheus
   ```

2. **Log aggregation**:
   - Ship JSON logs to your log aggregator (ELK, Loki, etc.)
   - Include correlation_id field for request tracing

3. **Alerting**:
   - Monitor `/ready` endpoint for service health
   - Alert on backup failures (check logs)
   - Alert on high error rates in metrics

### Configuration Validation

The application now validates all configuration on startup. Common validation errors:

- **SFD_SESSION_SECRET**: Must be at least 32 characters
- **SFD_ADMIN_PASS**: Must be a bcrypt hash (60 chars, starts with $2a$/$2b$/$2y$)
- **DATABASE_URL**: Must be a valid PostgreSQL connection string
- **SFD_BASE_URL**: Must be a valid HTTP/HTTPS URL
- **SMTP_PORT**: Must be a valid port number (1-65535)

Generate a bcrypt hash for admin password:
```bash
htpasswd -bnBC 12 "" yourpassword | tr -d ':'
```

### Troubleshooting

**Startup fails with "configuration validation failed"**:
- Check logs for specific validation errors
- Verify all required environment variables are set
- Ensure SFD_ADMIN_PASS is a bcrypt hash

**Backups not running**:
- Verify `SFD_BACKUP_ENABLED=true`
- Check logs for backup scheduler messages
- Ensure backup directory is writable
- Verify `pg_dump` is available in container

**Circuit breaker open**:
- Check `/metrics/prometheus` for circuit breaker stats
- Verify database/MinIO/SMTP connectivity
- Circuit will auto-recover after timeout period

**Rate limit exceeded**:
- Check `X-RateLimit-Limit-Type` header in response
- Adjust client request rate to stay within limits
- Contact admin if limits are too restrictive
