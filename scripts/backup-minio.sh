#!/bin/bash
#
# MinIO/S3 Backup Script for Secure File Drop
#
# This script mirrors the MinIO bucket to a backup location
# using mc (MinIO Client).
#
# Usage: ./backup-minio.sh [OPTIONS]
#   -d, --destination ALIAS  Backup destination alias (default: backup)
#   -k, --keep DAYS          Keep backups for N days (default: 30)
#   -h, --help               Show this help message

set -euo pipefail

# Configuration
MINIO_ENDPOINT="${SFD_MINIO_ENDPOINT:-localhost:9000}"
MINIO_ACCESS_KEY="${SFD_MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${SFD_MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${SFD_MINIO_BUCKET:-sfd-uploads}"
BACKUP_DESTINATION="${SFD_BACKUP_DESTINATION:-backup}"
RETENTION_DAYS="${SFD_BACKUP_RETENTION:-30}"
USE_SSL="${SFD_MINIO_USE_SSL:-false}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--destination)
            BACKUP_DESTINATION="$2"
            shift 2
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

# Check if mc is installed
if ! command -v mc &> /dev/null; then
    echo "Error: MinIO client (mc) is not installed"
    echo "Install it with: wget https://dl.min.io/client/mc/release/linux-amd64/mc && chmod +x mc"
    exit 1
fi

# Configure scheme
SCHEME="http"
if [[ "$USE_SSL" == "true" ]]; then
    SCHEME="https"
fi

echo "=== Secure File Drop MinIO Backup ==="
echo "Starting backup at $(date)"
echo "Source: ${SCHEME}://${MINIO_ENDPOINT}/${MINIO_BUCKET}"

# Configure source alias
mc alias set sfd-source "${SCHEME}://${MINIO_ENDPOINT}" "$MINIO_ACCESS_KEY" "$MINIO_SECRET_KEY"

# Test connection
if ! mc ls sfd-source >/dev/null 2>&1; then
    echo "Error: Cannot connect to source MinIO"
    exit 1
fi

# Get bucket stats before backup
OBJECT_COUNT=$(mc ls --recursive sfd-source/"$MINIO_BUCKET" 2>/dev/null | wc -l || echo "0")
BUCKET_SIZE=$(mc du sfd-source/"$MINIO_BUCKET" 2>/dev/null | awk '{print $1}' || echo "0")

echo "Objects to backup: $OBJECT_COUNT"
echo "Total size: $(numfmt --to=iec "$BUCKET_SIZE" 2>/dev/null || echo "${BUCKET_SIZE} bytes")"

# Create timestamped backup path
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_PATH="${MINIO_BUCKET}_${TIMESTAMP}"

# Mirror bucket to backup destination
echo "Mirroring bucket to backup..."
mc mirror --preserve \
    sfd-source/"$MINIO_BUCKET" \
    "$BACKUP_DESTINATION"/"$BACKUP_PATH"

# Verify backup
BACKUP_COUNT=$(mc ls --recursive "$BACKUP_DESTINATION"/"$BACKUP_PATH" 2>/dev/null | wc -l || echo "0")
echo "Backup completed: $BACKUP_COUNT objects"

# Create backup metadata
METADATA_FILE="/tmp/sfd_minio_backup_${TIMESTAMP}.json"
cat > "$METADATA_FILE" <<EOF
{
  "timestamp": "$(date -Iseconds)",
  "source_endpoint": "${MINIO_ENDPOINT}",
  "source_bucket": "${MINIO_BUCKET}",
  "backup_path": "${BACKUP_PATH}",
  "object_count": ${OBJECT_COUNT},
  "size_bytes": ${BUCKET_SIZE},
  "backup_count": ${BACKUP_COUNT}
}
EOF

mc cp "$METADATA_FILE" "$BACKUP_DESTINATION"/"$BACKUP_PATH"/backup_metadata.json
rm "$METADATA_FILE"

# Cleanup old backups
echo "Cleaning up old backups (keeping last $RETENTION_DAYS days)..."
CUTOFF_DATE=$(date -d "$RETENTION_DAYS days ago" +%Y%m%d)

mc ls "$BACKUP_DESTINATION" | grep "^[0-9]" | while read -r line; do
    BACKUP_DIR=$(echo "$line" | awk '{print $NF}' | sed 's:/$::')
    BACKUP_DATE=$(echo "$BACKUP_DIR" | grep -oP '\d{8}' | head -1 || echo "")
    
    if [[ -n "$BACKUP_DATE" ]] && [[ "$BACKUP_DATE" -lt "$CUTOFF_DATE" ]]; then
        echo "Removing old backup: $BACKUP_DIR"
        mc rm --recursive --force "$BACKUP_DESTINATION"/"$BACKUP_DIR"
    fi
done

# Count remaining backups
REMAINING_BACKUPS=$(mc ls "$BACKUP_DESTINATION" | grep "${MINIO_BUCKET}_" | wc -l)
echo "Backup retention: $REMAINING_BACKUPS backups"

echo "=== Backup completed successfully at $(date) ==="
echo "Backup location: $BACKUP_DESTINATION/$BACKUP_PATH"
