#!/bin/bash
#
# Database Restore Script for Secure File Drop
#
# This script restores a PostgreSQL database from a backup file.
#
# Usage: ./restore-database.sh [OPTIONS] BACKUP_FILE
#   -f, --force       Skip confirmation prompt
#   -d, --decrypt     Decrypt backup with GPG
#   -h, --help        Show this help message

set -euo pipefail

# Configuration
DB_HOST="${SFD_DB_HOST:-localhost}"
DB_PORT="${SFD_DB_PORT:-5432}"
DB_NAME="${SFD_DB_NAME:-sfd}"
DB_USER="${SFD_DB_USER:-postgres}"
FORCE_RESTORE=false
DECRYPT_BACKUP=false

# Parse arguments
BACKUP_FILE=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--force)
            FORCE_RESTORE=true
            shift
            ;;
        -d|--decrypt)
            DECRYPT_BACKUP=true
            shift
            ;;
        -h|--help)
            grep '^#' "$0" | grep -v '#!/bin/bash' | sed 's/^# //'
            exit 0
            ;;
        *)
            BACKUP_FILE="$1"
            shift
            ;;
    esac
done

# Validate backup file
if [[ -z "$BACKUP_FILE" ]]; then
    echo "Error: Backup file not specified"
    echo "Usage: $0 [OPTIONS] BACKUP_FILE"
    exit 1
fi

if [[ ! -f "$BACKUP_FILE" ]]; then
    echo "Error: Backup file not found: $BACKUP_FILE"
    exit 1
fi

echo "=== Secure File Drop Database Restore ==="
echo "Backup file: $BACKUP_FILE"
echo "Target database: $DB_NAME@$DB_HOST:$DB_PORT"

# Confirmation prompt
if [[ "$FORCE_RESTORE" != "true" ]]; then
    echo ""
    echo "WARNING: This will DROP and recreate the database!"
    echo "All existing data will be lost."
    read -p "Are you sure you want to continue? (yes/no): " -r
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        echo "Restore cancelled"
        exit 0
    fi
fi

# Decrypt backup if needed
TEMP_FILE=""
if [[ "$DECRYPT_BACKUP" == "true" ]] || [[ "$BACKUP_FILE" == *.gpg ]]; then
    echo "Decrypting backup..."
    TEMP_FILE=$(mktemp)
    gpg --decrypt --output "$TEMP_FILE" "$BACKUP_FILE"
    BACKUP_FILE="$TEMP_FILE"
fi

# Create backup of current database before restore
echo "Creating safety backup of current database..."
SAFETY_BACKUP="/tmp/sfd_safety_backup_$(date +%Y%m%d_%H%M%S).sql.gz"
PGPASSWORD="${PGPASSWORD:-}" pg_dump \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    --no-owner \
    --no-acl \
    | gzip > "$SAFETY_BACKUP" 2>/dev/null || echo "Warning: Could not create safety backup"

echo "Safety backup: $SAFETY_BACKUP"

# Drop existing connections
echo "Terminating existing connections..."
PGPASSWORD="${PGPASSWORD:-}" psql \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d postgres \
    -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid();" \
    >/dev/null 2>&1 || true

# Drop and recreate database
echo "Recreating database..."
PGPASSWORD="${PGPASSWORD:-}" psql \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d postgres \
    -c "DROP DATABASE IF EXISTS $DB_NAME;" \
    >/dev/null

PGPASSWORD="${PGPASSWORD:-}" psql \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d postgres \
    -c "CREATE DATABASE $DB_NAME;" \
    >/dev/null

# Restore database
echo "Restoring database from backup..."
if [[ "$BACKUP_FILE" == *.gz ]]; then
    gunzip -c "$BACKUP_FILE" | PGPASSWORD="${PGPASSWORD:-}" psql \
        -h "$DB_HOST" \
        -p "$DB_PORT" \
        -U "$DB_USER" \
        -d "$DB_NAME" \
        >/dev/null
else
    PGPASSWORD="${PGPASSWORD:-}" psql \
        -h "$DB_HOST" \
        -p "$DB_PORT" \
        -U "$DB_USER" \
        -d "$DB_NAME" \
        -f "$BACKUP_FILE" \
        >/dev/null
fi

# Cleanup temporary file
if [[ -n "$TEMP_FILE" ]]; then
    rm "$TEMP_FILE"
fi

# Verify restore
echo "Verifying restore..."
TABLE_COUNT=$(PGPASSWORD="${PGPASSWORD:-}" psql \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" | xargs)

echo "Tables restored: $TABLE_COUNT"

if [[ "$TABLE_COUNT" -gt 0 ]]; then
    echo "=== Restore completed successfully at $(date) ==="
    echo "Safety backup: $SAFETY_BACKUP"
    echo "You can remove the safety backup if everything works correctly."
else
    echo "Warning: No tables found in restored database"
    exit 1
fi
