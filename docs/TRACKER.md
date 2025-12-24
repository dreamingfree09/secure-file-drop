# Secure File Drop – Implementation Tracker (MVP)

This tracker is the source of truth for implementation progress. A task is only considered complete when its acceptance criteria are met and a commit exists that demonstrates it.

## Milestone 0 – Repository & Documentation Baseline

Status: In progress

Acceptance criteria:
- Repository initialised with Git
- Project folders exist: docs/, journal/, cmd/, internal/, web/
- README.md exists with project overview
- docs/SPEC.md exists with MVP scope
- docs/TRACKER.md exists and is up to date

## Milestone 1 – Local Dev Environment (Docker Compose)

Status: Not started

Acceptance criteria:
- docker-compose.yml exists
- Services start successfully:
  - PostgreSQL
  - MinIO
  - Backend API (placeholder)
- A single command starts everything locally
- Health checks show containers are running

## Milestone 2 – Backend Skeleton (Go)

Status: Not started

Acceptance criteria:
- Go module initialised
- Backend builds successfully
- Basic HTTP server responds at /health
- Structured logging enabled (request id per request)

## Milestone 3 – Authentication (Single Admin)

Status: Not started

Acceptance criteria:
- Admin user is created via environment variables or a one-time bootstrap command
- Login endpoint issues a secure session (JWT or signed cookie)
- Auth middleware protects upload endpoints
- Unauthenticated requests are rejected correctly

## Milestone 4 – Upload Pipeline (Backend + MinIO)

Status: Not started

Acceptance criteria:
- Authenticated upload endpoint accepts a file
- Upload size limit enforced (backend)
- File stored in MinIO (private bucket)
- Metadata stored in PostgreSQL
- Upload returns file id and basic metadata

## Milestone 5 – Integrity Utility (C) + Integration

Status: Not started

Acceptance criteria:
- C utility compiles on your machine
- Utility computes SHA-256 for an input file
- Backend calls the utility during upload
- SHA-256 is stored in PostgreSQL for each file
- Upload fails cleanly if hashing fails

## Milestone 6 – Signed Download Links + Expiry

Status: Not started

Acceptance criteria:
- Backend generates a signed token for a file id with expiry time
- Download endpoint validates token and expiry
- Download streams file from MinIO
- Invalid or expired tokens are rejected correctly

## Milestone 7 – Minimal Web UI (Upload + Link Display)

Status: Not started

Acceptance criteria:
- User can log in
- User can upload a file
- User receives a download link
- UI displays recent uploads

## Milestone 8 – Public Exposure Hardening (Day-One Safe)

Status: Not started

Acceptance criteria:
- Reverse proxy configured for HTTPS-only
- Rate limiting configured
- Max body size configured at proxy
- Basic security headers enabled
- Cloudflare fronting confirmed (DNS + proxy)

## Milestone 9 – Journal & Troubleshooting Discipline

Status: Not started

Acceptance criteria:
- journal/DEVLOG.md exists
- At least one entry created in the required format (template)
- Any troubleshooting performed is captured with outcome and commit references
