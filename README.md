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
go test -race ./internal/... ./cmd/... -count=1
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

The full-stack package explicitly skips without opt-in; that skip is not acceptance. With a working **local** Docker engine:

```sh
SFD_E2E=1 go test ./tests/e2e -v -count=1 -timeout 15m
```

This builds the real runtime image and C tool, creates unique disposable PostgreSQL/MinIO resources with loopback ports, verifies upload/hash/download bytes, negative authorization and expiry, migration upgrade/idempotency and cleanup. Ownership labels are checked before resource removal. It does not read your .env or use existing volumes. CI runs this separately with opt-in.

For a fresh development stack, follow [deployment](docs/DEPLOYMENT.md). Never point startup migrations or cleanup at existing storage for tests. Compose uses persistent development volumes and is not the disposable test harness. Production needs a separately reviewed TLS/proxy and operations configuration.

## Documentation

- [Specification](docs/SPEC.md), [architecture](docs/ARCHITECTURE.md)
- [API](docs/API.md), [configuration and usage](docs/USAGE.md)
- [Schema](docs/DB_SCHEMA.md), [migrations](docs/MIGRATIONS.md)
- [Deployment](docs/DEPLOYMENT.md), [contributing](CONTRIBUTING.md)
- [Roadmap](docs/ROADMAP.md), [dated development log](journal/DEVLOG.md)
