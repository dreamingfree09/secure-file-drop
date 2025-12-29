# Proxmox Deployment Guide

This guide provides a complete step-by-step walkthrough for deploying Secure File Drop on a Proxmox hypervisor using an LXC container. The guide includes both manual instructions and an automated setup script.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Architecture Overview](#architecture-overview)
- [Step-by-Step Deployment](#step-by-step-deployment)
- [Post-Deployment Configuration](#post-deployment-configuration)
- [Automated Deployment Script](#automated-deployment-script)
- [Monitoring and Maintenance](#monitoring-and-maintenance)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Proxmox Host Requirements

- Proxmox VE 7.0 or higher
- At least 4GB RAM available for the container
- 20GB+ storage for container and data
- Network access (DHCP or static IP configuration)
- (Optional) Domain name pointed to your Proxmox host's IP

### What You'll Need

- SSH access to your Proxmox host
- Root credentials for Proxmox
- Email credentials for SMTP (optional but recommended)
- SSL certificate (Let's Encrypt will be configured automatically)

## Architecture Overview

This deployment uses:

- **LXC Container**: Ubuntu 22.04 running Docker
- **Docker Compose**: Orchestrates backend, PostgreSQL, and MinIO services
- **Caddy**: Reverse proxy with automatic HTTPS via Let's Encrypt
- **Systemd**: Manages container auto-start and service persistence

**Network Flow:**
```
Internet → Proxmox Host (Port 80/443) → LXC Container (Port 80/443) → Caddy Reverse Proxy → Backend (Port 8080)
                                                                       ↓
                                                   PostgreSQL (5432) + MinIO (9000)
```

## Step-by-Step Deployment

### Step 1: Create LXC Container in Proxmox

1. **Access Proxmox Web Interface**
   - Navigate to `https://your-proxmox-ip:8006`
   - Log in with root credentials

2. **Create New Container**
   - Click "Create CT" in the top-right corner
   - **General Tab:**
     - Node: Select your Proxmox node
     - CT ID: Auto-assign or choose (e.g., 100)
     - Hostname: `sfd-production` (or your preference)
     - Unprivileged container: **Unchecked** (we need privileged for Docker)
     - Password: Set a strong root password
     - SSH public key: (Optional) Paste your SSH public key
   
   - **Template Tab:**
     - Storage: Select storage
     - Template: `ubuntu-22.04-standard_22.04-1_amd64.tar.zst` (or latest Ubuntu 22.04)
   
   - **Disks Tab:**
     - Disk size: `20 GB` minimum (recommend 50GB+ for production)
   
   - **CPU Tab:**
     - Cores: `2` minimum (recommend 4 for production)
   
   - **Memory Tab:**
     - Memory (MiB): `4096` minimum (recommend 8192 for production)
     - Swap (MiB): `2048`
   
   - **Network Tab:**
     - Bridge: `vmbr0` (or your network bridge)
     - IPv4: DHCP or static (e.g., `192.168.1.50/24`)
     - IPv4 Gateway: Your gateway IP (e.g., `192.168.1.1`)
     - IPv6: DHCP or leave blank
   
   - **DNS Tab:**
     - Use host settings: Checked (or configure custom DNS)
   
   - **Confirm**: Review and click "Finish"

3. **Configure Container Features**
   - After creation, select the container (e.g., 100)
   - Go to "Options"
   - Edit "Features":
     - Enable: `nesting=1` (required for Docker)
     - Enable: `keyctl=1` (optional, improves compatibility)
   - Click "OK"

4. **Start Container**
   - Click "Start" in the top-right
   - Wait for container to boot (check status shows "running")

### Step 2: Configure Container and Install Docker

1. **Access Container Console**
   - In Proxmox web UI, select your container
   - Click "Console" button
   - Log in as `root` with the password you set

2. **Update System and Install Prerequisites**
   ```bash
   apt update && apt upgrade -y
   apt install -y curl wget git nano ca-certificates gnupg lsb-release
   ```

3. **Install Docker**
   ```bash
   # Add Docker's official GPG key
   mkdir -p /etc/apt/keyrings
   curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
   
   # Set up Docker repository
   echo \
     "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
     $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
   
   # Install Docker Engine
   apt update
   apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
   
   # Verify installation
   docker --version
   docker compose version
   ```

4. **Configure Docker to Start on Boot**
   ```bash
   systemctl enable docker
   systemctl start docker
   systemctl status docker
   ```

### Step 3: Deploy Secure File Drop

1. **Create Application Directory**
   ```bash
   mkdir -p /opt/secure-file-drop
   cd /opt/secure-file-drop
   ```

2. **Clone Repository (or Upload Files)**
   
   **Option A: Clone from Git (if repository is available)**
   ```bash
   git clone https://github.com/yourusername/secure-file-drop.git .
   ```
   
   **Option B: Manual Upload**
   - On your local machine, compress the project:
     ```bash
     tar czf secure-file-drop.tar.gz -C "/home/dreamingfree09/Secure File Drop" .
     ```
   - Upload to container using SCP:
     ```bash
     scp secure-file-drop.tar.gz root@container-ip:/opt/secure-file-drop/
     ```
   - Extract in container:
     ```bash
     cd /opt/secure-file-drop
     tar xzf secure-file-drop.tar.gz
     rm secure-file-drop.tar.gz
     ```

3. **Create Environment Configuration**
   ```bash
   cd /opt/secure-file-drop
   cp .env.example .env
   nano .env
   ```

4. **Configure Environment Variables**
   
   Edit `.env` and set the following (replace with your actual values):
   
   ```bash
   # === Core Secrets (REQUIRED) ===
   # Generate with: openssl rand -base64 32
   SFD_SESSION_SECRET=YOUR_RANDOM_SECRET_HERE_32_CHARS
   SFD_DOWNLOAD_SECRET=YOUR_RANDOM_SECRET_HERE_32_CHARS
   
   # === Admin Account (REQUIRED) ===
   SFD_ADMIN_USER=admin
   # Generate with: echo -n "YourPasswordHere" | openssl passwd -6 -stdin
   SFD_ADMIN_PASS='$6$rounds=656000$...'  # bcrypt hash of your password
   
   # === Database (REQUIRED) ===
   DATABASE_URL=postgresql://sfd:sfd_password_here@postgres:5432/sfd?sslmode=disable
   POSTGRES_USER=sfd
   POSTGRES_PASSWORD=sfd_password_here
   POSTGRES_DB=sfd
   
   # === MinIO / S3 Storage (REQUIRED) ===
   SFD_S3_ENDPOINT=minio:9000
   SFD_S3_ACCESS_KEY=minioadmin
   SFD_S3_SECRET_KEY=minioadmin123
   SFD_BUCKET=sfd-private
   
   # === Public Base URL (REQUIRED for production) ===
   # Set to your domain name (e.g., https://files.yourdomain.com)
   SFD_PUBLIC_BASE_URL=https://files.example.com
   
   # === SMTP Email (Optional but recommended) ===
   SFD_SMTP_HOST=smtp.gmail.com
   SFD_SMTP_PORT=587
   SFD_SMTP_USER=your-email@gmail.com
   SFD_SMTP_PASS=your-app-password
   SFD_SMTP_FROM=noreply@yourdomain.com
   
   # === Upload & Cleanup Configuration ===
   SFD_MAX_UPLOAD_BYTES=53687091200  # 50GB default
   SFD_CLEANUP_ENABLED=true
   SFD_CLEANUP_INTERVAL=1h
   SFD_CLEANUP_MAX_AGE=24h
   ```

5. **Build and Start Services**
   ```bash
   cd /opt/secure-file-drop
   docker compose build
   docker compose up -d
   ```

6. **Verify Services are Running**
   ```bash
   docker compose ps
   docker compose logs -f backend
   ```
   
   Look for:
   - `migrations_complete`
   - `starting addr=:8080`
   - All containers showing "Up" status

7. **Test Readiness Endpoint**
   ```bash
   curl http://localhost:8080/ready
   ```
   
   Expected output:
   ```json
   {"status":"ok","components":{"minio":"ok","postgres":"ok"}}
   ```

### Step 4: Install and Configure Caddy Reverse Proxy

1. **Install Caddy**
   ```bash
   apt install -y debian-keyring debian-archive-keyring apt-transport-https
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
   apt update
   apt install -y caddy
   ```

2. **Create Caddyfile**
   ```bash
   nano /etc/caddy/Caddyfile
   ```
   
   Add the following configuration (replace `files.example.com` with your domain):
   
   ```
   files.example.com {
     # Automatic HTTPS via Let's Encrypt
     encode zstd gzip
     
     # Security headers
     header {
       Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
       X-Content-Type-Options "nosniff"
       X-Frame-Options "DENY"
       Referrer-Policy "no-referrer"
       Permissions-Policy "geolocation=(), microphone=(), camera=()"
     }
     
     # Rate limiting (basic IP-based)
     # For production, consider using Caddy rate limit module or external WAF
     
     # Proxy to backend
     reverse_proxy localhost:8080 {
       header_up X-Real-IP {remote_host}
       header_up X-Forwarded-For {remote_host}
       header_up X-Forwarded-Proto {scheme}
     }
     
     # Logging
     log {
       output file /var/log/caddy/sfd-access.log
       format json
     }
   }
   ```

3. **Configure Firewall (if UFW is enabled)**
   ```bash
   ufw allow 80/tcp
   ufw allow 443/tcp
   ufw reload
   ```

4. **Start and Enable Caddy**
   ```bash
   systemctl enable caddy
   systemctl restart caddy
   systemctl status caddy
   ```

5. **Verify Caddy is Working**
   ```bash
   curl -I http://localhost
   ```
   
   Should redirect to HTTPS and show backend response headers.

### Step 5: Configure Proxmox Port Forwarding

If your domain points to your Proxmox host's public IP, you need to forward ports 80 and 443 to the LXC container.

**Option A: Using iptables on Proxmox Host**

SSH into your **Proxmox host** (not the container) and run:

```bash
# Replace 192.168.1.50 with your container's IP
CONTAINER_IP=192.168.1.50

# Forward HTTP (80)
iptables -t nat -A PREROUTING -i vmbr0 -p tcp --dport 80 -j DNAT --to ${CONTAINER_IP}:80
iptables -t nat -A POSTROUTING -s ${CONTAINER_IP}/32 -o vmbr0 -p tcp --dport 80 -j MASQUERADE

# Forward HTTPS (443)
iptables -t nat -A PREROUTING -i vmbr0 -p tcp --dport 443 -j DNAT --to ${CONTAINER_IP}:443
iptables -t nat -A POSTROUTING -s ${CONTAINER_IP}/32 -o vmbr0 -p tcp --dport 443 -j MASQUERADE

# Save rules
apt install -y iptables-persistent
netfilter-persistent save
```

**Option B: Bridge Container Directly to Network**

Edit container network configuration in Proxmox UI to use a public-facing bridge or assign the container a public IP directly.

### Step 6: Verify Deployment

1. **Access Application via Domain**
   - Navigate to `https://files.example.com` in your browser
   - You should see the Secure File Drop login page
   - Certificate should be automatically issued by Let's Encrypt

2. **Test Login**
   - Username: Value of `SFD_ADMIN_USER` (e.g., `admin`)
   - Password: Plain text password you hashed for `SFD_ADMIN_PASS`

3. **Test File Upload**
   - Upload a small test file
   - Create a download link
   - Verify download works

4. **Check Email Notifications (if configured)**
   - Register a new user
   - Check that verification email arrives

## Post-Deployment Configuration

### Enable Container Auto-Start on Boot

1. **In Proxmox Web UI:**
   - Select your container
   - Go to "Options"
   - Edit "Start at boot": Enable
   - Edit "Startup order": Set order (e.g., 1)
   - Click "OK"

### Configure Automatic Backups

1. **In Proxmox Web UI:**
   - Navigate to Datacenter → Backup
   - Click "Add" to create a backup job
   - Configure:
     - Storage: Select backup storage
     - Schedule: Daily at 2:00 AM (or your preference)
     - Selection mode: All
     - Retention: Keep last 7 backups
     - Compression: ZSTD (recommended)
   - Click "Create"

### Set Up Log Rotation

Inside the container, create log rotation for Caddy and Docker:

```bash
cat > /etc/logrotate.d/caddy <<'EOF'
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
EOF

cat > /etc/logrotate.d/docker-compose <<'EOF'
/opt/secure-file-drop/logs/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    copytruncate
}
EOF
```

### Configure System Monitoring

1. **Install monitoring tools:**
   ```bash
   apt install -y htop iotop nethogs
   ```

2. **Set up health check cron:**
   ```bash
   cat > /usr/local/bin/sfd-healthcheck.sh <<'EOF'
   #!/bin/bash
   response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/ready)
   if [ "$response" != "200" ]; then
       echo "SFD health check failed (HTTP $response)" | systemd-cat -t sfd-healthcheck
       cd /opt/secure-file-drop && docker compose restart backend
   fi
   EOF
   
   chmod +x /usr/local/bin/sfd-healthcheck.sh
   
   # Add to crontab (run every 5 minutes)
   (crontab -l 2>/dev/null; echo "*/5 * * * * /usr/local/bin/sfd-healthcheck.sh") | crontab -
   ```

## Automated Deployment Script

The following script automates the entire deployment process inside an LXC container. Run this **after creating the container** in Proxmox.

### Prerequisites

1. Create LXC container in Proxmox (Steps 1.1-1.4 from above)
2. Have your configuration values ready (domain, secrets, SMTP credentials)

### Usage

1. **Access your container console**
2. **Download and run the script:**
   ```bash
   curl -o /tmp/deploy-sfd.sh https://raw.githubusercontent.com/yourusername/secure-file-drop/main/scripts/proxmox-deploy.sh
   chmod +x /tmp/deploy-sfd.sh
   /tmp/deploy-sfd.sh
   ```

Or copy the script below and save it as `/tmp/deploy-sfd.sh`:

```bash
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
#   2. Run: chmod +x deploy-sfd.sh && ./deploy-sfd.sh
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
```

### Running the Automated Script

1. **Save the script** as `/tmp/deploy-sfd.sh` in your container

2. **Edit configuration values** at the top of the script:
   ```bash
   nano /tmp/deploy-sfd.sh
   ```
   
   Update:
   - `DOMAIN="files.example.com"` - Your actual domain
   - `ADMIN_PASSWORD="YourStrongPasswordHere"` - Your admin password
   - SMTP settings (if using email)
   - `REPO_URL` - Your repository URL (or leave blank to upload files manually)

3. **Make executable and run:**
   ```bash
   chmod +x /tmp/deploy-sfd.sh
   /tmp/deploy-sfd.sh
   ```

4. **Monitor the deployment:**
   - The script will output progress as it runs
   - Takes approximately 5-10 minutes to complete
   - Credentials will be saved to `/root/sfd-credentials.txt`

5. **Post-deployment:**
   - Wait 1-2 minutes for Let's Encrypt certificate
   - Access your domain: `https://files.example.com`
   - Log in with admin credentials
   - Delete credentials file: `rm /root/sfd-credentials.txt`

## Monitoring and Maintenance

### Check Service Status

```bash
# Check all services
cd /opt/secure-file-drop
docker compose ps

# View backend logs
docker compose logs -f backend

# View all logs
docker compose logs -f

# Check Caddy status
systemctl status caddy

# View Caddy logs
journalctl -u caddy -f
```

### Update Application

```bash
cd /opt/secure-file-drop

# Pull latest changes
git pull

# Rebuild and restart
docker compose build
docker compose up -d

# Check health
curl http://localhost:8080/ready
```

### Backup Data

```bash
# Backup database
docker exec sfd_postgres pg_dump -U sfd sfd > /root/sfd-backup-$(date +%F).sql

# Backup MinIO data
docker exec sfd_minio mc mirror /data /backup/minio-$(date +%F)

# Or use Proxmox backup (recommended)
# Navigate to Proxmox UI → Backup → Create backup job
```

### Restore from Backup

```bash
# Restore database
cat /root/sfd-backup-2025-12-29.sql | docker exec -i sfd_postgres psql -U sfd -d sfd

# Restore Proxmox container backup
# Navigate to Proxmox UI → Select container → Backup → Select backup → Restore
```

## Troubleshooting

### Services Won't Start

```bash
# Check Docker daemon
systemctl status docker

# Check container logs
cd /opt/secure-file-drop
docker compose logs

# Restart all services
docker compose down
docker compose up -d
```

### Can't Access via Domain

1. **Check DNS resolution:**
   ```bash
   nslookup files.example.com
   dig files.example.com
   ```

2. **Check Caddy is running:**
   ```bash
   systemctl status caddy
   curl -I http://localhost
   ```

3. **Check firewall:**
   ```bash
   ufw status
   iptables -L -n -v
   ```

4. **Check port forwarding** (on Proxmox host):
   ```bash
   iptables -t nat -L -n -v
   ```

### Let's Encrypt Certificate Issues

```bash
# Check Caddy logs
journalctl -u caddy -n 100

# Common issues:
# - Domain doesn't point to server IP
# - Port 80/443 blocked by firewall
# - Previous certificate rate limit hit

# Force certificate renewal
caddy reload --config /etc/caddy/Caddyfile
```

### Database Connection Errors

```bash
# Check PostgreSQL is running
docker ps | grep postgres

# Test connection
docker exec -it sfd_postgres psql -U sfd -d sfd -c "SELECT 1;"

# Check credentials in .env match docker-compose.yml
cat /opt/secure-file-drop/.env | grep POSTGRES
```

### Upload Failures

```bash
# Check MinIO is running
docker ps | grep minio

# Check disk space
df -h

# Check MinIO logs
docker compose logs minio

# Verify bucket exists
docker exec sfd_minio mc ls /data/
```

### Email Not Sending

```bash
# Check SMTP settings in .env
cat /opt/secure-file-drop/.env | grep SMTP

# Test SMTP connection
telnet smtp.gmail.com 587

# Check backend logs for email errors
docker compose logs backend | grep -i smtp
```

## Security Hardening (Optional)

### Enable Fail2Ban

```bash
apt install -y fail2ban

cat > /etc/fail2ban/jail.local <<'EOF'
[sshd]
enabled = true
port = ssh
logpath = /var/log/auth.log
maxretry = 3
bantime = 3600
EOF

systemctl enable fail2ban
systemctl restart fail2ban
```

### Configure UFW Firewall

```bash
# Install and enable UFW
apt install -y ufw

# Default policies
ufw default deny incoming
ufw default allow outgoing

# Allow SSH (be careful!)
ufw allow 22/tcp

# Allow HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp

# Enable firewall
ufw enable

# Check status
ufw status verbose
```

### Disable Root SSH Login

```bash
nano /etc/ssh/sshd_config

# Set: PermitRootLogin no

systemctl restart sshd
```

---

**Deployment Guide Version:** 1.0  
**Last Updated:** December 29, 2025  
**Tested On:** Proxmox VE 8.0, Ubuntu 22.04 LXC

For issues or improvements to this guide, please open an issue in the repository.
