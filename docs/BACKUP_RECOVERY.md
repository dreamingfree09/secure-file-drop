# Backup & Disaster Recovery Guide

This guide covers backup strategies, procedures, and disaster recovery for Secure File Drop.

## Table of Contents

- [Overview](#overview)
- [Backup Components](#backup-components)
- [Automated Backups](#automated-backups)
- [Manual Backups](#manual-backups)
- [Restore Procedures](#restore-procedures)
- [Disaster Recovery](#disaster-recovery)
- [Testing](#testing)
- [Best Practices](#best-practices)

## Overview

A comprehensive backup strategy for Secure File Drop includes:

1. **PostgreSQL Database**: User accounts, file metadata, sessions, audit logs
2. **MinIO Object Storage**: Uploaded files and generated hashes
3. **Configuration Files**: Environment variables, secrets, Caddy/Nginx configs
4. **Application State**: Sessions, temporary files (optional)

### Backup Schedule Recommendation

| Component | Frequency | Retention | Method |
|-----------|-----------|-----------|--------|
| Database | Hourly | 7 days | Automated (cron) |
| MinIO | Daily | 30 days | Automated (cron) |
| Config | On change | Forever | Version control |
| Full System | Weekly | 4 weeks | Automated (cron) |

## Backup Components

### 1. PostgreSQL Database

**What's Backed Up:**
- User accounts and credentials
- File metadata (names, sizes, hashes)
- Download links and access logs
- Admin audit logs
- Session data

**Backup Methods:**
- `pg_dump`: Logical backups (recommended for small-medium databases)
- `pg_basebackup`: Physical backups (for large databases)
- Continuous archiving with WAL (Write-Ahead Log) shipping

### 2. MinIO Object Storage

**What's Backed Up:**
- Uploaded files in the bucket
- File hashes and metadata
- Object versioning history (if enabled)

**Backup Methods:**
- `mc mirror`: Incremental mirroring to backup location
- Bucket replication: Real-time replication to another MinIO instance
- S3 sync: Sync to AWS S3, Backblaze B2, or other S3-compatible storage

### 3. Configuration Files

**What's Backed Up:**
- `.env` files (sanitized, without secrets)
- `docker-compose.yml`
- Reverse proxy configs (Caddy, Nginx, Traefik)
- Application configs

**Backup Methods:**
- Git repository (recommended)
- Configuration management tools (Ansible, Terraform)
- Manual copies to secure location

## Automated Backups

### Database Backup Script

The included `backup-database.sh` script automates PostgreSQL backups:

```bash
# Basic backup
./scripts/backup-database.sh

# Encrypted backup
./scripts/backup-database.sh --encrypt

# Upload to remote storage
./scripts/backup-database.sh --remote

# Custom retention
./scripts/backup-database.sh --keep 60
```

**Configuration (environment variables):**

```bash
export SFD_DB_HOST=localhost
export SFD_DB_PORT=5432
export SFD_DB_NAME=sfd
export SFD_DB_USER=postgres
export SFD_BACKUP_DIR=/var/backups/sfd
export SFD_BACKUP_RETENTION=30
export SFD_BACKUP_ENCRYPT=true
export SFD_BACKUP_GPG_RECIPIENT=backup@example.com
export SFD_BACKUP_REMOTE=true
export SFD_BACKUP_S3_BUCKET=my-backups
export SFD_BACKUP_S3_PREFIX=sfd/database
```

### MinIO Backup Script

The included `backup-minio.sh` script mirrors MinIO buckets:

```bash
# Basic backup
./scripts/backup-minio.sh

# Custom destination
./scripts/backup-minio.sh --destination s3-backup

# Custom retention
./scripts/backup-minio.sh --keep 60
```

**Prerequisites:**

1. Install MinIO client:
   ```bash
   wget https://dl.min.io/client/mc/release/linux-amd64/mc
   chmod +x mc
   sudo mv mc /usr/local/bin/
   ```

2. Configure backup destination:
   ```bash
   # For S3
   mc alias set backup https://s3.amazonaws.com ACCESS_KEY SECRET_KEY
   
   # For Backblaze B2
   mc alias set backup https://s3.us-west-002.backblazeb2.com ACCESS_KEY SECRET_KEY
   
   # For another MinIO instance
   mc alias set backup https://backup.example.com:9000 ACCESS_KEY SECRET_KEY
   ```

### Cron Schedule

Add to `/etc/crontab` or user crontab:

```bash
# Database backups every 6 hours
0 */6 * * * /opt/sfd/scripts/backup-database.sh --encrypt --remote

# MinIO backups daily at 2 AM
0 2 * * * /opt/sfd/scripts/backup-minio.sh --destination backup

# Full system backup weekly on Sunday at 3 AM
0 3 * * 0 /opt/sfd/scripts/backup-full.sh
```

Or use systemd timers for better logging:

```ini
# /etc/systemd/system/sfd-backup.timer
[Unit]
Description=Secure File Drop Backup Timer
Requires=sfd-backup.service

[Timer]
OnCalendar=daily
OnCalendar=02:00
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/sfd-backup.service
[Unit]
Description=Secure File Drop Backup

[Service]
Type=oneshot
ExecStart=/opt/sfd/scripts/backup-database.sh --encrypt --remote
ExecStart=/opt/sfd/scripts/backup-minio.sh --destination backup
User=sfd
Group=sfd
```

Enable and start:
```bash
sudo systemctl enable sfd-backup.timer
sudo systemctl start sfd-backup.timer
sudo systemctl list-timers
```

## Manual Backups

### On-Demand Database Backup

```bash
# Simple backup
pg_dump -h localhost -U postgres -d sfd > sfd_backup.sql

# Compressed backup
pg_dump -h localhost -U postgres -d sfd | gzip > sfd_backup.sql.gz

# Custom format (faster restore, supports parallel restore)
pg_dump -h localhost -U postgres -d sfd -Fc -f sfd_backup.dump
```

### On-Demand MinIO Backup

```bash
# Mirror entire bucket
mc mirror --preserve sfd-source/sfd-uploads backup/sfd-uploads-backup

# Download all files
mc cp --recursive sfd-source/sfd-uploads /backup/sfd-files/

# Create tarball
mc cp --recursive sfd-source/sfd-uploads /tmp/sfd-backup/
tar czf sfd-uploads-$(date +%Y%m%d).tar.gz -C /tmp sfd-backup/
```

### Configuration Backup

```bash
# Backup all configs
tar czf sfd-config-$(date +%Y%m%d).tar.gz \
    /opt/sfd/.env \
    /opt/sfd/docker-compose.yml \
    /etc/caddy/Caddyfile \
    /etc/nginx/sites-available/sfd

# Or commit to git
cd /opt/sfd
git add .env.example docker-compose.yml
git commit -m "Config snapshot $(date +%Y-%m-%d)"
git push origin config-backups
```

## Restore Procedures

### Database Restore

**Using the restore script:**

```bash
# Interactive restore (with confirmation)
./scripts/restore-database.sh /backups/sfd_backup_20250129.sql.gz

# Decrypt and restore
./scripts/restore-database.sh --decrypt /backups/sfd_backup_20250129.sql.gz.gpg

# Force restore (skip confirmation)
./scripts/restore-database.sh --force /backups/sfd_backup_20250129.sql.gz
```

**Manual restore:**

```bash
# Stop application first
docker-compose down

# Drop and recreate database
psql -U postgres -c "DROP DATABASE sfd;"
psql -U postgres -c "CREATE DATABASE sfd;"

# Restore from backup
gunzip -c sfd_backup.sql.gz | psql -U postgres -d sfd

# Or from custom format
pg_restore -U postgres -d sfd sfd_backup.dump

# Restart application
docker-compose up -d
```

### MinIO Restore

```bash
# Mirror backup to production bucket
mc mirror --preserve backup/sfd-uploads-20250129 sfd-source/sfd-uploads

# Or restore from tarball
tar xzf sfd-uploads-20250129.tar.gz -C /tmp/
mc cp --recursive /tmp/sfd-backup/ sfd-source/sfd-uploads/
```

### Configuration Restore

```bash
# From tarball
tar xzf sfd-config-20250129.tar.gz -C /

# Or from git
cd /opt/sfd
git pull origin config-backups
cp .env.example .env
# Edit .env with production secrets
```

## Disaster Recovery

### Complete System Failure

**Recovery Steps:**

1. **Provision new infrastructure**
   - Deploy fresh VM/container
   - Install Docker and dependencies
   - Configure networking and DNS

2. **Restore configuration**
   ```bash
   # Clone repository
   git clone https://github.com/yourusername/secure-file-drop.git /opt/sfd
   cd /opt/sfd
   
   # Restore configs
   tar xzf /backup/sfd-config-latest.tar.gz -C /opt/sfd
   ```

3. **Restore database**
   ```bash
   # Start only PostgreSQL
   docker-compose up -d postgres
   
   # Restore latest backup
   ./scripts/restore-database.sh --force /backup/sfd_backup_latest.sql.gz
   ```

4. **Restore MinIO data**
   ```bash
   # Start MinIO
   docker-compose up -d minio
   
   # Configure mc client
   mc alias set sfd http://localhost:9000 minioadmin minioadmin
   
   # Restore from backup
   mc mirror --preserve backup/sfd-uploads-latest sfd/sfd-uploads
   ```

5. **Start application**
   ```bash
   docker-compose up -d
   ```

6. **Verify recovery**
   ```bash
   # Check services
   docker-compose ps
   
   # Test health endpoints
   curl http://localhost:8080/ready
   
   # Verify file count
   psql -U postgres -d sfd -c "SELECT COUNT(*) FROM files;"
   mc ls --recursive sfd/sfd-uploads | wc -l
   ```

### Database Corruption

If the database is corrupted but MinIO data is intact:

1. Drop corrupted database
2. Restore from latest backup
3. Verify file metadata matches MinIO objects
4. Regenerate missing metadata if needed

### MinIO Data Loss

If MinIO data is lost but database is intact:

1. Restore MinIO from backup
2. Verify object hashes match database
3. Mark files as "lost" in database if not recovered
4. Notify users of data loss

### Partial Data Loss

For individual file corruption or deletion:

```bash
# List backups
mc ls backup/sfd-uploads-20250129/

# Restore single file
mc cp backup/sfd-uploads-20250129/abc123/file.pdf sfd/sfd-uploads/abc123/file.pdf

# Verify hash
psql -U postgres -d sfd -c "SELECT hash FROM files WHERE id='abc123';"
```

## Testing

### Backup Verification

**Automated verification script:**

```bash
#!/bin/bash
# scripts/verify-backup.sh

# Test database backup
echo "Testing database backup..."
BACKUP_FILE=$(ls -t /var/backups/sfd/sfd_backup_*.sql.gz | head -1)
gunzip -t "$BACKUP_FILE" && echo "✓ Database backup valid" || echo "✗ Database backup corrupted"

# Test MinIO backup
echo "Testing MinIO backup..."
LATEST_BACKUP=$(mc ls backup | grep sfd-uploads | tail -1 | awk '{print $NF}')
OBJECT_COUNT=$(mc ls --recursive "backup/$LATEST_BACKUP" | wc -l)
echo "✓ MinIO backup contains $OBJECT_COUNT objects"

# Test restore (in isolated environment)
# ... additional restore testing
```

### Restore Testing

**Monthly restore drill:**

1. Create test environment
   ```bash
   docker-compose -f docker-compose.test.yml up -d
   ```

2. Restore latest backup to test environment
   ```bash
   export SFD_DB_HOST=localhost
   export SFD_DB_PORT=5433  # Test database port
   export SFD_DB_NAME=sfd_test
   ./scripts/restore-database.sh --force /backup/sfd_backup_latest.sql.gz
   ```

3. Verify data integrity
   ```bash
   psql -h localhost -p 5433 -U postgres -d sfd_test -c "SELECT COUNT(*) FROM users;"
   ```

4. Document results
   - Restore time
   - Data completeness
   - Any issues encountered

5. Clean up test environment
   ```bash
   docker-compose -f docker-compose.test.yml down -v
   ```

### Recovery Time Objective (RTO) Testing

Measure how long full recovery takes:

```bash
time ./scripts/disaster-recovery-test.sh
```

Document expected vs. actual recovery times.

## Best Practices

### 1. Follow 3-2-1 Rule

- **3** copies of data (original + 2 backups)
- **2** different media types (local disk + cloud)
- **1** copy offsite (remote datacenter)

### 2. Encrypt Sensitive Backups

```bash
# Encrypt with GPG
gpg --encrypt --recipient backup@example.com sfd_backup.sql.gz

# Or use age (modern alternative)
age -r age1... < sfd_backup.sql.gz > sfd_backup.sql.gz.age
```

### 3. Monitor Backup Health

Set up alerts for:
- Backup failures
- Backup size anomalies (too small = incomplete)
- Missing backups (skipped schedules)
- Old backups (staleness)

### 4. Document Recovery Procedures

- Keep printed copy of recovery steps
- Store in secure, accessible location
- Include emergency contact information
- Update after infrastructure changes

### 5. Automate Where Possible

- Cron jobs for scheduled backups
- Systemd timers with logging
- Monitoring integration (Prometheus alerts)
- Automated restore testing

### 6. Secure Backup Storage

- Use separate infrastructure for backups
- Apply least-privilege access
- Encrypt backups at rest
- Enable versioning and soft delete
- Regular access audits

### 7. Version Control for Configs

```bash
# Store configs in git
git init /opt/sfd/config
cd /opt/sfd/config
git add Caddyfile docker-compose.yml
git commit -m "Production config snapshot"
git push origin main
```

### 8. Test Regularly

- Monthly restore drills
- Annual disaster recovery simulation
- Document lessons learned
- Update procedures based on tests

## Troubleshooting

### Backup Fails with "Disk Full"

```bash
# Check disk usage
df -h /var/backups

# Clean old backups manually
find /var/backups/sfd -name "sfd_backup_*.sql.gz" -mtime +30 -delete

# Adjust retention period
export SFD_BACKUP_RETENTION=7
```

### GPG Decryption Fails

```bash
# List GPG keys
gpg --list-keys

# Import backup key
gpg --import backup-private-key.asc

# Decrypt backup
gpg --decrypt sfd_backup.sql.gz.gpg > sfd_backup.sql.gz
```

### MinIO Mirror Slow

```bash
# Use parallel workers
mc mirror --preserve --parallel 10 source/ dest/

# Or use rclone for better performance
rclone sync source/ dest/ --transfers=10 --checkers=20
```

### Restore Shows Wrong Data

- Check backup timestamp
- Verify backup metadata
- Ensure backup completed successfully
- Check for incremental vs. full backup

## Related Documentation

- [Deployment Guide](DEPLOYMENT.md) - Production deployment setup
- [Security Guide](SECURITY.md) - Security best practices
- [Proxmox Deployment](PROXMOX_DEPLOYMENT.md) - Proxmox-specific deployment
- [Monitoring Guide](../monitoring/README.md) - Monitoring and alerting

## Support

For backup and recovery assistance:
1. Check logs: `/var/log/sfd/backup.log`
2. Review [GitHub Issues](https://github.com/yourusername/secure-file-drop/issues)
3. Contact system administrator
4. Open a support ticket
