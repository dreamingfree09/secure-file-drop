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

## 2025-12-26 – Milestone 5: Integrity hashing integrated (C utility + backend)

Context:
After completing the upload pipeline to MinIO (pending -> stored), the next step was to implement server-side integrity verification. The project includes a native C SHA-256 utility to practise safe streaming file I/O and interoperability with the Go backend.

Observed behaviour:
The backend successfully stored uploaded objects in MinIO, then computed SHA-256 server-side and persisted the results in PostgreSQL.

Expected behaviour:
For a valid upload, the system should transition a file record from stored -> hashed, record sha256_hex, store the raw 32-byte digest, and record the number of bytes hashed.

Resolution:
- Built the native hashing tool inside the backend Docker build using Alpine (musl) + OpenSSL headers and linked against libcrypto.
- Ensured /app/sfd-hash is present in the runtime image and executable.
- Integrated hashing so the backend computes SHA-256 after storage and updates the database to status=hashed, persisting sha256_hex, sha256_bytes, and hashed_bytes.
- Verified end-to-end via API upload followed by SQL query confirming status=hashed and octet_length(sha256_bytes)=32.


## 2025-12-26 – Milestone 6: Signed download links with expiry

Context:
Following successful server-side integrity hashing, the next objective was to enable secure, time-limited file downloads without exposing the object storage layer directly. The goal was to allow authenticated users to generate expiring download links that could be safely shared.

Observed behaviour:
The backend generates signed URLs containing an HMAC-protected token encoding the file ID and expiry timestamp. Download requests validate the token, enforce expiry, and stream the object from MinIO to the client.

Expected behaviour:
A valid token should allow download until expiry, after which access must be denied. Token tampering must invalidate the request. No authentication cookie should be required for the download endpoint.

Resolution:
- Introduced a download signing secret via SFD_DOWNLOAD_SECRET.
- Implemented compact HMAC-SHA256 tokens with base64url encoding.
- Added a protected endpoint to create expiring download links.
- Added a public download endpoint that validates tokens, checks expiry, and streams content from MinIO.
- Verified behaviour by generating a link, downloading the file successfully, and confirming expiry enforcement.

Outcome:
Files transition cleanly through the lifecycle and can now be distributed securely via signed, time-limited links without exposing storage credentials.


## 2025-12-26 – Milestone 7: Minimal web UI served by backend

Context:
After implementing signed download links, the next step was to provide a minimal front-end entry point for basic interaction and manual verification, without introducing a full SPA build pipeline.

Observed behaviour:
The backend initially returned 404 for `/` because the static files were not present in the runtime image.

Expected behaviour:
`GET /` should return the bundled `index.html` and `/static/*` should serve assets from the container filesystem.

Root cause:
The `web/` directory was only copied into the build stage. The runtime stage did not contain `/app/web/static`, so `http.ServeFile` pointed to a non-existent path.

Resolution:
Copied `/src/web` from the build stage into `/app/web` in the runtime stage, and wired handlers in the server mux to serve `/` and `/static/*`. Verified by inspecting container paths and confirming `GET /` returns 200 with the HTML payload.


## 2025-12-27 – Milestone 9: Reverse proxy hardening and public exposure validation

Context:
With the backend and storage layers complete, the next objective was to harden public exposure by introducing a reverse proxy in front of the application. The goal was to ensure HTTPS-only access, remove direct backend exposure, and enforce basic safety controls at the edge before any real internet deployment.

Observed behaviour:
Prior to this milestone, the backend was directly published on the host port and relied solely on application-level controls.

Expected behaviour:
All public traffic should terminate at a reverse proxy. The backend must not be directly reachable. HTTPS should be enforced, basic security headers applied, request body size limits enforced at the proxy layer, and abusive request patterns throttled.

Resolution:
- Added a reverse proxy service (Traefik v2) to the Docker Compose stack.
- Removed direct host port publishing from the backend service.
- Configured HTTP to HTTPS redirection at the proxy.
- Enabled TLS termination for proxied traffic.
- Applied basic security headers (HSTS, X-Content-Type-Options, X-Frame-Options, XSS protection, Referrer-Policy).
- Configured proxy-level rate limiting.
- Configured proxy-level request body buffering and maximum body size enforcement.
- Ensured all public access routes exclusively through the proxy.

Validation:
- Direct access to the backend on localhost:8080 fails.
- HTTP requests to the proxy are redirected to HTTPS.
- HTTPS requests to the proxy successfully reach the backend.
- Security headers are present on proxied responses.
- Burst request testing produces HTTP 429 responses, confirming rate limiting.
- Oversized request testing produces HTTP 413 responses, confirming proxy-level body size enforcement.

Outcome:
The application is now safe to expose behind a reverse proxy, with critical edge protections validated via real requests rather than configuration inspection alone.

Changes implemented and committed as 1a96baf.
---

## 2025-12-28 – Modern UI Redesign (WeTransfer-inspired)

Context:
After completing the core backend functionality, the next step was to improve the user experience with a modern, professional interface inspired by WeTransfer's clean design.

Observed behaviour:
The frontend was functional but minimal. The new design features animated gradients, drag-and-drop upload, progress bars with shimmer effects, and responsive mobile layout.

Expected behaviour:
Users should experience a visually appealing, modern interface with smooth animations, clear visual feedback during uploads, and easy-to-use file sharing.

Resolution:
- Redesigned the entire UI with animated gradient background (purple/pink theme)
- Integrated Inter font from Google Fonts for professional typography
- Implemented drag-and-drop file upload with visual feedback
- Added progress bars with shimmer animations during upload
- Created modern card-based layout with rounded corners and shadows
- Added one-click copy-to-clipboard functionality for download links
- Implemented responsive design for mobile devices
- Styled admin dashboard with metrics grid and file management table
- Used CSS custom properties for consistent theming
- Added smooth transitions and hover effects throughout

Validation:
- UI loads with animated gradient background
- Drag-and-drop upload works seamlessly
- Progress indicators show clear upload status
- Download links can be copied with single click
- Admin dashboard displays metrics in clean grid layout
- Mobile layout adapts properly to smaller screens

Outcome:
The application now has a modern, professional interface that significantly improves user experience while maintaining all existing functionality.

---

## 2025-12-28 – User Registration System

Context:
With the modern UI in place, the next objective was to expand from single-admin authentication to support multiple users with individual accounts and secure password storage.

Observed behaviour:
Previously, only a single admin account existed (via environment variables). The new system allows users to register accounts with email/username/password, stored securely in the database.

Expected behaviour:
Users should be able to register new accounts through the UI, with comprehensive validation and secure password hashing. Authentication should support both new database users and legacy admin credentials.

Resolution:
- Created database migration (000003_add_users_table) with users table:
  - UUID primary key for user IDs
  - Unique email and username constraints
  - bcrypt password hashing (cost factor 12)
  - Timestamps for created_at and updated_at
  - Indexes on email and username for performance
- Added user_id foreign key to files table for user association
- Implemented comprehensive server-side validation:
  - Email: RFC-compliant regex pattern validation
  - Username: 3-50 characters, alphanumeric + underscore only
  - Password: minimum 8 characters, must contain letters AND numbers
- Created POST /register endpoint with validation and bcrypt hashing
- Updated authentication to support both database users and legacy admin (backward compatible)
- Designed registration form UI matching modern aesthetic:
  - Toggle between login and registration screens
  - Client-side validation with helpful error messages
  - Password confirmation field
  - Success message with auto-redirect to login
- Added bcrypt dependency (golang.org/x/crypto/bcrypt)

Validation:
- Registration form validates all inputs client-side before submission
- Server enforces email format, username constraints, and password strength
- Passwords are hashed with bcrypt before database storage (never stored plaintext)
- Duplicate email or username returns appropriate error
- Successful registration creates user and redirects to login
- Database authentication works alongside legacy admin credentials
- UI seamlessly toggles between login and registration forms

Outcome:
The application now supports multi-user registration with industry-standard security practices, while maintaining backward compatibility with existing admin authentication.

---

## 2025-12-28 – Large File Support and Real-time Upload Progress

Context:
With the core functionality complete, the next objective was to support large file transfers (up to 50GB) and provide users with real-time feedback during uploads.

Observed behaviour:
Previously, the upload limit was set to 10MB and the UI used a simple fetch API with staged progress indicators (30%, 50%, 80%). Users had no visibility into actual upload progress for large files.

Expected behaviour:
The system should support files up to 50GB with real-time upload progress showing exact bytes transferred and percentage complete. Downloads should include proper headers for browser progress tracking.

Resolution:
- Increased SFD_MAX_UPLOAD_BYTES from 10MB to 50GB (53,687,091,200 bytes)
- Replaced fetch API with XMLHttpRequest to access upload progress events
- Implemented real-time progress tracking:
  - Shows exact megabytes transferred (e.g., "Uploading... 245.3MB / 512.0MB (48%)")
  - Progress bar updates dynamically from 50% to 80% based on actual upload progress
  - Calculates and displays percentage complete
- Extended download timeout from 5 minutes to 30 minutes for large files
- Content-Length headers enable native browser download progress
- Updated documentation (FRONTEND.md, API.md, USAGE.md) to reflect changes

Validation:
- Upload progress shows real-time MB transferred and percentage
- Large files up to 50GB can be uploaded (configurable)
- Download timeout accommodates 30-minute transfers
- Browser download UI shows progress natively

Outcome:
The application now supports enterprise-scale file transfers with professional progress tracking, providing users with clear visibility into upload and download status.

## 2026-10-18 — SFD-ADMIN-SESSION-COMPATIBILITY-20261018 (Codex producer; Aether review pending)

Owner delegation authorizes the backend correction. Dedicated worker is based explicitly on the preserved live feature/v2-enhancements at 2b8e56de4b79e2508125ca02c18455ae88dee7f6. Environment administrator /me used a name as a database UUID. Added signed principal separation, persisted hashed sessions and logout revocation, additive migration 12, role/owner isolation regressions, owner checks on legacy transfer routes and the discovered native byte-count correction. Corrected reachable pgx/x-text dependencies. Source and sanitized tests are reviewable; current docs/CONTINUITY.md and the exact-revision worker PR record evidence and limitations. Windows source c320861536914623ea22f28c1d4ea156d4c2132a and distributed executable f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca remain unchanged. Live backend/schema/configuration/data, all other services and TLS/credentials are preserved. Independent exact-SHA Aether review and mandatory CI precede any live backend adoption; actual Owner Windows acceptance remains pending. No integration or public production readiness is claimed.


### Legacy session CI build correction

Initial published revision 19650318fb05878ef7ecf63f99d437a4739b44d1 passed Windows and Go/vet/vulnerability jobs, but the old Go 1.24 Docker builder rejected the corrected dependency Go minimum with GOTOOLCHAIN=local. Pin the builder to reviewed Go 1.26.9/Alpine 3.24; keep the runtime stage unchanged and repeat mandatory CI. Failed runs 39172424115 and 39172488862 are retained. No live adoption occurred.


### Browser fixture policy alignment

Pre-execution source review found the legacy UI explicitly disallows framing. Replace the assignment-created iframe fixture with direct top-level Playwright UI login, reading synthetic credentials only from the private fixture input. Existing frame/CSP protection remains intact. Superseded CI revisions remain retained and are not acceptance.


### Expired session review correction

Address automated review finding discussion_r4309050562: reclaim at most 256 expired auth_sessions records per login, using indexed expiry and SKIP LOCKED while preserving active sessions. Add real PostgreSQL regressions for bounded reclamation and active-session survival. Independent Aether acceptance remains pending; no live session/database was changed.


### Live hash metadata schema reconciliation

Read-only actual metadata confirms schema version 11 clean, users UUID/roles and files.user_id UUID, but files.sha256_bytes is BYTEA rather than the embedded BIGINT definition. Preserve the live type and all stored values. Resolve the actual column type before upload side effects; write raw digest for BYTEA, byte count for BIGINT, reject unknown types. Require both isolated layouts with non-null sentinel file/user/hash preservation checks. No live migration, data conversion or backend recreation occurred.
