#!/usr/bin/env bash
set -euo pipefail
: "${DATABASE_URL:?Set DATABASE_URL explicitly after verifying the intended database and backup}"
cd "$(dirname "$0")/.."
go run ./cmd/migrate
