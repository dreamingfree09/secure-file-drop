# Secure File Drop

[![Docs](https://img.shields.io/badge/docs-up%E2%86%92-blue)](#docs)
[![Status](https://img.shields.io/badge/status-active-brightgreen)](#status)

Secure File Drop is a lightweight, self-hosted service for authenticated file uploads and short-lived, signed downloads. It's designed to be safe to expose on the public internet from day one while remaining small and auditable.

## Quick summary

- **Modern UI**: WeTransfer-inspired interface with drag-and-drop, file type icons, and QR codes
- **User System**: Secure registration with email verification and password reset
- **File Management**: Multi-file uploads with progress tracking and auto-expiration
- **Secure Downloads**: Signed, time-limited links with optional password protection
- **Admin Dashboard**: System metrics, file search/filtering, and manual cleanup
- **Storage Quotas**: Per-user storage limits with real-time usage tracking
- **Email Notifications**: SMTP support for upload, download, and deletion alerts
- **Rate Limiting**: 100 requests/minute per IP with token bucket algorithm
- **Health Monitoring**: Comprehensive health checks and request logging

## Table of contents

- [Status](#status)
- [Technology](#technology)
- [Quickstart](#quickstart)
- [Development](#development)
- [Usage](#usage)
- [Documentation](#documentation)
- [Contributing](#contributing)

## Status

[![CI](https://github.com/dreamingfree09/secure-file-drop/actions/workflows/ci.yml/badge.svg)](https://github.com/dreamingfree09/secure-file-drop/actions)
[![Coverage](https://img.shields.io/badge/coverage-unknown-lightgrey)](https://codecov.io/gh/dreamingfree09/secure-file-drop)

This repository contains a production-ready file sharing service with 20+ features:

Docs link health: links are checked in pre-commit (local Markdown link checker) and in CI via a local checker plus Lychee for external URLs. See [.pre-commit-config.yaml](.pre-commit-config.yaml) and [.github/workflows/docs-link-check.yml](.github/workflows/docs-link-check.yml).

**Core Features:**
- Multi-file upload with drag & drop
- Email verification and password reset
- Password-protected downloads
- File expiration and auto-delete
- QR code generation for download links
- Real-time upload progress tracking

**Advanced Features:**
- User storage quotas (configurable)
- Download statistics and tracking
- File search and filtering
- Email notifications (upload, download, deletion)
- Rate limiting (100 req/min per IP)
- Comprehensive API documentation

Built with Go, PostgreSQL, MinIO, and deployed via Docker Compose.

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

3. Migrations apply automatically on backend startup. Verify readiness:

  curl http://localhost:8080/ready

4. Visit the web UI (default: http://localhost:8080) and log in using `SFD_ADMIN_USER`/`SFD_ADMIN_PASS`.

## Development

- Build the backend locally:

  go build ./cmd/backend

- Build the hashing utility:

  make -C native

- Run the server locally with environment variables set; Docker Compose is useful for a full stack dev environment.

## Usage (overview)

### Authentication & Upload Flow
- **Register**: POST /register with email verification
- **Verify Email**: GET /verify?token={token}
- **Login**: POST /login (session cookie authentication)
- **Reset Password**: POST /reset-password-request and POST /reset-password
- **Check Quota**: GET /quota (storage usage and limits)
- **Create File**: POST /files (multi-file support with TTL)
- **Upload**: POST /upload?id={file-id} (with progress tracking)
- **Create Link**: POST /links (with password and expiration options)
- **Download**: GET /download?token={signed-token}&password={optional}

### User Features
- **Drag & Drop Upload**: Multiple files with queue processing
- **File Type Icons**: Visual file type identification
- **QR Code Links**: Generate QR codes for easy mobile sharing
- **Upload History**: View all uploaded files with download stats
- **Storage Quota**: Real-time usage tracking (10GB default)
- **File Search**: Filter files by name and status
- **Email Alerts**: Notifications for uploads, downloads, and deletions

### Admin Dashboard
After logging in with admin credentials, access powerful management features:
- **System Metrics**: Real-time stats for uploads, downloads, storage, and authentication
- **File Management**: Browse all files with status, size, hash, download counts, and timestamps
- **File Search**: Filter by filename and status (pending, stored, hashed, failed)
- **Storage Monitoring**: Track total storage usage across all users
- **Manual Cleanup**: Trigger immediate cleanup of expired and failed files
- **File Deletion**: Remove individual files with automatic email notifications
- **Health Checks**: Monitor database and storage health

Admin endpoints (require authentication):
- GET /admin/files - List all files with full metadata
- DELETE /admin/files/{id} - Delete file and notify owner
- POST /admin/cleanup - Run manual cleanup job
- GET /metrics - System-wide usage statistics
- GET /quota - User storage quota information

See Security best practices for admin routes and deployment hardening in [docs/SECURITY.md](docs/SECURITY.md).

### Background Jobs
The server runs an automated cleanup job (configurable via environment):
- `SFD_CLEANUP_ENABLED=true` - Enable/disable cleanup (default: true)
- `SFD_CLEANUP_INTERVAL=1h` - How often to run (default: 1 hour)
- `SFD_CLEANUP_MAX_AGE=24h` - Delete files older than this in pending/failed states (default: 24 hours)

Refer to `docs/API.md` for comprehensive API documentation with request/response examples, `docs/EMAIL_NOTIFICATIONS.md` for SMTP configuration, and `docs/USAGE.md` for detailed usage guides.

## Frontend UX

- **My Uploads Controls**: Sorting, search, status filters, compact view. The section is collapsed by default and its collapse state persists via localStorage. Keyboard shortcuts: `/` focuses search, `e` toggles the section, `a` selects all visible, `Esc` clears selection, `r` refreshes the list.
- **Drag-and-Drop Stability**: Drop events are debounced and uploads deduplicated to prevent accidental double-initiations when dragging files over the page.
- **Quota Banner**: If `/quota` is unavailable or rate-limited, a non-blocking information banner appears with an ℹ️ tooltip. Users can dismiss the banner; dismissal persists locally and resets after a successful quota load.
 - **Proxy Tuning**: See reverse proxy examples (Traefik/Nginx) for `/quota` rate-limit guidance in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Documentation

Comprehensive documentation is available in `docs/`:

- **[API.md](docs/API.md)**: Complete API reference with 25+ endpoints
- **[EMAIL_NOTIFICATIONS.md](docs/EMAIL_NOTIFICATIONS.md)**: SMTP setup and email templates
- **[SPEC.md](docs/SPEC.md)**: Original MVP specification
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)**: System architecture and components
- **[SECURITY.md](docs/SECURITY.md)**: Session model, hashing, signed links, and best practices
- **[USAGE.md](docs/USAGE.md)**: Detailed usage examples
- **[DEPLOYMENT.md](docs/DEPLOYMENT.md)**: Production deployment guide
- **[DB_SCHEMA.md](docs/DB_SCHEMA.md)**: Database schema and migrations

## Contributing

Please read `docs/CONTRIBUTING.md` for development setup, coding style, and PR guidelines.

---

If you'd like, I can open a branch and prepare a PR with a larger docs revision (adding `docs/ARCHITECTURE.md`, `docs/USAGE.md`, `docs/API.md`, and `docs/CONTRIBUTING.md`). Reply with permission to push and open the PR or say if you prefer to review drafts first.
