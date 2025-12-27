# Contributing

Thanks for your interest! This project welcomes contributions — small documentation fixes to larger features.

## Developer setup

1. Clone the repository and build with Go 1.20+.

   git clone <repo>
   cd Secure\ File\ Drop
   go build ./cmd/backend

2. Optionally use Docker Compose for a full dev stack (Postgres + MinIO):

   docker compose up -d

3. Initialize DB schema:

   psql -h localhost -U postgres -d sfd -f internal/db/schema.sql

4. Build the native hashing tool:

   make -C native

## Coding & style

- Keep functions small and testable.
- Be explicit about error handling and logging.
- Add unit tests where appropriate.

## Tests

- Run unit tests locally: `make test` or `go test ./...`.
- Run linter: `make lint` (requires `golangci-lint` installed).
- For integration/e2e tests (requires Docker), run: `go test ./tests/e2e -v`.

## Pull request

- Fork and create a feature branch.
- Rebase / keep history tidy; open a PR against the project's `master` branch with a clear description and testing steps.
- Ensure your PR includes: passing `go test`, linter checks, and updated documentation if applicable.
- Add changelog entry to `CHANGELOG.md` for notable changes.

If you'd like, I can prepare a PR with these docs and a README update — confirm whether you want me to push a branch and open a PR (I will need push access or a fork to open the PR).