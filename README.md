# Secure File Drop

Secure File Drop is a public-facing, self-hosted application that allows authenticated users to upload files and generate secure, time-limited download links.

The project is designed as an educational but production-oriented system, focusing on:
- Secure public deployment
- Clean backend architecture
- Practical use of C for performance-critical components
- Modern backend and web technologies
- Strong documentation and traceability

## Core Goals

- Upload files securely via a web interface
- Store files privately in object storage
- Generate signed, expiring download links
- Enforce size limits, rate limits, and authentication
- Verify file integrity using a C-based hashing utility

## Technology Overview (Planned)

- Backend API: Go
- Integrity Utility: C (SHA-256 hashing, later chunking)
- Database: PostgreSQL
- Object Storage: MinIO (S3-compatible)
- Reverse Proxy & TLS: Caddy (behind Cloudflare)
- Frontend: Web UI (initially minimal)
- Deployment: Docker Compose (initially)

## Project Structure

- `docs/` – Specifications, API contracts, and planning documents
- `journal/` – Development log and troubleshooting history
- `cmd/` – Application entry points
- `internal/` – Internal backend packages
- `web/` – Frontend assets and UI
