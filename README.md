# Secure File Drop

[![Docs](https://img.shields.io/badge/docs-up%E2%86%92-blue)](#docs)
[![Status](https://img.shields.io/badge/status-active-brightgreen)](#status)

Secure File Drop is a lightweight, self-hosted service for authenticated file uploads and short-lived, signed downloads. It's designed to be safe to expose on the public internet from day one while remaining small and auditable.

## Quick summary

- Users authenticate with a single admin username/password (session cookie) to upload files
- Files are stored privately in S3-compatible object storage (MinIO)
- The server verifies integrity using a native C hashing utility and stores SHA-256 metadata
- Download links are signed and time-limited

## Table of contents

- [Status](#status)
- [Technology](#technology)
- [Quickstart](#quickstart)
- [Development](#development)
- [Usage](#usage)
- [Documentation](#documentation)
- [Contributing](#contributing)

## Status

This repository contains an MVP-ready backend written in Go, a small web UI, a C-based hashing utility in `native/`, and deployment infrastructure using Docker Compose.

## Technology

- Backend: Go
- Integrity utility: C (SHA-256)
- Database: PostgreSQL
- Object storage: MinIO (S3-compatible)
- Reverse proxy: Caddy (recommended)
- Deployment: Docker Compose

## Quickstart (Docker Compose)

1. Copy `docker-compose.yml` and set required environment variables (see `docs/USAGE.md` for a full list).
2. Start services:

   docker compose up -d

3. Initialize database schema (example using `psql`):

   psql -h localhost -U postgres -d sfd -f internal/db/schema.sql

4. Visit the web UI (default: http://localhost:8080) and log in using `SFD_ADMIN_USER`/`SFD_ADMIN_PASS`.

## Development

- Build the backend locally:

  go build ./cmd/backend

- Build the hashing utility:

  make -C native

- Run the server locally with environment variables set; Docker Compose is useful for a full stack dev environment.

## Usage (overview)

- Authenticate: POST /login with JSON {"username":"...","password":"..."}
- Create file metadata: POST /files (JSON with orig_name, content_type, size_bytes)
- Upload: POST /upload?id=<file-id> as multipart form field `file`
- Create link: POST /links with JSON {"id": "<file-id>", "ttl_seconds": 300}
- Download: GET /download?token=<signed-token>

Refer to `docs/USAGE.md` and `docs/API.md` for detailed examples and request/response samples.

## Documentation

Primary docs live in `docs/` — see `docs/SPEC.md` for the MVP specification and `docs/ARCHITECTURE.md` for component-level notes.

## Contributing

Please read `docs/CONTRIBUTING.md` for development setup, coding style, and PR guidelines.

---

If you'd like, I can open a branch and prepare a PR with a larger docs revision (adding `docs/ARCHITECTURE.md`, `docs/USAGE.md`, `docs/API.md`, and `docs/CONTRIBUTING.md`). Reply with permission to push and open the PR or say if you prefer to review drafts first.
