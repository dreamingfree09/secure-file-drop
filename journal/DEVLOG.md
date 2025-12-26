# Secure File Drop – Development Log

This file records implementation progress and troubleshooting in chronological order.
Each entry must include: context, reproduction steps (if an issue), observed behaviour, expected behaviour, root cause (if known), resolution, and the commit hash.

---

## 2025-12-24 – Project initialisation

Context:
The repository was created and moved to a non-synchronised local path for stability. Initial documentation and folder structure were added to anchor scope and progress tracking.

Notes:
- Git initialised
- Project structure created (docs/, journal/, cmd/, internal/, web/)
- Documentation baseline established (README, SPEC, TRACKER)
- WSL 2 installed
_ Docker Desktop installed
- Firmware virtualization enabled
- Docker Authentication issue resolved
- Successfully created the MinIO bucket and access key

## 2025-12-25

Notes:
- Installed Ubuntu WSL and Go
- Implemented the backend skeleton with /health and request-id logging
- containerised the backend with Dockerfile
- enabled Docker Desktop WSL integration and resolved Docker socket permissions
- fixed MinIO healthcheck by using mc(since curl was not present)
- confirmed all services healthy with docker compose ps

## 2025-12-25

Notes:
- Admin login implementation with signed cookie sessions
- Added a protected test endpoint
- Wired seccrets via .env
- Validated via Docker Compose with all services healthy

## 2025-12-26 – Streaming upload to MinIO (pending → stored)

Context:
Implementation of the authenticated upload pipeline as part of Milestone 4. The goal was to stream files directly to private object storage without buffering, while maintaining explicit lifecycle state in PostgreSQL.

Observed behaviour:
The backend initially crash-looped on startup after MinIO client integration.

Expected behaviour:
Backend should start successfully, validate MinIO configuration, and accept authenticated uploads.

Root cause:
The MinIO Go SDK expects the endpoint as host:port, not a fully qualified URL. The environment variable was set to `http://minio:9000`, causing a panic during client initialisation.

Resolution:
Normalised the MinIO endpoint in the backend to accept either `host:port` or `http(s)://host:port`. Added explicit startup validation and fail-fast behaviour if configuration is invalid.

Outcome:
- Authenticated multipart uploads stream directly to MinIO
- File lifecycle transitions from `pending` to `stored`
- Upload failures mark records as `failed`
- Behaviour verified via API, PostgreSQL, and MinIO
- Changes committed as `38fe3bd`
