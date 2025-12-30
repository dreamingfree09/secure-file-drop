# Production Deployment Guide

## Table of Contents
- [Deploying to Proxmox/Remote Server](#deploying-to-proxmoxremote-server)
- [Production Features Enabled](#production-features-enabled)
- [Getting Started](#getting-started)
- [Production Best Practices](#production-best-practices)

---

## Deploying to Proxmox/Remote Server

### Complete Deployment Instructions

This section provides step-by-step instructions for deploying Secure File Drop to a Proxmox hypervisor or any remote server.

### Prerequisites

**On your Proxmox server:**
- Ubuntu 22.04 LTS or newer (recommended) or Debian 12
- Minimum 2 CPU cores, 4GB RAM, 20GB disk
- Root or sudo access
- Internet connection

### Step 1: Prepare Proxmox VM/Container

#### Option A: Create LXC Container (Recommended - Lightweight)
```bash
# From Proxmox web UI:
# 1. Create → CT (Container)
# 2. Template: ubuntu-22.04-standard
# 3. Resources: 2 cores, 4GB RAM, 20GB disk
# 4. Network: Bridge, DHCP or static IP
# 5. Start the container
```

#### Option B: Create VM
```bash
# From Proxmox web UI:
# 1. Create → VM
# 2. ISO: Ubuntu 22.04 Server
# 3. Resources: 2 cores, 4GB RAM, 20GB disk
# 4. Install Ubuntu with Docker option
```

### Step 2: Initial Server Setup

SSH into your Proxmox container/VM:

```bash
# From your local machine
ssh root@YOUR_PROXMOX_IP

# Update system
apt update && apt upgrade -y

# Install required packages
apt install -y git curl wget nano htop

# Create app user (recommended for security)
useradd -m -s /bin/bash sfd
usermod -aG sudo sfd
```

### Step 3: Install Docker & Docker Compose

```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Add user to docker group (if using non-root user)
usermod -aG docker sfd

# Install Docker Compose
apt install -y docker-compose-plugin

# Verify installation
docker --version
docker compose version
```

### Step 4: Transfer Project to Server

#### Method 1: Clone from GitHub (Recommended)
```bash
# Switch to app user
su - sfd

# Clone repository
cd ~
git clone https://github.com/dreamingfree09/secure-file-drop.git
cd secure-file-drop

# Switch to production branch
git checkout feature/v2-enhancements
```

#### Method 2: Transfer via SCP (from your local machine)
```bash
# From your LOCAL machine (where you have the project)
cd "/home/dreamingfree09/Secure File Drop"
tar czf secure-file-drop.tar.gz .

# Transfer to server
scp secure-file-drop.tar.gz sfd@YOUR_PROXMOX_IP:~/

# On the server
ssh sfd@YOUR_PROXMOX_IP
tar xzf secure-file-drop.tar.gz
cd secure-file-drop
```

### Step 5: Configure Environment

```bash
# Copy environment template
cp .env.example .env

# Generate secure secrets
echo "SFD_SESSION_SECRET=$(openssl rand -hex 32)" >> .env.tmp
echo "SFD_DOWNLOAD_SECRET=$(openssl rand -hex 32)" >> .env.tmp
echo "POSTGRES_PASSWORD=$(openssl rand -base64 24)" >> .env.tmp
echo "MINIO_ROOT_PASSWORD=$(openssl rand -base64 24)" >> .env.tmp
echo "GRAFANA_ADMIN_PASSWORD=$(openssl rand -base64 16)" >> .env.tmp

# Generate bcrypt hash for admin password
ADMIN_HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'YOUR_ADMIN_PASSWORD', bcrypt.gensalt(12)).decode())")
echo "SFD_ADMIN_PASS=$ADMIN_HASH" >> .env.tmp

# Edit .env with generated values
nano .env

# Update these values in .env:
# - POSTGRES_PASSWORD (use generated value)
# - MINIO_ROOT_PASSWORD (use generated value)
# - SFD_ADMIN_PASS (use generated bcrypt hash)
# - SFD_SESSION_SECRET (use generated value)
# - SFD_DOWNLOAD_SECRET (use generated value)
# - GRAFANA_ADMIN_PASSWORD (use generated value)
# - SFD_PUBLIC_BASE_URL (set to your domain or IP)
# - DATABASE_URL (update with POSTGRES_PASSWORD)

# Example .env configuration:
# POSTGRES_PASSWORD=Xy8zK9pLm3nQ2wR5vT7yU1aB4cD6eF8g
# MINIO_ROOT_PASSWORD=Ab3dEf7gHi9jKl2mNp5qRs8tUv1wXy4z
# SFD_ADMIN_PASS=$2b$12$abc123...xyz789
# SFD_PUBLIC_BASE_URL=https://files.yourdomain.com
```

### Step 6: Configure Firewall (Optional but Recommended)

```bash
# Install UFW (Uncomplicated Firewall)
apt install -y ufw

# Allow SSH (important - don't lock yourself out!)
ufw allow 22/tcp

# Allow HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp

# Allow application ports (adjust as needed)
ufw allow 8080/tcp   # Direct backend access
ufw allow 9090/tcp   # Prometheus (consider restricting to internal network)
ufw allow 3000/tcp   # Grafana (consider restricting to internal network)

# Enable firewall
ufw --force enable

# Check status
ufw status
```

### Step 7: Set Up HTTPS with Caddy (Recommended)

```bash
# Install Caddy
apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
apt update
apt install -y caddy

# Create Caddyfile
nano /etc/caddy/Caddyfile

# Add this configuration:
# files.yourdomain.com {
#     reverse_proxy localhost:8080
#     tls your-email@example.com
# }
# 
# grafana.yourdomain.com {
#     reverse_proxy localhost:3000
#     tls your-email@example.com
# }

# Reload Caddy
systemctl reload caddy
systemctl enable caddy
```

### Step 8: Deploy Application

```bash
# Navigate to project directory
cd ~/secure-file-drop

# Build and start services
docker compose up -d

# Wait for services to start (30-60 seconds)
sleep 30

# Check service status
docker compose ps

# View logs
docker compose logs -f backend

# Verify health
curl http://localhost:8080/health
```

### Step 9: Verify Deployment

```bash
# Check all services are running
docker compose ps

# Test backend health
curl http://localhost:8080/ready

# Test Prometheus metrics
curl http://localhost:8080/metrics/prometheus | head -20

# Check backup was created
docker exec sfd_backend ls -lh /var/backups/sfd/

# View logs
docker compose logs backend | tail -50
```

### Step 10: Access Your Application

**Without HTTPS (local testing):**
- Application: http://YOUR_SERVER_IP:8080
- Grafana: http://YOUR_SERVER_IP:3000
- Prometheus: http://YOUR_SERVER_IP:9090

**With HTTPS (via Caddy):**
- Application: https://files.yourdomain.com
- Grafana: https://grafana.yourdomain.com

### Troubleshooting Deployment

#### Container won't start
```bash
# Check logs
docker compose logs backend

# Check if ports are available
netstat -tlnp | grep -E '(8080|9090|3000)'

# Restart services
docker compose down
docker compose up -d
```

#### Database connection errors
```bash
# Verify PostgreSQL is running
docker compose ps postgres

# Check DATABASE_URL matches POSTGRES_PASSWORD in .env
grep -E "(DATABASE_URL|POSTGRES_PASSWORD)" .env

# Restart postgres
docker compose restart postgres
```

#### Permission errors
```bash
# Fix ownership
chown -R sfd:sfd ~/secure-file-drop

# Fix docker permissions
usermod -aG docker sfd
```

### Updating Your Deployment

```bash
# Pull latest changes
cd ~/secure-file-drop
git pull origin feature/v2-enhancements

# Rebuild containers
docker compose build backend

# Restart services (with zero downtime)
docker compose up -d

# Check logs
docker compose logs -f backend
```

### Backup & Restore

#### Manual Backup
```bash
# Backup database
docker exec sfd_postgres pg_dump -U sfd sfd > backup-$(date +%Y%m%d).sql

# Backup .env file
cp .env .env.backup-$(date +%Y%m%d)

# Backup Docker volumes
docker run --rm -v securefiledrop_sfd_pgdata:/data -v $(pwd):/backup ubuntu tar czf /backup/pgdata-backup-$(date +%Y%m%d).tar.gz /data
```

#### Restore from Backup
```bash
# Restore database
cat backup-20251230.sql | docker exec -i sfd_postgres psql -U sfd -d sfd

# Restore .env
cp .env.backup-20251230 .env

# Restart services
docker compose down && docker compose up -d
```

---

## Production Features Enabled

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
