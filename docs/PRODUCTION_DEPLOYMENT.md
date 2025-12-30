# Production Deployment Guide

## Optional Configuration Enabled

Your Secure File Drop instance is configured with the following production features:

### ✅ Structured JSON Logging
- **Enabled**: `SFD_LOG_FORMAT=json`
- **Level**: `info` (configurable to debug/warn/error)
- **Environment**: `production`
- **Benefits**: Machine-parsable logs for centralized logging systems (ELK, Loki, etc.)

### ✅ Automated Database Backups
- **Enabled**: `SFD_BACKUP_ENABLED=true`
- **Schedule**: Daily (24h interval)
- **Retention**: 7 days
- **Compression**: Gzip enabled
- **S3 Upload**: Enabled to MinIO/S3 bucket `sfd-backups`
- **Notifications**: Email alerts on backup failures
- **Location**: `/var/backups/sfd` (persistent Docker volume)

### ✅ Monitoring Stack (Prometheus + Grafana)
- **Prometheus**: Metrics collection at http://localhost:9090
- **Grafana**: Visualization at http://localhost:3000
- **Dashboards**: Auto-provisioned "Secure File Drop - Overview" dashboard
- **Metrics**: 
  - HTTP requests and durations (p50, p95, p99)
  - Upload/download rates and totals
  - Storage usage
  - Authentication metrics (login attempts, failures, active sessions)
  - System uptime
  - Circuit breaker stats

## Getting Started

### 1. Initial Setup

```bash
# Copy environment template
cp .env.example .env

# Generate secure secrets
openssl rand -hex 32  # For SFD_SESSION_SECRET and SFD_DOWNLOAD_SECRET
openssl rand -base64 24  # For passwords
htpasswd -bnBC 12 "" yourpassword | tr -d ':'  # For SFD_ADMIN_PASS (bcrypt)

# Edit .env with your secrets
nano .env
```

### 2. Start Services

```bash
# Start all services (backend, postgres, minio, prometheus, grafana)
docker-compose up -d

# Check logs
docker-compose logs -f backend

# Verify health
curl http://localhost:8080/ready
curl http://localhost:8080/health
```

### 3. Access Services

| Service | URL | Credentials |
|---------|-----|-------------|
| **Application** | http://localhost:8080 | From `.env`: `SFD_ADMIN_USER` / `SFD_ADMIN_PASS` |
| **Prometheus** | http://localhost:9090 | None (configure auth in production) |
| **Grafana** | http://localhost:3000 | admin / `GRAFANA_ADMIN_PASSWORD` from `.env` |
| **MinIO Console** | http://localhost:9001 | `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` |

### 4. Verify Backups

```bash
# Check backup directory
docker exec sfd_backend ls -lh /var/backups/sfd/

# Check S3/MinIO bucket
# Visit http://localhost:9001 → Browse → sfd-backups bucket

# Trigger manual backup (if needed)
# Backups run automatically every 24h
```

### 5. Monitor Metrics

1. Open Grafana: http://localhost:3000
2. Login with admin / your_password
3. Navigate to Dashboards → Secure File Drop - Overview
4. View real-time metrics:
   - Request rates and latencies
   - Upload/download activity
   - Storage usage trends
   - Authentication events

## Production Best Practices

### Security Hardening

1. **Change default Grafana password** immediately after first login
2. **Use HTTPS** in production (configure reverse proxy like Caddy/Nginx)
3. **Restrict metrics access** - add authentication to Prometheus endpoint
4. **Secure SMTP credentials** - use app-specific passwords
5. **Regular bcrypt hashes** - update `SFD_ADMIN_PASS` periodically

### Backup Management

```bash
# Manual backup restoration
docker exec -i sfd_postgres psql -U sfd -d sfd < backup.sql

# View backup logs
docker-compose logs backend | grep -i backup

# Check S3 upload status
docker exec sfd_backend ls -lh /var/backups/sfd/
```

### Monitoring Configuration

**Alert on high error rates**:
1. Create Prometheus alert rules in `monitoring/prometheus.yml`
2. Configure Alertmanager for notifications

**Example alert rule**:
```yaml
groups:
  - name: sfd_alerts
    rules:
      - alert: HighErrorRate
        expr: rate(sfd_http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "High error rate detected"
```

### Log Management

**View structured logs**:
```bash
docker-compose logs backend | jq .
```

**Filter by correlation ID**:
```bash
docker-compose logs backend | jq 'select(.correlation_id=="abc-123")'
```

**Send logs to external system**:
- Use Docker logging driver (Fluentd, Loki, etc.)
- Configure in `docker-compose.yml`:
  ```yaml
  backend:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
  ```

## Troubleshooting

### Backups Not Running

```bash
# Check backup service status
docker-compose logs backend | grep -i "backup"

# Verify S3 bucket exists
docker exec sfd_backend sh -c 'mc alias set myminio http://minio:9000 $SFD_S3_ACCESS_KEY $SFD_S3_SECRET_KEY && mc ls myminio/sfd-backups'

# Check environment variables
docker exec sfd_backend env | grep BACKUP
```

### Prometheus Not Scraping Metrics

```bash
# Test metrics endpoint
curl http://localhost:8080/metrics/prometheus

# Check Prometheus targets
# Visit http://localhost:9090/targets
# Ensure "sfd-backend" is UP

# Check Prometheus config
docker exec sfd_prometheus cat /etc/prometheus/prometheus.yml
```

### Grafana Dashboard Empty

1. Verify Prometheus datasource: Configuration → Data sources → Prometheus
2. Check time range (default: Last 1 hour)
3. Generate traffic to populate metrics
4. Verify backend is exposing metrics

## Scaling Recommendations

### High-Traffic Deployments

1. **Increase connection pool** (in `.env`):
   - Default: 25 max connections
   - For high load: Set via environment or code modification

2. **Add more backend replicas**:
   ```yaml
   backend:
     deploy:
       replicas: 3
   ```

3. **Use external PostgreSQL/MinIO**:
   - Managed database services (AWS RDS, GCP Cloud SQL)
   - Scalable object storage (AWS S3, GCP Storage)

4. **Enable caching**:
   - Add Redis for session storage
   - Cache frequently accessed file metadata

### Storage Management

**Monitor disk usage**:
```bash
# Check Docker volumes
docker system df -v

# MinIO storage
docker exec sfd_minio du -sh /data

# Backup storage
docker exec sfd_backend du -sh /var/backups/sfd
```

**Cleanup old backups manually**:
```bash
# Retention policy auto-deletes after 7 days
# Manual cleanup if needed:
docker exec sfd_backend find /var/backups/sfd -name "*.sql.gz" -mtime +7 -delete
```

## Additional Resources

- [Monitoring README](../monitoring/README.md) - Detailed monitoring setup
- [PRODUCTION_ENHANCEMENTS.md](PRODUCTION_ENHANCEMENTS.md) - All production features
- [FEATURES.md](FEATURES.md) - Complete feature list
- [Prometheus Best Practices](https://prometheus.io/docs/practices/)
- [Grafana Documentation](https://grafana.com/docs/)

## Support

For issues or questions:
1. Check application logs: `docker-compose logs backend`
2. Review health endpoint: `curl http://localhost:8080/health`
3. Check Prometheus alerts: http://localhost:9090/alerts
4. View circuit breaker stats in Grafana dashboard
