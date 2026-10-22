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
## 2026-10-15 — SFD-QUALITY-CLOSEOUT-20261015-CODEX

Owner-authorised master-based review found ordinary-session access to administrative functions and missing file ownership enforcement, alongside session, expiry, lifecycle and configuration defects. Baseline security regressions reproduce the affected boundaries. Fixes and actual outcomes are maintained in [the current tracker](../docs/TRACKER.md); [continuity](../docs/CONTINUITY.md) records the preserved older checkout, workflow installation, source-only MinIO decision and recovery state. The owned [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21) is the exact-revision handover and final validation evidence. No master merge or public deployment occurred.


## 2026-10-16 — RustFS preparation and synthetic migration rehearsal

Work ID SFD-RUSTFS-ADOPTION-20261016-CODEX; Owner-approved bounded 12-path implementation subject to Aether's PR #23 conditions. Exact base 0349bc0c8f6b7f9b34dc8b68d850efa946c79950; separate producer branch codex/sfd-rustfs-adoption-preparation-20261016 targets the existing evaluation branch. Implementation 92cba68c2dd6969fccbe58337218d874d8d3c2a5; separate workflow installation db87d5c37000d33c4f7560436063fae7e8a798c3; lint refinement ec1e47f7fb01656eb0f65f1dbee374c7cb861f82.

Added immutable-pinned RustFS 1.0.2 preparation configuration, independently generated scoped identities and bucket policy, nested-helper dependency hygiene and mandatory isolated migration/restored-pair/rollback CI. No application/native/schema/UI/root-dependency/default-Compose/existing-acceptance changes. Read docs/CONTINUITY.md and docs/DEPLOYMENT.md for current evidence, operations and limits.

At exact implementation head ec1e47f7fb01656eb0f65f1dbee374c7cb861f82, preparation CI 38968347629 passed nested IAM, 18 candidate contract scenarios (124.87s) and all six migration phases (240.95s), with zero failures/skips in dedicated runtime jobs. Four-file source, coherent checkpoint, exact-byte/scoped logical copy, pre-write rollback, actual unchanged app against restored PG + cold-restored RustFS, real Chromium/new upload and six-file coherent post-write/delete rollback passed. Endpoint-only reversal failed reconciliation as expected (three missing additions, stale deleted object); baseline checkpoint/object bytes remained unchanged. Existing CI 38968347529 passed Go/native/race/security/lint/docs and 15 default-MinIO scenarios; its separate new opt-in rehearsal skip is not migration evidence. Local Docker remains unavailable.

Helper tests explicitly report no Go test files; vet/module verification and three guard rejection cases passed, with real IAM provisioning/denials in the fixture. Source scan retains one unimported module-only OpenPGP advisory GO-2026-5932/no fix, zero imported-package/symbol/initialization traces. Runtime OS/SBOM inventories do not certify vulnerability absence or Rust binary-symbol reachability.

Original DEVLOG preservation anchor: blob 4f976745c6ab14e8ad79963ba81eb5075e35444c, exact 14,783-byte prefix retained byte-for-byte; appended only after real implementation commits and verification. Original/WSL checkouts, .qodo, remotes and protected refs preserved. No bypass reproduction, real-data operation, deployment or integration. Sanitized evidence retained 14 days; raw checkpoints/secrets/volumes remain ephemeral runner-local.

Producer preparation acceptance is complete; final draft-PR SHA/CI/readback handover and independent Aether acknowledgement remain separate. Production S3 TLS, capacity/maximum size, topology/maturity, security-update ownership/rotation, historical data/schema/owners, combined cross-host DR, production cutover/migration, public deployment, protected integration and final vendor adoption remain open. No production readiness claim.


### 2026-10-16 — Complete runtime identity evidence

After successful bounded acceptance, evidence-only implementation 4167a233aaaf7e8ead64564a28b71005672a2c4f records exact built source-MinIO/backend/RustFS image IDs in the sanitized rehearsal artifact. It changes only the authorised test path and does not change runtime behavior or acceptance assertions. Fixture compilation and zero-issue lint passed before the implementation commit; exact final-head hosted reruns/readback are in draft PR #24's GHP-01 handover.

This entry is appended after that real implementation hash. All preceding 17,867 bytes and the original 14,783-byte preservation anchor remain byte-identical. Production/reconciliation/transport/capacity/recovery/adoption gates and independent Aether review remain open.


## 2026-10-17 — Delegated completion: exact integrated acceptance and core closeout

Owner Konstantinos Aidinidis delegated SFD-OWNER-DELEGATED-COMPLETION-20261017 to Codex, retaining Aether independent security-sensitive acceptance. Existing M24 integration 8d54031c26ec10027ecebadd754f34996d55e8dd is preserved; no repeat merge or history rewrite. Separate branch codex/sfd-integrated-acceptance-20261017 / PR #25 adds exact-M24 workflow installation 871ca4d2d2533ed0231ed36771c3ebab3aa532b6, with immutable source/tree checks and unchanged suites.

Hosted run https://github.com/dreamingfree09/secure-file-drop/actions/runs/38982360717 actually passed contract 18/18 (134.04s), six-phase rehearsal 6/6 (245.85s), zero failures/skips, nested IAM vet/mod verify and three safe guard rejections. Downloaded artifacts confirm M24 and empty before/after labelled resource inventories. Four baseline/six rollback manifests, checkpoint and runtime image identities are retained for 14 days. Checkpoint SHA-256 36f37ee6df19859357e6c522136eb6b95f2b3ed14d64761b926f3aadd35a5439. No raw dump/secrets/volume publication or real-data operation.

M24 default-MinIO 15/15 and root/browser/native/security/lint/docs evidence, plus passing workflow-proposal CI, support bounded core development closure. Existing MinIO 75 binary advisory IDs and backend/nested module-only finding remain; no production storage clearance. Continuity/tracker record full evidence and retained production TLS, capacity, operational ownership/rotation, schema/ownership reconciliation, cross-host DR, migration and deployment gates. Aether acknowledgement/integration of the new proposal is pending.

Next authorised stage: separate lightweight native Windows companion, existing authenticated API, streaming progress, TLS validation and protected session storage. Compose 10 MiB default and five-minute upload/two-minute hash bounds remain; no arbitrary multi-GiB or resumable claim. Preserved checkouts, branches, local work, .qodo and unrelated environments remain untouched. This entry appends to all received DEVLOG bytes.


## 2026-10-17 — Delegated native Windows companion implementation

SFD-OWNER-DELEGATED-COMPLETION-20261017 phase three follows verified integrated M24 RustFS 18/18, migration 6/6 and bounded core closeout. Codex created separate stacked codex/sfd-windows-companion-20261017 from core proposal 6f7c57dada6bac670baa0d676c7ac025f69a3f98. Existing master/branches/checkouts/data remain preserved; independent Aether acceptance and integration are pending.

Native .NET Framework Windows Forms implements tray Quick Send, current cookie authentication, bounded streaming/progress, local/server SHA-256 equality, current expiring HTTPS links, health status, protected remembered sessions, safe per-user Explorer command/IPC, cancellation/uncertain-outcome reconciliation and companion-only exit. No backend protocol, schema, vendor, root dependency or service-control change. The portable real-client harness is opt-in inside the actual isolated TLS/backend fixture; normal suite assertions are retained. Workflow installation remains separate.

Actual local execution: 26 Windows platform/security/lifecycle checks and 10 HTTP protocol checks passed, including Unicode argument/MIME handling, DPAPI/tamper/origin isolation, owned registry and IPC, in-flight cancellation/reconciliation and redirect credential isolation. Go tests/vet/race and five native vectors passed. First SDK use generated one untrusted development certificate; exact metadata/time were verified and only that newly created certificate/private key was removed; no trusted root or existing certificate changed. Subsequent builds disable SDK certificate generation.

Hosted real HTTPS/backend client execution and final-head readback are pending. Visual tray, picker, Explorer, clipboard, real-user-profile startup and complete Windows Framework HTTPS deployment journeys remain unexecuted; no false GUI acceptance. Documentation records existing 10 MiB Compose/five-minute upload bounds and no arbitrary multi-GiB or resume promise. Production storage/security/TLS/capacity/ownership/reconciliation/cross-host DR/migration/deployment gates remain open. This entry is strictly appended to all received DEVLOG bytes.


## 2026-10-17 — Companion validation refinement

Initial hosted Windows build/26 platform plus 10 protocol checks passed at ffef0bafa7d64f617fc5cd4058590b8127b77826. Initial root CI reported one unused Go helper parameter; the actual desktop/backend job reported an incorrect relative harness DLL path because Go executes tests from tests/e2e. Corrected the parameter and resolve the repository root explicitly for the child process and source identity. Failed runs remain visible; no assertion or lint rule was disabled.

Returned-link validation now also rejects control/whitespace, extra query parameters and malformed bearer-token grammar, with six additional focused regressions. Actual local Windows result is 32 platform/security/lifecycle checks plus 10 protocol checks; GUI/manual gates remain distinct. Final hosted exact-revision results belong in PR #26. All received DEVLOG bytes remain unchanged.


## 2026-10-17 — Companion asynchronous view isolation

Source review corrected stale progress/readiness callbacks across transfers/backend changes by binding results to the initiating operation/client. Configuration changes now retain an actionable warning when complete sign-out from the previous service cannot be confirmed. Local Windows 32 platform/security/lifecycle and 10 protocol checks still pass; these do not substitute for the explicitly unexecuted visual settings/progress journeys. The final published revision and hosted evidence remain authoritative in PR #26. No backend, storage or deployment change.


## 2026-10-17 — Companion independent automated-review disposition

Confirmed the configuration timeout issue: sign-out now returns an explicit incomplete result on an old-backend timeout, permits saving the replacement address, retains the warning and still propagates explicit cancellation. A remembered but unverified session is also signed out/cleared. Actual Windows protocol regressions now total 14, alongside 32 platform/security/lifecycle checks; the native test executable reports CLR 4.0.30319.42000. Visible GUI/full Windows HTTPS deployment acceptance remains unexecuted.

Confirmed the concurrency placeholder issue; its expression sigil is corrected in a separate workflow-only commit. The reported workflow/application commit mixing is not reproducible: initial application 1126fc923e2618f90c71e85fcdbbcf1f138287fe excludes workflows; installation ffef0bafa7d64f617fc5cd4058590b8127b77826 changes only the workflow; later count corrections are separate. Preserve normal history; do not rewrite it to satisfy that false positive. Exact CI/proposal and review dispositions are recorded in PR #26; Aether acceptance remains pending.


## 2026-10-17 — Shipping HTTPS boundary

Aligned the companion with the Owner's no-TLS-bypass requirement: removed the user-facing HTTP option. The shipping client is HTTPS-only. Plain HTTP is available solely under SFD_SYNTHETIC_TESTS for owned protocol/readiness fixtures, absent from release compilation and non-serializable. Three independently compiled release-policy checks prove valid HTTPS, rejection of configuration-injected HTTP flags, and absence of the synthetic capability. Native Windows 32 platform/security/lifecycle plus 14 protocol checks and all three policy checks passed; release build passed with warnings as errors. Existing backend HTTP development behavior is unchanged. This is a stricter client boundary, not a production storage/deployment approval.


## 2026-10-17 — Final desktop review and integration preparation

Owner-approved SFD-DESKTOP-FINAL-REVIEW-20261017. Codex reviewed all four core proposal paths at aa784cab5b32860a72958e35b0b89085ff65de72 and all 22 Windows proposal paths at received 8f55c9deb24b477ecfc516bb67ee8078bb9ad0f8, with M24 master unchanged. No new confirmed product defect; source and real test assertions reviewed rather than test labels alone. Fresh artifact/log readback verifies exact M24 RustFS 18/18, rehearsal 6/6, explicit nested IAM guards/module-only limitation, and Windows 32 + 14 + 3 versus portable real backend 15 + 17 with no mandatory failures/skips and empty owned inventories. Product executable/config/licence/provenance verified, including normal Git-to-Windows CRLF conversion; unsigned preview remains a test candidate at its actual received build revision.

Prepared a precise fifteen-step actual-Windows operator checklist. Every interactive step remains unexecuted; no profile installation/Explorer registration, certificate/security/Docker/WSL alteration, service control or new bypass experiment. Conflict-free merge trees and normal-merge availability were checked without integration. Aether exact-head acknowledgement/GHP-01 gate remain outstanding; master and source branches preserved. GitHub protection lacks required status/review rules, so those gates need explicit verification. Final-review publication/CI evidence belongs in PR #26's exact-final-revision handover. All original checkout/local work/.qodo/DnD, previous branches and prior DEVLOG bytes remain preserved. Production storage/maintenance/TLS/capacity/reconciliation/cross-host recovery/migration/deployment gates remain open.


## 2026-10-18 — Standing delegation and local TLS continuation

SFD-OWNER-AUTONOMY-20261018 delegates bounded engineering and governed dependency-ordered integration to Codex, with exact-revision independent Aether acceptance and mandatory gates retained. Recorded standing authority in AGENTS/GHP-01, current TLS/secure-signing/maintenance/rollback in the local runbook, and the native acceptance prerequisite in continuity/companion documentation. This entry follows every byte of the received journal; no historical content changed.

Received master 8d54031c26ec10027ecebadd754f34996d55e8dd, PR #25 aa784cab5b32860a72958e35b0b89085ff65de72 and PR #26 590d9f2c5f924f19c70bff460a319733ef5a5017 match GitHub. Both received proposal heads have six successful root-CI checks; no Aether exact-revision acknowledgement is recorded. Signing script hash reverified, but no signed output exists. Strict Schannel/Framework validation still fails leaf revocation; activation/persistence/interactive Windows acceptance remain unexecuted. The Owner's sole required immediate action is local hidden-prompt signing, not new routine approval. Existing runtime, trust, original checkouts, branches, local work, data and backups remain preserved. Production storage/TLS/capacity/rotation/topology/reconciliation/recovery/public-deployment gates stay open. Exact final documentation SHA, fresh CI and independent handover are recorded in PR #26 after publication.


## 2026-10-18 — Local TLS automated acceptance and Windows handoff

The Owner completed protected signing; Codex verified and activated leaf 4106ecd1f0bd2d94be7a7fcf2236cb9b0119dde5aa82a97edd2cdab584809b70 and issuer CRL 0x1000 through the existing CA/key, without new roots or weakened validation. Windows retrieved the exact signed CRL on loopback, normal Schannel health/ready and revocation-enabled Framework requests returned 200, and strict Windows chain validation had no errors. An authorised proxy-only recreation retained the leaf and repeated all Windows results; all five non-proxy identities/images/start times/mounts and root inventories stayed unchanged. Offline missing/expired/corrupt-CRL tests rejected; actual leaf revocation and Windows expired/revoked runtime tests were not executed. Maintenance requires CRL refresh before 2026-10-25T12:17:40Z; no automatic renewal/scheduler.

Updated runbook/continuity/companion prerequisite only, preserving every received DEVLOG byte. No app/workflow/dependency changes, backend/data operations, existing-process termination, application launch, Explorer/credential/settings/clipboard access, merge or production deployment. Existing historical backend provenance/full authenticated compatibility and actual native Owner journey remain open alongside Aether acceptance and production gates. Final exact documentation SHA, fresh CI, public certificate/CRL identities, service preservation and rollback evidence are recorded in PR #26 after publication.


## 2026-10-18 — SFD-WINDOWS-TRAY-CONNECTION-20261018

Owner-authorised Windows-only correction from PR #26 received 4c3c2736129a1b14238c6e0f2e3b50910a1b5311. Reproduced historical preview legacy TLS defaults from missing compiled TargetFrameworkAttribute; explicit Framework 4.8 target fixes actual credential-free HTTPS requests without trust/server changes. Added explicit revocation checks, safe transport classification and fail-closed session-verification diagnostics. Local 32 platform +16 protocol +6 release-policy and five native credential-free live HTTPS checks passed. Historical WSL account/API differences remain visible; Owner-operated authenticated tray acceptance and Aether review are separate gates. Final exact publication, CI and replacement binary identities are recorded in PR #26. Original executable/process/settings, backend/data/TLS/credentials, branches and unrelated projects remain preserved; no merge or public-production readiness claim.


## 2026-10-20 - SFD-FULL-E2E-ACCEPTANCE-20261020: acceptance execution

Received canonical combined source c320861536914623ea22f28c1d4ea156d4c2132a (PR26 over PR25), separately preserved legacy proposal 8c548d8da15e280f3a5b595eaa8a0687627f5452 (PR27 over WSL feature2b). No source-lineage substitution, live backend adoption or integration is authorised while Aether exact-revision gates remain unresolved. Baseline Go/unit/race/vet/build/native/lint/docs/environment checks passed; explicit opt-in/fault fixture skips are not runtime acceptance. Expanded the existing real browser journey with privately piped synthetic credentials, keyboard/role/file-boundary/integrity/expiry/interruption checks and isolated clipboard instrumentation. Fixed stale upload error text at the start of a subsequent attempt and marked existing feedback as alerts. Actual runtime results and remaining visual/legacy reconciliation gates will be recorded after execution. Original DEVLOG prefix and all existing worktrees/services remain preserved.


### 2026-10-20 — Rehearsal credential handoff

The expanded browser uses fresh synthetic passwords; rollback phase F still assumed its historical constant and correctly rejected authentication. Pass a fresh rehearsal-owned browser password privately through stdin and reuse it only to verify the restored synthetic account. Retain both failed rehearsal namespaces stopped with checkpoints. The five completed phases are not six-phase acceptance; rerun all six in a fresh namespace.


### 2026-10-20 — Current-head mandatory acceptance CI

Install the full E2E workflow separately from application corrections. Test the actual checkout revision, retain strict 15 default / 18 RustFS / 6 rehearsal counts and 37 real browser checks, require 17 portable real-backend client checks and native Windows 32/16/6, and independently verify fixture cleanup. Public runtime evidence contains structured assertions only; private raw logs/checkpoints cannot be uploaded by this workflow. Historical pinned integrated-master workflows remain unchanged.


### 2026-10-20 — Bound fresh browser fixture input

The fresh rehearsal browser password exceeded the registration UI maximum. Bound it to a random 41-character synthetic value. Browser registration timeout is retained as a failed fixture attempt; no six-phase pass is claimed.


### 2026-10-20 — Full E2E current state and retained gates

Record the exact received PR ancestry, scoped UI/API/audit corrections, real browser/Windows evidence, preserved reviewed executable, failed rehearsal evidence and pending independent acceptance. No live adoption or integration occurred. Retain the rejected fallback decision and explicit storage/deployment, schema/ownership, capacity and cross-host recovery gates. Canonical PR handover is the authoritative revision-specific report.


### 2026-10-20 — Dependency scan failure attribution

Hosted contract scans reached the verified RustFS binary but failed inside a Docker helper closing over the parent test, causing Goexit and hiding the actual scanner failure in the child summary. Bind the dependency scanner helper to its current subtest while keeping ownership cleanup on the parent. Do not suppress the failed scan or count later scenarios as passes. Local18/18 and hosted6/6 evidence remain separate.


### 2026-10-20 — Public SBOM scanner permission correction

Exact hosted failure: private wrapper umask0077 reduced the public upstream SBOM creation mode to0600; the restricted scanner could not read its bind mount. Explicitly apply intended0644 only to the checksum-verified public SBOM in t.TempDir. Keep private log/fixture umask0077, scanner restrictions, provenance digest and all advisory assertions. A narrow G302 annotation documents this public-only boundary; legacy lint annotations/policy remain unchanged.


## 2026-10-20 — SFD-FINAL-RECONCILIATION-20261020

Refreshed exact PR25-29 refs and read Aether6301566150. Added a read-only canonical migration-lineage guard after reproducing two version4 false-compatibility cases on fresh isolated PostgreSQL; eight regression subtests pass and rejected fixtures preserve metadata/rows. CI installation is a separate commit. Re-inspected exact successor/recovery binaries and reran42 isolated checks per BYTEA/BIGINT layout. Recorded legacy deletion wrong-key/storage-failure and lifecycle preservation blockers, exact lineage/contract differences, selected-port proposal and normal integration plan in docs/LINEAGE_RECONCILIATION.md. Independent security/architecture acceptance, unified baseline, live adoption and Owner visual checks remain open. All six live containers, original repositories/local work, environment/TLS/CRL and retained failure resources preserved; no protected integration or new product feature.


### Final reconciliation follow-up — active schema and retained-image failures

A valid PR30 review finding reproduced three custom-schema false acceptance/rejection cases in the initial guard. Corrected it on the same borrowed connection as the migration driver, with cross-schema fallback rejection; eleven isolated PostgreSQL/race cases now pass and snapshots retain rows/metadata. Retained and executed a reproducible defensive deletion regression on each exact legacy image separately: both FAIL because administrator DELETE204 forgets metadata but leaves the actual object, and a distinct owned DELETE during a fixture-only storage outage also returns204/forgets metadata. Initial reused-row fixture mistake is preserved separately. Both final failure fixtures cleaned only owned resources and left live services unchanged. Both images are unsuitable for operational adoption until complete lifecycle correction/independent review. Historical positive42-check recovery results remain evidence with this explicit limit. No new feature, live mutation, schema downgrade or protected integration.


## 2026-10-21 — reconciliation continuation and authority clarification

PR30 guard b34 final CI/artifacts all pass; exact evidence is recorded in LINEAGE_RECONCILIATION. The existing standing delegation assigns bounded port design/implementation to Codex; prior direction-approval wording was an overly restrictive interpretation and is superseded. PR31 implements the schema12 lifecycle/Windows selected-port candidate with real owned migration/COMMIT/concurrency/cleanup coverage. Exact independent Aether source/security/recovery acceptance still gates integration and live adoption. This commit changes documentation only and preserves historical records; no live/protected changes.


## 2026-10-22 - SFD-FINAL-ACCEPTANCE-CONTINUATION-20261021

Codex prepared an explicit selected-source canonical worker from PR30 with PR31 runtime/schema/companion allowlist, preserving every original branch and canonical evidence/test file. Reconciled missing canonical JSON/cancellation/password/filename/browser safeguards; retained fixed uncapped lint policy and meaningful real-session tests. Source manifest and docs/CANONICAL_RECONCILIATION.md describe exact selection and limitations. No protected merge/live adoption/real migration; CRL renewal prepared against established ledger for secure OpenSSL Owner input. Final publication/tests/CI and signing state are recorded separately after execution; this entry does not claim pending checks passed.


## 2026-10-22 - Fresh runtime advisory correction

Current authoritative scans exposed newly published GO-2026-6809 (reachable Go x509 Verify, fixed1.26.10) and Alpine musl/musl-utils1.2.6-r2 below patchedr3. Codex raised actual build/test toolchain and runtime constraints; prior images retained as evidence and superseded for adoption. Rebuild and full exact-image acceptance are required; no production change or exploit reproduction.


## 2026-10-22 - PR32 source and deployment review corrections

Codex reconciled confirmed startup/download-secret, untrusted forwarded-header, multipart proxy, missing monitoring configuration and registration-delivery defects. Disabled SMTP now fails before user mutations; failed delivery rolls back; synthetic loopback SMTP exercises real verification. Unaccepted reset email fails closed501 instead of publishing a dead UI link. Ordinary fail-before unit checks and real isolated Traefik/mailer checks retain existing security assertions. Application and workflow changes remain separate normal commits. No live service/data/credential/TLS changes or new product feature. Exact final tests and revision remain in the producer handover.


## 2026-10-22 - Final canonical source and bounded evidence

Codex completed the actual normal canonical selected port and confirmed review corrections at791f1adc0bed00722e3d24be73e6b07e2c32aa4e. Local92 checks/layout and203 real PostgreSQL events/no skips, real SMTP/verification and normal-TLS Traefik200/413 pass. Actual F19644/43 and PostgreSQL16.15 private custom restore roundtrips pass. Own-head push39696157037/PR39696161254 all-success artefacts were downloaded and read back:94+27 browser/layout,203 Go,18 config and Windows32/16/6. Source/image/blob/lineage/preservation evidence is committed as structured JSON; raw logs/credentials/dumps remain private. Earlier failures and root skips remain explicit. No protected merge/live adoption or Owner GUI claim. Secure encrypted-CA CRL signing is pending Owner hidden input only; all independent work completed, final evidence-head CI and Aether exact acceptance remain distinct.


## 2026-10-22 - Owner-signed CRL publication and preservation

Owner executed the checksum-verified hidden-input helper once. Codex verified the established ledger/counter, signature/AKI/issuer/number/validity/active leaf and atomically published only CRL0x1001/hash69bbe78994b7e6abe9849e65fd8a56fe59dbe0803e6db498f3e8c82ad70f6f1e. Renew by2026-10-28T18:10:29Z; expiry2026-10-29T18:10:29Z. Normal Schannel and explicitly revocation-enabled.NET health/ready200, exact Windows DER retrieval and full before/after live preservation pass. No service restart, real-data operation, root/key/cert/config replacement or insecure fallback. Initial local wrapper syntax error failed before execution; corrected parsed wrapper published once. Prior output and old public DER are retained. Sanitised evidence and current TLS/continuity entries are added; README toolchain spelling now matches1.26.10. Application/native/desktop/build/workflow bytes unchanged. Exact final evidence-head CI and Aether review remain separate gates; no protected merge/live adoption/Owner GUI/public readiness claim.
