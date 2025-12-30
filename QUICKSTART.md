# 🚀 Quick Start Guide - Production Features Enabled

## Services Running

All services are now running with production monitoring enabled:

| Service | URL | Status | Credentials |
|---------|-----|--------|-------------|
| **Secure File Drop** | http://localhost:8080 | ✅ Running | admin / admin |
| **Prometheus** | http://localhost:9090 | ✅ Running | No auth |
| **Grafana** | http://localhost:3000 | ✅ Running | admin / admin123 |
| **MinIO Console** | http://localhost:9001 | ✅ Running | minioadmin / CHANGE_THIS_LATER |

## ✅ Production Features Enabled

### 1. JSON Logging
- **Status**: Enabled (`SFD_LOG_FORMAT=json`)
- **Level**: info
- **Environment**: production
- **View logs**: `docker-compose logs -f backend | python3 -m json.tool`

### 2. Automated Backups
- **Status**: ✅ Working (first backup completed!)
- **Schedule**: Every 24 hours
- **Location**: `/var/backups/sfd` (inside container)
- **Compression**: Enabled (gzip)
- **Retention**: 7 days
- **View backups**: `docker exec sfd_backend ls -lh /var/backups/sfd/`

### 3. Prometheus Metrics
- **Status**: ✅ Collecting metrics
- **Metrics endpoint**: http://localhost:8080/metrics/prometheus
- **Available metrics**:
  - `sfd_http_requests_total` - Total HTTP requests
  - `sfd_uploads_total` - File uploads
  - `sfd_downloads_total` - File downloads
  - `sfd_storage_bytes` - Storage usage
  - `sfd_login_success_total` / `sfd_login_failures_total` - Authentication metrics
  - `sfd_uptime_seconds` - Application uptime

### 4. Grafana Dashboards
- **Status**: ✅ Auto-provisioned
- **Dashboard**: "Secure File Drop - Overview"
- **Access**: http://localhost:3000 → Login → Dashboards

## 📊 View Monitoring Dashboard

1. Open Grafana: **http://localhost:3000**
2. Login:
   - Username: `admin`
   - Password: `admin123`
3. Navigate to: **Dashboards** → **Secure File Drop - Overview**
4. View real-time metrics:
   - Request rates and latency percentiles (p50, p95, p99)
   - Upload/download activity
   - Storage usage trends
   - Authentication events
   - System uptime

## 🔍 Check Metrics Directly

### Backend Metrics
```bash
curl http://localhost:8080/metrics/prometheus
```

### Prometheus Targets (should show "up")
```bash
curl http://localhost:9090/api/v1/targets | python3 -m json.tool
```

### View Backups
```bash
docker exec sfd_backend ls -lh /var/backups/sfd/
```

### Live JSON Logs
```bash
docker-compose logs -f backend --tail=50
```

## 🛠️ Useful Commands

### Restart All Services
```bash
docker-compose down && docker-compose up -d
```

### View Service Status
```bash
docker-compose ps
```

### Trigger Manual Actions
```bash
# Test authentication (should increment login metrics)
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}'

# View updated metrics
curl http://localhost:8080/metrics/prometheus | grep login
```

### Check Health
```bash
curl http://localhost:8080/health | python3 -m json.tool
```

## 📈 Monitoring Examples

### View Login Failures in Prometheus
1. Open http://localhost:9090
2. Go to **Graph** tab
3. Enter query: `sfd_login_failures_total`
4. Click **Execute**

### Create Alert for High Error Rate
1. Edit `monitoring/prometheus.yml`
2. Add alert rule:
```yaml
groups:
  - name: sfd_alerts
    rules:
      - alert: HighErrorRate
        expr: rate(sfd_http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 5m
```

## 🔐 Security Notes

### Change Default Passwords
```bash
# Edit .env file
nano .env

# Update:
# - GRAFANA_ADMIN_PASSWORD
# - POSTGRES_PASSWORD  
# - MINIO_ROOT_PASSWORD

# Restart services
docker-compose down && docker-compose up -d
```

### Login to Application
- **URL**: http://localhost:8080
- **Username**: admin
- **Password**: admin (bcrypt hash configured)

## 🐛 Troubleshooting

### Prometheus Not Scraping Backend
```bash
# Check backend metrics endpoint
curl http://localhost:8080/metrics/prometheus

# Check Prometheus targets
curl http://localhost:9090/api/v1/targets
```

### Grafana Dashboard Empty
1. Verify time range (default: Last 1 hour)
2. Generate some traffic (upload files, login, etc.)
3. Wait 10-15 seconds for metrics to populate

### Backups Not Running
```bash
# Check backup logs
docker-compose logs backend | grep backup

# View backup files
docker exec sfd_backend ls -lh /var/backups/sfd/

# Check next backup time (every 24h from first backup)
```

### View Detailed Logs
```bash
# All backend logs
docker-compose logs backend --tail=100

# Follow logs in real-time
docker-compose logs -f backend

# Filter for errors only
docker-compose logs backend | grep -i error
```

## 📚 Documentation

- **Complete Features**: [docs/FEATURES.md](docs/FEATURES.md)
- **Production Guide**: [docs/PRODUCTION_DEPLOYMENT.md](docs/PRODUCTION_DEPLOYMENT.md)
- **Production Enhancements**: [docs/PRODUCTION_ENHANCEMENTS.md](docs/PRODUCTION_ENHANCEMENTS.md)
- **Monitoring Setup**: [monitoring/README.md](monitoring/README.md)

## ✨ Next Steps

1. **Explore Grafana Dashboard**: View real-time metrics at http://localhost:3000
2. **Generate Activity**: Upload files, test downloads to populate metrics
3. **Check Backups**: Verify automated backups in `/var/backups/sfd`
4. **Review Logs**: See structured JSON logs in action
5. **Customize**: Add custom Grafana dashboards and Prometheus alerts

---

**All production features are now enabled and working!** 🎉
