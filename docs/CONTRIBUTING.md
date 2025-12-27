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

- There are no large test suites yet; add tests in `_test.go` files and run `go test ./...`.

## Pull request

- Fork and create a feature branch.
- Rebase / keep history tidy; open a PR against `main` with a clear description and testing steps.
- Add changelog entry to `CHANGELOG.md` for notable changes.

If you'd like, I can prepare a PR with these docs and a README update — confirm whether you want me to push a branch and open a PR (I will need push access or a fork to open the PR).