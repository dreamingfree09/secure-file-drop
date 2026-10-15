# Migration operations

The backend applies embedded forward migrations from internal/db/migrations at startup. A migration failure stops startup. The standalone early schema/alter scripts do not install the complete current schema.

For a separately verified target with an appropriate backup and authorised maintenance window:

```sh
DATABASE_URL='postgres://...verified-target...' ./scripts/run_migrations.sh
```

The script requires an explicit DSN and invokes cmd/migrate using the same embedded runner as the backend. It has no localhost/default-database fallback. Do not run it on existing databases during tests. The disposable full-stack test verifies the empty-database path, early files-only schema upgrade, and repeated forward migration.

## New migrations

Add a new numbered up/down pair. Do not edit or delete applied migrations. Review data effects and rollback before deployment. Migration 4 adds token-hash sessions; old stateless cookies require login again. Its down migration drops session records and therefore signs everyone out; it does not remove users or files.

## Existing or damaged databases

Inspect schema_migrations version/dirty state and compare the actual schema with the received revision before choosing a recovery plan. A manually installed users schema without migration history may collide with historical migration 3 indexes. This review does not auto-adopt or mutate such unknown states. Obtain a reviewed reconciliation/backup plan.

Never remove schema_migrations, force a version, delete historical SQL, or mark a dirty migration clean merely to bypass an error. Those actions can conceal partial schema changes. Preserve evidence, stop startup against that target and resolve the actual schema drift before retrying. Live rollback/restore testing is outside this isolated review.
