# Secure File Drop

Secure File Drop is a self-hosted Go application for registered-user uploads to private MinIO storage, native C SHA-256 verification and expiring bearer download links.

Start with [continuity](docs/CONTINUITY.md), [GHP-01](docs/GHP-01.md), [agent instructions](AGENTS.md) and the [current review/validation tracker](docs/TRACKER.md). Historical readiness claims are not current production certification.

## Behavior

- Registration creates ordinary users with bcrypt passwords. They can create metadata, upload, list their own files and create links for their files.
- The environment administrator can list/delete all files, inspect metrics and trigger cleanup. A registered username does not confer administration.
- Sessions carry signed identity and role; PostgreSQL stores token hashes for logout revocation. Deactivated accounts lose access.
- File bytes are bounded, stored under generated keys, hashed by the C utility and downloaded through signed, expiring links. Anyone possessing a valid link can download until expiry.
- Old ownerless records remain administrator-only. Existing signed sessions require a new login after the session migration.

## Development and validation

Use Go 1.26.9 (the module toolchain), GCC, OpenSSL development headers, Make and Python 3.

```sh
go build -o /tmp/sfd-backend ./cmd/backend
make -C native test
go test ./... -v
go vet ./...
node --test tests/web/*.test.cjs
go test -race ./internal/... ./cmd/... -count=1
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

The full-stack package explicitly skips without opt-in; that skip is not acceptance. With a working **local** Docker engine:

```sh
SFD_E2E=1 go test ./tests/e2e -v -count=1 -timeout 25m
```

This builds the real runtime image and C tool, creates unique disposable PostgreSQL/MinIO resources with loopback ports, verifies upload/hash/download bytes, negative authorization and expiry, migration upgrade/idempotency and cleanup. Ownership labels are checked before resource removal. It does not read your .env or use existing volumes. CI runs this separately with opt-in, including real Chromium. To require that journey locally, install the pinned test-only package with `npm ci --prefix tests/browser --ignore-scripts`, install its Chromium with `node tests/browser/node_modules/playwright-core/cli.js install --only-shell chromium`, then set both `SFD_E2E=1 SFD_BROWSER=1`. The host native tool must first be built with `make -C native test` for the real COMMIT fault fixture.

For a fresh development stack, follow [deployment](docs/DEPLOYMENT.md). Never point startup migrations or cleanup at existing storage for tests. Compose uses persistent development volumes and is not the disposable test harness. Production needs a separately reviewed TLS/proxy and operations configuration.

## Documentation

- [Specification](docs/SPEC.md), [architecture](docs/ARCHITECTURE.md)
- [API](docs/API.md), [configuration and usage](docs/USAGE.md)
- [Schema](docs/DB_SCHEMA.md), [migrations](docs/MIGRATIONS.md)
- [Deployment](docs/DEPLOYMENT.md), [contributing](CONTRIBUTING.md)
- [Roadmap](docs/ROADMAP.md), [dated development log](journal/DEVLOG.md)

## Current source-build and request contract

Build the backend from the reviewed source with the pinned Go toolchain or Dockerfile.backend; the obsolete tracked root executable has been removed, and local /backend output is ignored. Build artifacts belong outside Git and must retain the source revision plus fresh dependency/runtime scan evidence.

JSON endpoints accept exactly one complete value in a total 16 KiB body (including whitespace). Uploads accept exactly one multipart file part named file, with a filename and a complete closing boundary. Other fields/parts are rejected. A bounded MIME epilogue is permitted and counted. Empty files are valid. Declared metadata size remains a preflight check; stored and hashed byte counts are authoritative.

SFD_MAX_UPLOAD_BYTES bounds file bytes. In Compose, set SFD_MAX_REQUEST_BYTES to that value plus 1048576; both backend and actual proxy use this envelope. The backend rejects mismatched configuration. The sample pair is 10485760 / 11534336. Canonical SFD_PUBLIC_BASE_URL must be an HTTP(S) origin, including its externally visible port, without credentials/path/query/fragment. The sample HTTPS port is 8443; HTTP redirect includes it. Arbitrary forwarded headers remain untrusted.

Small JSON reads have a 10-second deadline; affected SQL operations have a 5-second request-derived budget. Upload processing/body reads have 5 minutes, the native tool at most 2 minutes within that budget, and downloads 30 minutes (HTTP writes at most 31). Shutdown drains for 5 seconds, then cancels active transfers and waits within a 20-second application budget; Compose allows 25 seconds before forced termination. These are existing-workflow bounds, not demonstrated 50 GiB capacity or slow-network support.

A COMMIT error returns a server error. Compensation reacquires the row: hashed/ready or unavailable reconciliation retains bytes; only a locked pending row proves rollback and permits deletion. An unknown pending outcome can leave bytes until existing retention cleanup. Do not manually remove bytes from a possibly completed upload merely because a client saw an error.

## Temporary storage boundary

Archived MinIO remains affected and is approved only for trusted isolated synthetic development. Compose removes storage host ports and places it on a private internal network shared only with the backend. Do not attach untrusted containers, expose the API/console/peer ports, publish storage through another proxy, or use real data. This is temporary isolation, not a storage security patch. See the [dated assessment and correction/storage decision](docs/DEEP_REVIEW_20261015.md). Public deployment, storage replacement, existing-schema/ownership reconciliation, maximum-size support and whole-service recovery remain separate review gates.
