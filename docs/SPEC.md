# Secure File Drop – MVP Specification

## Purpose

Secure File Drop is a public-facing application that allows authenticated users to upload files and generate secure, time-limited download links. The system is designed to be safe to expose on the public internet from day one, while remaining simple enough to implement incrementally.

This project is also an educational exercise, combining systems programming in C with modern backend and web development practices.

## MVP Scope (Strict)

The MVP MUST include:
- Authenticated file uploads
- Private object storage (not directly exposed)
- Signed, expiring download links
- Server-side file integrity verification
- HTTPS-only access
- Rate limiting and size limits
- Basic audit logging

The MVP MUST NOT include:
- Public anonymous uploads
- User-to-user sharing or permissions
- Client-side encryption
- Resumable uploads
- Antivirus scanning
- Folder hierarchies
- Public object storage access

Anything outside this list is explicitly out of scope for v1.

## High-Level Architecture

- Reverse proxy terminates TLS and enforces request limits
- Backend API handles authentication, uploads, downloads, and metadata
- Object storage stores file blobs privately
- Database stores file metadata and link state
- C utility computes cryptographic hashes for integrity checks

## Components

### Backend API (Go)
Responsibilities:
- User authentication
- Upload handling
- Download token validation
- Metadata persistence
- Calling the C hashing utility
- Streaming downloads safely

### Integrity Utility (C)
Responsibilities:
- Read files in a streaming manner
- Compute SHA-256 hashes
- Output results in a machine-readable format
- Fail safely on invalid input

This component exists to teach:
- File I/O
- Memory discipline
- Process execution
- Interoperability with higher-level languages

### Storage
- Object storage: MinIO (S3-compatible)
- Database: PostgreSQL

Object storage must never be publicly accessible.

## Security Constraints

- HTTPS is mandatory
- All uploads require authentication
- Maximum upload size enforced at proxy and backend
- Download links must be signed and time-limited
- Files are streamed, never fully loaded into memory
- Secrets are provided via environment variables only

## Deployment (Initial)

- Docker Compose
- Local development with public exposure via Cloudflare Tunnel or reverse proxy
