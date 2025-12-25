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