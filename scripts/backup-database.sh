#!/bin/bash
#
# Database Backup Script for Secure File Drop
# 
# This script creates timestamped backups of the PostgreSQL database
# with compression and optional encryption.
#
# Usage: ./backup-database.sh [OPTIONS]
#   -e, --encrypt     Encrypt backup with GPG
#   -r, --remote      Upload to remote storage (S3/B2)
#   -k, --keep DAYS   Keep backups for N days (default: 30)
#   -h, --help        Show this help message

set -euo pipefail

# Configuration (can be overridden by environment variables)
DB_HOST="${SFD_DB_HOST:-localhost}"
DB_PORT="${SFD_DB_PORT:-5432}"
DB_NAME="${SFD_DB_NAME:-sfd}"
DB_USER="${SFD_DB_USER:-postgres}"
BACKUP_DIR="${SFD_BACKUP_DIR:-/var/backups/sfd}"
RETENTION_DAYS="${SFD_BACKUP_RETENTION:-30}"
ENCRYPT_BACKUPS="${SFD_BACKUP_ENCRYPT:-false}"
GPG_RECIPIENT="${SFD_BACKUP_GPG_RECIPIENT:-}"
REMOTE_UPLOAD="${SFD_BACKUP_REMOTE:-false}"
S3_BUCKET="${SFD_BACKUP_S3_BUCKET:-}"
S3_PREFIX="${SFD_BACKUP_S3_PREFIX:-backups/database}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--encrypt)
            ENCRYPT_BACKUPS=true
            shift
            ;;
        -r|--remote)
            REMOTE_UPLOAD=true
            shift
            ;;
        -k|--keep)
            RETENTION_DAYS="$2"
            shift 2
            ;;
        -h|--help)
            grep '^#' "$0" | grep -v '#!/bin/bash' | sed 's/^# //'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate configuration
if [[ "$ENCRYPT_BACKUPS" == "true" ]] && [[ -z "$GPG_RECIPIENT" ]]; then
    echo "Error: GPG_RECIPIENT must be set when encryption is enabled"
    exit 1
fi

if [[ "$REMOTE_UPLOAD" == "true" ]] && [[ -z "$S3_BUCKET" ]]; then
    echo "Error: S3_BUCKET must be set when remote upload is enabled"
    exit 1
fi

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Generate backup filename with timestamp
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/sfd_backup_${TIMESTAMP}.sql.gz"

echo "=== Secure File Drop Database Backup ==="
echo "Starting backup at $(date)"
echo "Database: $DB_NAME@$DB_HOST:$DB_PORT"
echo "Backup file: $BACKUP_FILE"

# Create database dump with compression
echo "Creating database dump..."
PGPASSWORD="${PGPASSWORD:-}" pg_dump \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    --no-owner \
    --no-acl \
    --format=plain \
    | gzip > "$BACKUP_FILE"

# Verify backup was created
if [[ ! -f "$BACKUP_FILE" ]]; then
    echo "Error: Backup file was not created"
    exit 1
fi

BACKUP_SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
echo "Backup created successfully (size: $BACKUP_SIZE)"

# Encrypt backup if requested
if [[ "$ENCRYPT_BACKUPS" == "true" ]]; then
    echo "Encrypting backup..."
    gpg --encrypt \
        --recipient "$GPG_RECIPIENT" \
        --output "${BACKUP_FILE}.gpg" \
        "$BACKUP_FILE"
    
    # Remove unencrypted backup
    rm "$BACKUP_FILE"
    BACKUP_FILE="${BACKUP_FILE}.gpg"
    echo "Backup encrypted successfully"
fi

# Upload to remote storage if requested
if [[ "$REMOTE_UPLOAD" == "true" ]]; then
    echo "Uploading to remote storage..."
    
    # Determine upload tool (aws-cli or rclone)
    if command -v aws &> /dev/null; then
        aws s3 cp "$BACKUP_FILE" "s3://${S3_BUCKET}/${S3_PREFIX}/$(basename "$BACKUP_FILE")"
    elif command -v rclone &> /dev/null; then
        rclone copy "$BACKUP_FILE" "remote:${S3_BUCKET}/${S3_PREFIX}/"
    else
        echo "Warning: Neither aws-cli nor rclone found. Skipping remote upload."
    fi
    
    echo "Upload completed"
fi

# Cleanup old backups
echo "Cleaning up old backups (keeping last $RETENTION_DAYS days)..."
find "$BACKUP_DIR" -name "sfd_backup_*.sql.gz*" -type f -mtime "+$RETENTION_DAYS" -delete

# Count remaining backups
BACKUP_COUNT=$(find "$BACKUP_DIR" -name "sfd_backup_*.sql.gz*" -type f | wc -l)
echo "Backup retention: $BACKUP_COUNT backups in $BACKUP_DIR"

# Create backup metadata
cat > "${BACKUP_FILE}.meta" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "database": "$DB_NAME",
  "host": "$DB_HOST",
  "size_bytes": $(stat -c%s "$BACKUP_FILE"),
  "encrypted": $ENCRYPT_BACKUPS,
  "remote_upload": $REMOTE_UPLOAD,
  "version": "$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c 'SELECT version();' | head -n1 | xargs)"
}
EOF

echo "=== Backup completed successfully at $(date) ==="
echo "Backup file: $BACKUP_FILE"
echo "Metadata: ${BACKUP_FILE}.meta"
