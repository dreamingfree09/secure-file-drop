#!/usr/bin/env bash
set -euo pipefail

: ${PGHOST:=localhost}
: ${PGPORT:=5432}
: ${PGUSER:=postgres}
: ${PGDATABASE:=sfd}

echo "Applying migrations from internal/db/*.sql to ${PGUSER}@${PGHOST}:${PGPORT}/${PGDATABASE}"
for f in internal/db/*.sql; do
  echo "-- applying $f"
  psql "postgresql://${PGUSER}@${PGHOST}:${PGPORT}/${PGDATABASE}" -f "$f"
done

echo "Migrations applied."