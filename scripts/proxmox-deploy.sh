#!/bin/bash
set -euo pipefail

#
# Secure File Drop - Proxmox LXC Automated Deployment Script
#
# This script automates the complete deployment of Secure File Drop
# on a Proxmox LXC container running Ubuntu 22.04.
#
# Prerequisites:
#   - LXC container created with nesting=1 enabled
#   - Ubuntu 22.04 LXC template
#   - At least 4GB RAM, 20GB disk, 2 CPU cores
#   - Root access to the container
#
# Usage:
#   1. Edit the CONFIGURATION section below with your values
#   2. Run: chmod +x proxmox-deploy.sh && ./proxmox-deploy.sh
#

# ============================================================================
# CONFIGURATION - Edit these values before running
# ============================================================================

# Domain configuration
DOMAIN="files.example.com"  # Your domain name

# Admin credentials
ADMIN_USER="admin"
ADMIN_PASSWORD="YourStrongPasswordHere"  # Will be hashed automatically

# Database credentials
DB_PASSWORD="$(openssl rand -base64 32 | tr -d '/+=')"  # Auto-generated

# MinIO credentials
MINIO_ACCESS_KEY="minioadmin"
MINIO_SECRET_KEY="$(openssl rand -base64 32 | tr -d '/+=')"  # Auto-generated

# Session secrets (auto-generated)
SESSION_SECRET="$(openssl rand -base64 32)"
DOWNLOAD_SECRET="$(openssl rand -base64 32)"

# SMTP configuration (optional - leave empty to skip)
SMTP_HOST=""  # e.g., smtp.gmail.com
SMTP_PORT="587"
SMTP_USER=""  # e.g., your-email@gmail.com
SMTP_PASS=""  # e.g., your-app-password
SMTP_FROM=""  # e.g., noreply@yourdomain.com

# Upload limits
MAX_UPLOAD_BYTES="53687091200"  # 50GB

# Repository source (edit if using custom repo/branch)
REPO_URL="https://github.com/yourusername/secure-file-drop.git"
REPO_BRANCH="main"

# Installation directory
INSTALL_DIR="/opt/secure-file-drop"

# ============================================================================
# SCRIPT START - Do not edit below unless you know what you're doing
# ============================================================================

echo "=========================================="
echo "Secure File Drop - Proxmox Deployment"
echo "=========================================="
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    log_error "This script must be run as root"
    exit 1
fi

# Step 1: Update system
log_info "Updating system packages..."
apt update && apt upgrade -y

# Step 2: Install prerequisites
log_info "Installing prerequisites..."
apt install -y curl wget git nano ca-certificates gnupg lsb-release apache2-utils

# Step 3: Install Docker
log_info "Installing Docker..."
if ! command -v docker &> /dev/null; then
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt update
    apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    systemctl enable docker
    systemctl start docker
    log_info "Docker installed successfully"
else
    log_info "Docker already installed"
fi

# Step 4: Clone repository
log_info "Cloning Secure File Drop repository..."
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

if [ -n "$REPO_URL" ]; then
    git clone -b "$REPO_BRANCH" "$REPO_URL" .
else
    log_error "REPO_URL not set. Please configure repository URL or manually upload files."
    exit 1
fi

# Step 5: Generate bcrypt hash for admin password
log_info "Generating admin password hash..."
ADMIN_PASS_HASH=$(echo -n "$ADMIN_PASSWORD" | openssl passwd -6 -stdin)

# Step 6: Create .env file
log_info "Creating environment configuration..."
cat > "$INSTALL_DIR/.env" <<EOF
# Secure File Drop Environment Configuration
# Generated on $(date)

# === Core Secrets ===
SFD_SESSION_SECRET=${SESSION_SECRET}
SFD_DOWNLOAD_SECRET=${DOWNLOAD_SECRET}

# === Admin Account ===
SFD_ADMIN_USER=${ADMIN_USER}
SFD_ADMIN_PASS='${ADMIN_PASS_HASH}'

# === Database ===
DATABASE_URL=postgresql://sfd:${DB_PASSWORD}@postgres:5432/sfd?sslmode=disable
POSTGRES_USER=sfd
POSTGRES_PASSWORD=${DB_PASSWORD}
POSTGRES_DB=sfd

# === MinIO / S3 Storage ===
SFD_S3_ENDPOINT=minio:9000
SFD_S3_ACCESS_KEY=${MINIO_ACCESS_KEY}
SFD_S3_SECRET_KEY=${MINIO_SECRET_KEY}
SFD_BUCKET=sfd-private

# === Public Base URL ===
SFD_PUBLIC_BASE_URL=https://${DOMAIN}

# === Upload & Cleanup Configuration ===
SFD_MAX_UPLOAD_BYTES=${MAX_UPLOAD_BYTES}
SFD_CLEANUP_ENABLED=true
SFD_CLEANUP_INTERVAL=1h
SFD_CLEANUP_MAX_AGE=24h
EOF

# Add SMTP configuration if provided
if [ -n "$SMTP_HOST" ]; then
    cat >> "$INSTALL_DIR/.env" <<EOF

# === SMTP Email ===
SFD_SMTP_HOST=${SMTP_HOST}
SFD_SMTP_PORT=${SMTP_PORT}
SFD_SMTP_USER=${SMTP_USER}
SFD_SMTP_PASS=${SMTP_PASS}
SFD_SMTP_FROM=${SMTP_FROM}
EOF
fi

chmod 600 "$INSTALL_DIR/.env"

# Step 7: Build and start services
log_info "Building Docker images..."
cd "$INSTALL_DIR"
docker compose build

log_info "Starting services..."
docker compose up -d

# Wait for services to be ready
log_info "Waiting for services to initialize..."
sleep 10

# Step 8: Verify backend is healthy
log_info "Checking backend health..."
for i in {1..30}; do
    if curl -f -s http://localhost:8080/ready > /dev/null 2>&1; then
        log_info "Backend is healthy!"
        break
    fi
    if [ $i -eq 30 ]; then
        log_error "Backend failed to start. Check logs with: docker compose logs backend"
        exit 1
    fi
    sleep 2
done

# Step 9: Install Caddy
log_info "Installing Caddy reverse proxy..."
if ! command -v caddy &> /dev/null; then
    apt install -y debian-keyring debian-archive-keyring apt-transport-https
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
    apt update
    apt install -y caddy
else
    log_info "Caddy already installed"
fi

# Step 10: Configure Caddy
log_info "Configuring Caddy..."
cat > /etc/caddy/Caddyfile <<EOF
${DOMAIN} {
    encode zstd gzip
    
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        Referrer-Policy "no-referrer"
        Permissions-Policy "geolocation=(), microphone=(), camera=()"
    }
    
    reverse_proxy localhost:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
    
    log {
        output file /var/log/caddy/sfd-access.log
        format json
    }
}
EOF

# Step 11: Start Caddy
log_info "Starting Caddy..."
systemctl enable caddy
systemctl restart caddy

# Step 12: Set up health check cron
log_info "Setting up health monitoring..."
cat > /usr/local/bin/sfd-healthcheck.sh <<'HEALTHCHECK'
#!/bin/bash
response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/ready)
if [ "$response" != "200" ]; then
    echo "SFD health check failed (HTTP $response)" | systemd-cat -t sfd-healthcheck
    cd /opt/secure-file-drop && docker compose restart backend
fi
HEALTHCHECK

chmod +x /usr/local/bin/sfd-healthcheck.sh
(crontab -l 2>/dev/null; echo "*/5 * * * * /usr/local/bin/sfd-healthcheck.sh") | crontab -

# Step 13: Configure log rotation
log_info "Configuring log rotation..."
cat > /etc/logrotate.d/caddy <<'LOGROTATE'
/var/log/caddy/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 caddy caddy
    sharedscripts
    postrotate
        systemctl reload caddy
    endscript
}
LOGROTATE

# Step 14: Save credentials to file
log_info "Saving credentials..."
cat > /root/sfd-credentials.txt <<EOF
Secure File Drop - Deployment Credentials
==========================================
Generated on: $(date)

Domain: https://${DOMAIN}

Admin Login:
  Username: ${ADMIN_USER}
  Password: ${ADMIN_PASSWORD}

Database:
  User: sfd
  Password: ${DB_PASSWORD}

MinIO:
  Access Key: ${MINIO_ACCESS_KEY}
  Secret Key: ${MINIO_SECRET_KEY}

Session Secret: ${SESSION_SECRET}
Download Secret: ${DOWNLOAD_SECRET}

IMPORTANT: Store these credentials securely and delete this file after saving them elsewhere.
EOF

chmod 600 /root/sfd-credentials.txt

# Final output
echo ""
echo "=========================================="
log_info "Deployment Complete!"
echo "=========================================="
echo ""
echo "Application URL: https://${DOMAIN}"
echo "Admin Username: ${ADMIN_USER}"
echo "Admin Password: ${ADMIN_PASSWORD}"
echo ""
echo "Credentials saved to: /root/sfd-credentials.txt"
echo ""
echo "Next steps:"
echo "  1. Ensure your domain ${DOMAIN} points to this server's IP"
echo "  2. Configure port forwarding on Proxmox host (if needed)"
echo "  3. Wait 1-2 minutes for Let's Encrypt certificate provisioning"
echo "  4. Access https://${DOMAIN} and log in"
echo "  5. Delete /root/sfd-credentials.txt after saving credentials securely"
echo ""
echo "Useful commands:"
echo "  - View logs: cd ${INSTALL_DIR} && docker compose logs -f"
echo "  - Restart: cd ${INSTALL_DIR} && docker compose restart"
echo "  - Stop: cd ${INSTALL_DIR} && docker compose down"
echo "  - Start: cd ${INSTALL_DIR} && docker compose up -d"
echo ""
log_info "Deployment successful!"
