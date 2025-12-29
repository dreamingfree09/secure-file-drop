# Contributing

Thanks for your interest! This project welcomes contributions — small documentation fixes to larger features.

## Developer setup

1. Clone the repository and build with Go 1.20+.

   git clone <repo>
   cd Secure\ File\ Drop
   go build ./cmd/backend

2. Optionally use Docker Compose for a full dev stack (Postgres + MinIO):

   docker compose up -d

3. Database migrations:

   Migrations are embedded and auto-applied on backend startup via golang-migrate. Ensure `DATABASE_URL` is set, then start the backend.

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

## Pre-commit hooks (optional)

Use the `pre-commit` framework to run quick checks before each commit:

- Install: `pip install pre-commit` (or see https://pre-commit.com/#install)
- Enable in this repo: `pre-commit install`
- The configuration in `.pre-commit-config.yaml` runs the local docs link checker (`scripts/link-check.sh`) against `README.md` and `docs/*.md` to prevent broken cross-links.

## Docs Link Checks (CI)

External links in `README.md` and `docs/*.md` are verified in CI using Lychee with repo-specific configuration:

- See `lychee.toml` for exclusions (e.g., `localhost`, placeholder domains like `yourdomain.com`) and accepted redirects (3xx).
- CI workflow: `.github/workflows/docs-link-check.yml` runs both the local checker and Lychee.
- Run Lychee locally via Docker:

   ```bash
   cd "/home/dreamingfree09/Secure File Drop"
   docker run --rm -v "$PWD":/work -w /work lycheeverse/lychee \
      --no-progress --max-concurrency 4 -c lychee.toml README.md docs/**/*.md
   ```

This avoids false positives for local development endpoints and example URLs while still ensuring public links remain healthy.

## Pull request

- Fork and create a feature branch.
- Rebase / keep history tidy; open a PR against the project's `master` branch with a clear description and testing steps.
- Ensure your PR includes: passing `go test`, linter checks, and updated documentation if applicable.
- If you modified docs, run `bash scripts/link-check.sh` to verify local cross-links before submitting.
- Add changelog entry to `CHANGELOG.md` for notable changes.

If you'd like, I can prepare a PR with these docs and a README update — confirm whether you want me to push a branch and open a PR (I will need push access or a fork to open the PR).