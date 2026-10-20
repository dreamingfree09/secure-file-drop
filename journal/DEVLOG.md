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


### PostgreSQL fixture readiness correction

Superseded CI runs 39172914130 and 39172916797 built the actual images and passed Windows/static/vulnerability gates, but hit the PostgreSQL initialization Unix-socket readiness race before migrations. The temporary initialization server exits before the final TCP server starts. Require pg_isready on 127.0.0.1 TCP before issuing fixture DDL; do not retry partially applied DDL or suppress acceptance. Cleanup/preservation passed; repeat both layouts at the successor SHA.


### Synthetic BYTEA fixture correction

The numeric-layout full-stack suite passed 27 checks. The fresh BYTEA fixture stopped before application startup because the numeric migration check (sha256_bytes >= 0) cannot apply to BYTEA. Remove that numeric-only check solely while constructing the empty owned BYTEA fixture, then seed non-null preservation metadata and repeat acceptance. This is test-only DDL on disposable databases; no live constraint or data is changed.


### Final local acceptance and strict legacy lint gate

Dual-layout source-built acceptance passed 27 checks per layout, preserving non-null existing fixture metadata and verifying digest/byte-count writes. The unchanged f196... Windows executable passed nine normal TLS/revocation login/me/readiness/logout checks against the final application and BYTEA fixture. All owned cleanup/live preservation checks passed. Strict linter v2.14.0 with frozen reviewed config fails: 79 base findings and 79 proposal findings, none on added/modified Go lines after correcting new test issues and quoting identity diagnostics. Preserve the full failure, not a fabricated lint pass; independent Aether review and mandatory gate reconciliation remain outstanding.


### Browser cleanup diagnostic correction

CI at 2a683fd28ea54a9f069fbe9962f1da229d4e3de2 passed 27 API/full-stack/security checks and normal CI fixture trust setup, then failed its browser command. Source inspection found the cleanup step attempting a visibility-waiting fill on the now-hidden password field. Clear that owned field directly after the actual visible login assertions, and emit only sanitized phase/error-type diagnostics on failure. Do not bypass TLS, revocation, CSP or UI login. Repeat mandatory exact-head CI; no browser pass is claimed yet.


## 2026-10-19 — SFD-ADMIN-SESSION-COMPATIBILITY-20261018 quality gate correction

Codex continued the existing 75d92f2 proposal under standing Owner delegation. The inherited strict-lint backlog was corrected in a separate normal commit: checked response/row/notification errors; encoded synchronized request logs without referer/query credentials; static bound audit filters; cancellation and bounded responses in the existing webhook placeholder; private backup streaming, checked gzip/sync/close, exclusive publication and parsed connection environment. Operator native paths must be absolute regular executables without group/other write access. Native-tool gosec annotations retain the reviewed operator-only/no-shell trust boundary; the sole new permission annotation belongs to an intentionally invalid owned test fixture. No additional linter rule is disabled.

The unsafe old Go E2E body is replaced by an adapter to the same owned real PostgreSQL/storage/native/browser harness. Fresh dual-layout execution passed 28 checks per layout, including all guarded PostgreSQL race/security/quality tests without skips. Strict golangci-lint v2.14.0, formatter and vet pass; reachable govulncheck findings remain zero. The failed PGDATABASE-URI experiment and two CGO test-driver failures are retained, accurately failed, with full owned cleanup. See [sanitized quality evidence](../tests/auth/evidence/20261019-quality.json).

No live adoption, migration, credential reset, certificate/trust change, real-data operation or merge occurred. Independent exact-revision Aether acceptance remains required. The legacy audit API/migration shape mismatch, storage/backup/recovery/deployment gates, TUS and S3/email/webhook placeholders remain explicit limitations. Browser and unchanged Windows acceptance will be repeated at the final published successor; passing producer tests do not constitute Aether review.


## 2026-10-19 — SFD-LEGACY-LINT-RECONCILIATION-20261019

Codex reconciled the assignment's pinned 75d92f2 with already published successor f39faa9. Exact pinned tool/config reproduction reports 79 default diagnostics at base 2b8e56d and 75d92f2; removing count caps gives 130, and also disabling line de-duplication gives 140 (including overlapping rules/related notes). Structural comparison matches all 140: one moved expression, one edited pre-existing package header, no new defect identities or severity escalation. Full corrected configured analysis is zero; this is not a baseline-filtered pass. Four scoped gosec diagnostics remain visible when annotations are ignored, across three explicitly justified comments; they are not concealed or claimed exception-free. Existing G104 policy exclusion is unchanged. [Complete findings/classification/evidence](../tests/auth/evidence/20261019-lint-reconciliation.json) records every identity, severity, position and decision, including all historical 79-entry reports.

The uncapped/non-deduplicated CI gate is paired with seven static policy/mutation scenarios rejecting new unused/error/security findings, new suppressions, ignored Go build scope and disabled security rules. Only owned temporary snapshots are mutated; no probe source is executed. Configuration and exact allowed annotations/build tags are pinned, so expansion requires an explicit reviewed policy/test change. The active E2E signed-download flow now enforces owned HTTPS origin/path and rejects redirects rather than merely retiring the old arbitrary-URL Go helper.

A separate inherited authentication gap was found: login checked lockout state but never recorded failed/successful attempts. Two existing-method callbacks now enforce its configured policy; guarded PostgreSQL tests verify threshold, success reset, expiry and account isolation. Both real database layouts passed 30 checks each, including actual HTTP rate limiting, with all Go race/security/quality tests without skips. Native empty/abc/1MiB vectors, vet and zero-reachable advisory scan pass. A new rate-limit assertion failed due to the established header value and was corrected without changing the limiter; the failure is retained.

Integration ancestry is explicit: master 8d54031 and feature 2b8e56d share 1a0f771, with 46 master-only and 33 feature-only commits. Their migrations, administrator credential contracts and session-table names differ. Do not merge the whole feature branch into master, copy migration12 into master's migration4 lineage, downgrade master dependencies, or substitute master's backend into the live WSL stack. PR27 remains a legacy-targeted compatibility proposal.

Rollback testing disproved the prior assumption that the original live image can start at schema12: it stops on missing migration12 metadata. This startup failure is retained; synthetic records/sessions/schema were preserved and the corrected candidate was restored. The separately identified f39faa9 image cb3c849f... is a schema-compatible fallback and passed 33 owned BYTEA checks, including rollback, preservation and restored candidate login/me. Aether must explicitly accept BOTH the final successor and this fallback before any live adoption; fallback lacks the newly wired lockout callbacks and is not public production approval. Never downgrade schema metadata or run Down to make the old image start.

Read-only current backup preflight found four entries and none retention-eligible now or within one hour; no settings/files changed. Adoption requires a fresh effective-config/backup inventory covering the full maintenance window and stops if startup retention could remove an existing backup. Preserve the existing job settings; do not silently disable/re-enable them. The canonical PR handover will record exact final CI/image/Windows identities after publication. No live migration/recreation, credential reset, real-record operation, CA/CRL/trust change, protected-branch integration or GUI acceptance occurred. Independent exact-revision Aether review remains pending; production/storage/ownership/audit-schema/recovery/deployment limitations remain open.


## 2026-10-20 - SFD-FULL-E2E-ACCEPTANCE-20261020: legacy browser correction

Separate worker from exact PR27 candidate8c; no canonical/legacy application substitution or live adoption. Correct environment-administrator quota feedback for the existing registered-owner-only API; require successful authenticated identity readback before entering the UI; clear submitted sign-in/registration passwords; invalidate and cancel owned protected requests/uploads and stale queue/dashboard/link callbacks at logout or HTTP401. Preserve existing registered quotas and backend permissions, all Go authentication/schema/source and original producer branches. Extend trusted-HTTPS browser acceptance using fresh per-fixture accounts/passwords and keep actual OS clipboard/Explorer/visual acceptance unexecuted. The full GHP-01 parent acceptance handover will retain baseline failures, actual test results and remaining integration gates.


### 2026-10-20 — Real Windows upload reconciliation compatibility

The preserved F196 reviewed executable authenticated normally over trusted HTTPS but its first real upload ended at GET /files HTTP 405. Reuse the existing authenticated, UUID-owned /user/files listing for GET /files; POST metadata creation and administrator separation remain unchanged. Fresh real PostgreSQL tests require identical owned metadata, anonymous401, environment-admin403 and cross-account isolation. No live backend or existing records changed. Independent Aether review remains mandatory.


### 2026-10-20 — Legacy correction current-head CI

Extend the existing protected lint/security and real PostgreSQL BYTEA/BIGINT HTTPS acceptance workflow to the owned correction branch and its PR27 base. Preserve full lint invocation and enforcement verifier, all existing security tests and the frozen reviewed Windows transport reference. Workflow installation is a separate commit.


### 2026-10-20 — Preserved executable real Windows acceptance

Retain the exact local C# acceptance probe source used to load and exercise the unchanged F196 Framework 4.8 executable. On backend image sha256:3d774f3e4c13a3094fcbb9555010a2260f32505a48a5d2bda56f8bbca5e5c4c7, both real BYTEA and BIGINT fixtures passed 34 backend checks and 36 actual release checks. This includes normal Windows certificate/revocation validation, rejected untrusted/name-mismatched certificates, multipart/native SHA/download equality, DPAPI persistence/isolation, real server expiry, logout, account switching and an owned programmatic tray context/exit. No Owner visual/file-picker/Explorer/OS clipboard acceptance is claimed. Empty files correctly generate no byte-chunk progress events; completion is separately verified. Only new labelled synthetic database/session records were expired. Private fixture files and binaries remain local.


### 2026-10-20 — Applied audit-schema correction

Source review confirmed the unused audit helper queried resource/details and inserted a text ID against a migration defining resource_id/metadata, required event_type and BIGSERIAL ID. Map the existing helper to the applied columns and database-generated ID; no migration or historical record conversion. Replace the temporary invented-schema test with the real fresh migrated public table under the existing owner guard. Automatic operation-wide audit wiring remains absent and is not claimed or added as a new feature. Independent review is required before adoption.


### 2026-10-20 — Audit timestamp boundary correction

Real-schema bounds tests failed because migration timestamp has no timezone while the helper wrote local wall time and compared timestamptz filters. Store UTC explicitly and interpret the applied timestamp column as UTC for bounded filtering. Regression uses a non-UTC session timezone. No existing rows or schema changed.


### 2026-10-20 — Full E2E current state and retained gates

Record the exact received PR ancestry, scoped UI/API/audit corrections, real browser/Windows evidence, preserved reviewed executable, failed rehearsal evidence and pending independent acceptance. No live adoption or integration occurred. Retain the rejected fallback decision and explicit storage/deployment, schema/ownership, capacity and cross-host recovery gates. Canonical PR handover is the authoritative revision-specific report.


### 2026-10-20 — Successor-specific recovery acceptance

Extend the existing owned rollback fixture to require actual admin/registered identity, revocation, account isolation and five-failure/sixth-lockout behavior before the IP ceiling, in addition to schema and fingerprint preservation. Record actual runtime image IDs. Prepare a separately labelled artifact recovery image from the same reviewed application inputs; identical application bytes are artifact recovery only, with fail-closed stop/forward fix for shared defects. Neither image is adopted without exact Aether acceptance.


### 2026-10-20 — Actual release cancellation and backend interruption

Extend the actual F196 release probe beyond pre-cancellation: cancel during real request serialization, reject unconfirmed link creation, stop only the verified synthetic backend during another transfer, verify unavailable readiness and restart/session persistence. The opt-in private owner-marked fixture control never targets live services and rechecks loopback bindings after restart. This does not operate native picker, real Explorer or OS clipboard.


### 2026-10-20 — Actual Windows-generated link expiry

Add a bounded opt-in test waiting for the unchanged release-generated five-minute signed link to expire, then verify real HTTPS download401. This uses only a synthetic fixture link and normal certificate validation, not a changed clock, forged token or altered backend TTL. No link value is logged. Existing visual/Explorer/OS clipboard exclusions remain.


### 2026-10-20 — Exact legacy expiry status contract

The real-time expiry probe incorrectly expected canonical401; legacy download.go explicitly returns410 Gone for errTokenExpired and401 for invalid signatures. Preserve that failed assertion and require exactly410 in the rerun, with credential-free status readback. No backend status behavior changed. The earlier test description of expiry401 is superseded by this contract clarification.


## 2026-10-21 — SFD-FINAL-RECONCILIATION-20261020 selected ports

Codex chose and implemented the bounded schema12 lifecycle reconciliation under standing Owner delegation; Aether gates independent acceptance and integration, not routine implementation. Preserved existing authentication/roles/verification/UUID/quota/hash layouts and frozen c320 Windows/native source. Added profile guards, exact-key storage-first deletion/cleanup, locked upload/compensation, request bounds, explicit expiration and private-bucket startup checks. Real owned race coverage includes10 schema cases,4 driver COMMIT acknowledgement modes and5 locking/cleanup cases; raw failures/logs stay local. New runtime Alpine3.24 retains PG16 tooling. Former a565/575 images fail deletion-preservation and remain rejected for adoption; equal binaries never imply software-fault rollback. Public production/Owner GUI acceptance remain open. No live/protected changes. Final executed evidence and exact SHA are in the producer PR handover; no independent acceptance is implied.
