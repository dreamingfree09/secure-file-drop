# Database Migration Guide

This document explains how to manage database migrations for Secure File Drop.

## Overview

We use [golang-migrate](https://github.com/golang-migrate/migrate) for database schema versioning and migrations. Migrations are automatically applied when the backend starts.

## Migration Files

Migration files are located in `internal/db/migrations/` and follow this naming convention:

```
{version}_{description}.{up|down}.sql
```

For example:
- `000001_initial_schema.up.sql` — Creates the initial schema
- `000001_initial_schema.down.sql` — Rolls back the initial schema

## Automatic Migrations

The backend automatically runs pending migrations on startup:

```go
// In cmd/backend/main.go
if err := db.RunMigrations(dbConn); err != nil {
    log.Printf("service=backend msg=%q err=%v", "migration_failed", err)
    os.Exit(1)
}
```

**Migration behavior:**
- ✅ Applies all pending migrations in order
- ✅ Skips if already at latest version (no-op)
- ❌ Exits on migration failure (fail-fast)

## Creating New Migrations

### 1. Install golang-migrate CLI (optional, for manual operations)

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### 2. Create migration files

Create two files for each migration (up and down):

```bash
# Choose next version number (e.g., 000003)
VERSION=000003
DESCRIPTION="add_user_roles"

# Create up migration
cat > internal/db/migrations/${VERSION}_${DESCRIPTION}.up.sql << 'EOF'
BEGIN;

-- Your schema changes here
ALTER TABLE files ADD COLUMN user_id UUID;
CREATE INDEX idx_files_user_id ON files(user_id);

COMMIT;
EOF

# Create down migration (rollback)
cat > internal/db/migrations/${VERSION}_${DESCRIPTION}.down.sql << 'EOF'
BEGIN;

-- Reverse your changes
DROP INDEX IF EXISTS idx_files_user_id;
ALTER TABLE files DROP COLUMN IF EXISTS user_id;

COMMIT;
EOF
```

### 3. Test the migration

Restart the backend to apply:

```bash
docker-compose restart backend
```

Check logs for migration success:

```bash
docker-compose logs backend | grep migration
```

## Manual Migration Operations

### Check Current Version

```bash
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        version
```

### Apply All Pending Migrations

```bash
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        up
```

### Rollback Last Migration

```bash
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        down 1
```

### Rollback to Specific Version

```bash
# Rollback to version 2
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        goto 2
```

### Force Version (for broken migrations)

⚠️ **Use with caution** — only when migration state is corrupted:

```bash
# Set version to 2 without running migrations
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        force 2
```

## Rollback Procedures

### Scenario 1: Rollback During Development

If you just applied a migration and need to undo it:

```bash
# Method 1: Use migrate CLI
migrate -path internal/db/migrations \
        -database "$DATABASE_URL" \
        down 1

# Method 2: Restart backend after removing migration files
rm internal/db/migrations/000003_*
docker-compose restart backend
```

### Scenario 2: Rollback in Production

**Before deploying a migration:**

1. **Test the rollback in staging:**
   ```bash
   # Apply migration
   migrate up 1
   
   # Test your app
   
   # Rollback
   migrate down 1
   
   # Verify app still works
   ```

2. **Document the rollback in your deployment plan:**
   ```
   Deployment: Add user_id column (migration 000003)
   Rollback:   migrate down 1
   Impact:     Zero downtime (column is nullable)
   ```

**During a production incident:**

1. **Stop the backend:**
   ```bash
   docker-compose stop backend
   ```

2. **Rollback the migration:**
   ```bash
   migrate -path internal/db/migrations \
           -database "$DATABASE_URL" \
           down 1
   ```

3. **Revert to previous backend version:**
   ```bash
   git checkout <previous-tag>
   docker-compose up -d backend
   ```

4. **Verify rollback:**
   ```bash
   psql $DATABASE_URL -c "SELECT version, dirty FROM schema_migrations;"
   ```

### Scenario 3: Migration Failed Mid-Execution

If a migration fails partway through:

1. **Check the dirty flag:**
   ```bash
   psql $DATABASE_URL -c "SELECT version, dirty FROM schema_migrations;"
   ```

2. **If dirty=true, the migration is incomplete:**
   
   **Option A: Fix forward (recommended)**
   ```bash
   # Manually complete the migration
   psql $DATABASE_URL < internal/db/migrations/000003_description.up.sql
   
   # Mark as clean
   psql $DATABASE_URL -c "UPDATE schema_migrations SET dirty = false;"
   ```

   **Option B: Force version and retry**
   ```bash
   # Force to previous version
   migrate force 2
   
   # Manually clean up partial changes if needed
   psql $DATABASE_URL
   # > DROP TABLE IF EXISTS new_table;
   # > \q
   
   # Retry migration
   migrate up 1
   ```

## Best Practices

### ✅ DO:
- Always write both `.up.sql` and `.down.sql` for every migration
- Wrap migrations in `BEGIN;` ... `COMMIT;` transactions
- Test rollback before deploying to production
- Use `IF EXISTS` / `IF NOT EXISTS` for idempotency
- Make migrations backward-compatible when possible (add nullable columns, not required ones)
- Document breaking changes in the migration file header

### ❌ DON'T:
- Never edit a migration file after it's been applied
- Don't delete old migration files (breaks version history)
- Avoid destructive operations without backups (`DROP TABLE`, `ALTER COLUMN DROP`)
- Don't skip version numbers
- Never commit migrations that haven't been tested locally

## Migration File Template

```sql
-- Migration: {version}_{description}
-- Author: {your-name}
-- Date: {YYYY-MM-DD}
-- Description: {what this migration does and why}
-- Rollback Impact: {what happens when rolled back}

BEGIN;

-- Add your schema changes here
-- Example:
-- CREATE TABLE new_table (...);
-- ALTER TABLE existing_table ADD COLUMN new_col TEXT;

COMMIT;
```

## Troubleshooting

### "Dirty database version"

**Cause:** Migration failed mid-execution

**Fix:**
```bash
# Check current state
psql $DATABASE_URL -c "SELECT version, dirty FROM schema_migrations;"

# Manually fix the schema, then mark clean
psql $DATABASE_URL -c "UPDATE schema_migrations SET dirty = false;"
```

### "No change" when running up/down

**Cause:** Already at target version

**Fix:** Check current version:
```bash
migrate -path internal/db/migrations -database "$DATABASE_URL" version
```

### Backend won't start after migration

**Cause:** Migration broke the schema or app compatibility

**Fix:**
```bash
# Stop backend
docker-compose stop backend

# Rollback migration
migrate -path internal/db/migrations -database "$DATABASE_URL" down 1

# Revert code
git revert <commit-hash>

# Restart
docker-compose up -d backend
```

## Emergency Recovery

If migrations are completely broken:

1. **Backup the database:**
   ```bash
   docker-compose exec postgres pg_dump -U sfd sfd > backup.sql
   ```

2. **Reset migration state:**
   ```bash
   psql $DATABASE_URL -c "DROP TABLE schema_migrations;"
   ```

3. **Re-apply from scratch:**
   ```bash
   migrate -path internal/db/migrations -database "$DATABASE_URL" up
   ```

4. **Verify data integrity:**
   ```bash
   psql $DATABASE_URL -c "SELECT COUNT(*) FROM files;"
   ```

## Additional Resources

- [golang-migrate documentation](https://github.com/golang-migrate/migrate)
- [PostgreSQL ALTER TABLE reference](https://www.postgresql.org/docs/current/sql-altertable.html)
- [Zero-downtime migrations guide](https://www.braintreepayments.com/blog/safe-operations-for-high-volume-postgresql/)
