# Secure File Drop – MVP Specification

## Table of contents

- [Purpose](#purpose)
- [MVP scope](#mvp-scope)
- [High-level architecture](#high-level-architecture)
- [Components](#components)
- [Security constraints](#security-constraints)
- [Environment variables (required/important)](#environment-variables-requiredimportant)
- [Deployment (initial)](#deployment-initial)

## Purpose

Secure File Drop is a public-facing application that allows authenticated users to upload files and generate secure, time-limited download links. The system is designed to be safe to expose on the public internet from day one, while remaining simple and auditable.

This project is an educational exercise that combines a memory-safe backend (Go) with a small C utility for efficient, low-level hashing.

## MVP scope

**Must include**:
- Authenticated file uploads with server-side metadata
- Private object storage (MinIO) not exposed to the public internet
- Signed, expiring download links
- Server-side file integrity verification (SHA-256)
- HTTPS-only access (enforced at the reverse proxy)
- Rate limiting and maximum upload size
- Basic audit logging for uploads and downloads

**Explicitly out of scope for v1**:
- Public anonymous uploads
- User-to-user sharing or fine-grained permissions
- Client-side encryption
- Resumable uploads
- Antivirus scanning
- Folder hierarchies in storage
- Public object storage access

## High-Level Architecture

- Reverse proxy terminates TLS and enforces request limits
- Backend API handles authentication, upload lifecycle, link generation, and metadata
- Object storage stores file blobs privately; backend streams uploads to object storage
- Database (Postgres) stores file metadata, states, and audit records
- C hashing utility computes SHA-256 to provide an auditable integrity check

## Components

### Backend API (Go)
- Login (/login)
- Create file record (/files)
- Upload file (/upload?id=<uuid>) – multipart form, field `file`
- Create signed download link (/links)
- Download via signed token (/download?token=...)

### Integrity Utility (C)
- Stream-based SHA-256
- Intended to be small, auditable, and fast

### Storage
- MinIO for object store (S3-compatible)
- PostgreSQL for metadata and state

## Security constraints

- TLS is mandatory in production (reverse proxy)
- Admin credentials, session secret, and download secret must be kept secret
- MinIO and Postgres must not be public
- Limit upload sizes at both proxy and server

## Environment variables (required / important)

- SFD_ADMIN_USER, SFD_ADMIN_PASS
- SFD_SESSION_SECRET
- SFD_DOWNLOAD_SECRET
- SFD_MINIO_ENDPOINT, SFD_MINIO_ACCESS_KEY, SFD_MINIO_SECRET_KEY, SFD_MINIO_BUCKET
- SFD_DB_DSN
- SFD_PUBLIC_BASE_URL (optional but recommended in deployments)
- SFD_MAX_UPLOAD_BYTES (optional)

## Deployment (initial)

- Docker Compose (development and simple production)
- Use a reverse proxy (Caddy, Nginx, Traefik) to manage TLS and public endpoints
- In production, consider secret management for credentials and secrets

---

If you'd like, I can expand this with diagrams, sequence diagrams for the upload flow, or configuration examples for common reverse proxies.
