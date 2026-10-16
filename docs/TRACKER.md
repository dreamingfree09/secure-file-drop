# Focused corrections — 2026-10-16

Work ID: SFD-DEEP-CORRECTIONS-20261016-CODEX. Owner-approved implementation following [Aether's assessment review](https://github.com/dreamingfree09/secure-file-drop/pull/22#issuecomment-6204810242). Base master: `0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7`; received assessment: `1f30bb374420d3285ebdaac0c192240897c5d0f4`. [PR #22](https://github.com/dreamingfree09/secure-file-drop/pull/22) carries the exact final-revision handover; the [canonical correction appendix](DEEP_REVIEW_20261015.md#approved-correction-status--2026-10-16) supplies evidence and limits.

| Finding | Current disposition |
| --- | --- |
| F1 | Awaiting storage decision; confirmed affected source, runtime bypass unexecuted |
| F2 | Fixed: complete single-file multipart framing, error latch, bounded cleanup; empty/actual-size contract retained |
| F3 | Fixed: entire 16 KiB JSON envelope and one-value validation on all four endpoints |
| F4 | Fixed: request cancellation and bounded affected SQL, verified with real PostgreSQL fixtures |
| F5 | Fixed: migration-owned resources released, shared pool preserved; repeated/small-pool/error tests |
| F6 | Fixed: obsolete root executable removed normally, ignored future output, documented source build |
| F7 | Application/runtime remediation verified; archived storage advisories remain unresolved |
| F8 | Fixed within tested scope: actual proxy limits/TLS/canonical origin/port and explicit network |
| F9 | Fixed: clear credential fields, abort owned requests, reject stale callbacks; seven script tests and real browser |
| F10 | Fixed: accumulating/redacted validator and native stdout failure handling, synthetic/vector/sanitizer regressions |

Verified code/test/CI revision `709942ee7a490a9d36c647760a21c4c74bd2c51c`: both [push](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38830402197) and [PR](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38830407424) CI passed all jobs; all 15 real-stack scenarios ran, including real Chromium, proxy, native byte equality and PostgreSQL fault/pool/cancellation checks. Local Go/race/vet/lint/native/sanitizer/script checks passed. Actual application binary: zero symbol/package findings and one unused-module finding; actual storage: 75 advisory IDs (51 symbol, 4 package-only, 20 module-only). Both rebuilt Alpine inventories had zero queried secfix matches. Bounded COMMIT uncertainty, native readiness and active-transfer shutdown checks are documented in the canonical appendix. Local Docker remains unavailable; CI supplies the real disposable stack/browser evidence. No bypass test, existing-data operation, master integration or deployment.

Outstanding decisions/gates: maintained free/open-source storage (recommend a separate SeaweedFS Community evaluation, RustFS alternative), candidate qualification/migration/rollback, existing-schema/ownership reconciliation, maximum-size/slow-client capacity, public TLS/ingress and whole-service recovery. Temporary storage isolation does not close F1/F7. Independent acknowledgement and later integration remain pending. Earlier records below are dated history.

---

# Deep assessment — 2026-10-15

Work ID: SFD-DEEP-REVIEW-20261015-CODEX. Affected accepted revision: `0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7`.

See the canonical [deep assessment report](DEEP_REVIEW_20261015.md) for findings and the operating matrix; [PR #22](https://github.com/dreamingfree09/secure-file-drop/pull/22) is the exact-revision handover. Review tests/evidence only: production code and dependencies are unchanged. Truncated multipart and JSON-envelope regressions fail on the accepted implementation. Storage/advisory applicability, SQL cancellation, migration connections, stale artifacts and browser/diagnostic limitations are recorded without implying runtime exploitation.

Assessment is complete pending Aether/Owner review of the proposed correction package. Implementation and master integration are not authorised. The following quality-closeout and milestone records are preserved as dated history; current acceptance is bounded by the new report.

---

# Quality closeout — 2026-10-15

Work ID: SFD-QUALITY-CLOSEOUT-20261015-CODEX. Base: `1a0f7715faa89c53fcec266f59e43cad9b1a057b`. Producer: Codex; independent review: Aether in “GitHub workflow implementation”. See [continuity](CONTINUITY.md) for checkout preservation and decisions; the owned PR is the complete final-revision handover.

## Confirmed findings and fixes

| Area | Baseline finding | Correction / evidence |
| --- | --- | --- |
| Roles and ownership | Any session could call admin routes; file ownership was ignored and metadata attributed to admin | Signed administrator marker, route guards, active-user checks, principal-owned metadata and owner-filtered list/upload/link queries |
| Sessions/logout | No server logout/revocation; empty secret/subject accepted | Persist only token hashes, random signed session IDs, server revocation and cookie expiry; migration 4; stronger startup secrets |
| Links | Expiry second still accepted; forwarded headers trusted | Expired at now >= exp; canonical configured origin or direct request fallback |
| Bounds/filenames | Negative/zero/unset bounds could disable limits; declared size used for downloads; unsafe attachment interpolation | Finite default, reject invalid bounds, independently bound payload/envelope, actual stored/hash counts, basename/control validation and MIME attachment formatting |
| Lifecycle/storage | Concurrent uploads could both act on pending; admin deletion used UUID instead of object_key; storage failure could orphan bytes | Transaction row locks; cleanup skips locked rows; exact object keys; object deletion before metadata; retain failed rows on errors |
| Schema/migrations | Admin listing queried absent updated_at; standalone runner omitted versioned users schema | Query actual columns; shared embedded migration command; immutable old migrations |
| Native/container | Legacy e2e could silently fall back to Go; runtime omitted libcrypto3 | Native-only acceptance, five asserted SHA-256 vectors, matching Alpine build/runtime and explicit shared library |
| Browser | Admin UI shown to ordinary users; metric field mismatch; no logout; static path broke local source run; false E2E encryption claim | Role-aware actions/list, real logout, correct counter names, configurable repo-relative assets, corrected privacy wording |
| Tooling/dependencies | CI Go 1.20 vs module 1.24; masked failures and unsafe e2e resources | Go 1.26.9, maintained lint configuration, no failure masking; unique labelled disposable resources and mandatory opt-in e2e CI |
| Dependency scanner | Reachable pgx and x/text advisory paths | pgx v5.9.2 and x/text v0.41.0; post-fix scan reports zero reachable vulnerabilities |

## Actual producer validation

- Baseline safe unit packages, go vet and backend/native builds passed on Go 1.24.11. Legacy Docker e2e was not executed because it used a fixed port, incomplete cleanup and Go hash fallback.
- A temporary export of the exact base reproduced failures in missing-secret/empty-subject sessions, exact download-expiry boundary, upload configuration bounds and untrusted-forwarded-origin handling. New regression tests pass against the changed source.
- Patched Go 1.26.9: go test ./... -v -count=1 passed the unit packages. The Docker package explicitly **skipped** without SFD_E2E=1; this is not full-stack acceptance.
- go vet ./..., go test -race ./internal/... ./cmd/... -count=1, backend compilation and make -C native test passed. Native checks assert empty, abc, multiblock, million-a and binary vectors plus CLI/I/O failures.
- go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...: zero reachable vulnerabilities after fixes; 2 imported-package and 30 module-only advisories remain outside detected calls. This does not certify all dependencies or container images.
- Baseline reachable advisories: [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629), [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970), [GO-2026-5004](https://pkg.go.dev/vuln/GO-2026-5004). Traces enter via pgx connection normalization / SQL sanitization. The HTTP SQL here is parameterized and does not use dollar-quoted literals; no remotely exploitable injection was demonstrated. Dependencies were patched rather than assuming those paths safe.
- Browser observations: actual static login and registration screens rendered; empty registration and 73-byte passwords rejected; corrected privacy wording verified. The temporary loopback server was verified by listener/process identity, then stopped. Successful login/upload/download through the browser is **blocked**, not inferred from static rendering.
- Local opt-in full-stack execution was attempted and **failed before resource creation** because the local Docker Desktop engine pipe is absent. Starting the installed backend failed initializing its inference socket and then timed out. Host repair/settings changes are outside scope. No assignment database, bucket, volume or container was created locally.
- golangci-lint v2.14.0: zero issues. Local Markdown target verification and Node syntax verification of the actual inline UI script passed; external URLs were not checked. Final GitHub container/full-stack results will be recorded in the PR before handover.

## Remaining decisions and acceptance limits

Existing ownerless files stay administrator-only; historical ownership cannot be safely inferred. Unknown manually migrated users schemas require a reviewed reconciliation plan. No existing database or storage was inspected or migrated. Existing private files, local notes and alternate feature work are preserved as recorded in continuity.

No public deployment, large-file throughput claim, production TLS/proxy certification, backup restore, native library/OS advisory audit or roadmap feature delivery is claimed. The hashing subprocess uses temporary disk, a 2-minute timeout and a 5-minute upload deadline; representative maximum-size load validation remains required.

The draft [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21) is published; its handover carries the final full SHA, complete scope, latest CI outcome and producer completion state. Next action after successful publication readback is Aether's independent review at that exact revision. Master integration remains unauthorised.

## Historical milestone tracker received from the baseline

The following dated baseline snapshot is preserved. Its Complete statuses are historical claims, not fresh acceptance results; current evidence above and the PR take precedence.

---

# Secure File Drop – Implementation Tracker (MVP)

This tracker is the source of truth for implementation progress. A task is only considered complete when its acceptance criteria are met and a commit exists that demonstrates it.

## Milestone 0 – Repository & Documentation Baseline

Status: Complete

Acceptance criteria:
- Repository initialised with Git
- Project folders exist: docs/, journal/, cmd/, internal/, web/
- README.md exists with project overview
- docs/SPEC.md exists with MVP scope
- docs/TRACKER.md exists and is up to date

## Milestone 1 – Local Dev Environment (Docker Compose)

Status: Complete

Acceptance criteria:
- docker-compose.yml exists
- Services start successfully:
  - PostgreSQL
  - MinIO
  - Backend API (placeholder)
- A single command starts everything locally
- Health checks show containers are running
-PostgreSQL and MinIO are running via Docker Compose and that the MinIO bucket sfd-private exists.

## Milestone 2 – Backend Skeleton (Go)

Status: Complete

Acceptance criteria:
- Go module initialised
- Backend builds successfully
- Basic HTTP server responds at /health
- Structured logging enabled (request id per request)

## Milestone 3 – Authentication (Single Admin)

Status: Complete

Acceptance criteria:
- Admin user is created via environment variables or a one-time bootstrap command
- Login endpoint issues a secure session (JWT or signed cookie)
- Auth middleware protects upload endpoints
- Unauthenticated requests are rejected correctly

## Milestone 4 – Upload Pipeline (Backend + MinIO)

Status: Complete

Acceptance criteria:
- Authenticated upload endpoint accepts a file
- Upload size limit enforced (backend)
- File stored in MinIO (private bucket)
- Metadata stored in PostgreSQL
- Upload returns file id and basic metadata
- Explicit lifecycle enforcement (pending → stored → failed)
- Fail-fast configuration validation
- Streaming-only handling (no full buffering)
- DEVLOG entry exists documenting a real production failure and resolution

## Milestone 5 – Integrity Utility (C) + Integration

Status: Complete

Acceptance criteria:
- C utility compiles on your machine
- Utility computes SHA-256 for an input file
- Backend calls the utility during upload
- SHA-256 is stored in PostgreSQL for each file
- Upload fails cleanly if hashing fails

## Milestone 6 – Signed Download Links + Expiry

Status: Completed

Acceptance criteria:
- Backend generates a signed token for a file id with expiry time
- Download endpoint validates token and expiry
- Download streams file from MinIO
- Invalid or expired tokens are rejected correctly

## Milestone 7 – Minimal Web UI (Upload + Link Display)

Status: Complete

Acceptance criteria:
- User can log in
- User can upload a file
- User receives a download link
- UI displays recent uploads

## Milestone 8 – Public Exposure Hardening (Day-One Safe)

Status: Complete

Note: Canonical public base URL handling for signed links implemented and committed as fce217b; remaining proxy hardening tasks still pending

Acceptance criteria:
- Reverse proxy configured for HTTPS-only
- Rate limiting configured
- Max body size configured at proxy
- Basic security headers enabled
- Cloudflare fronting confirmed (DNS + proxy)

## ## Milestone 9 – Public Exposure Hardening (Reverse Proxy Layer)

Status: Complete

Acceptance criteria:
- Reverse proxy deployed in front of the backend
- HTTPS termination confirmed end-to-end
- Backend API is not directly exposed to the internet
- Maximum request body size enforced at proxy level
- Rate limiting enforced at proxy level
- Basic security headers applied (e.g. HSTS, X-Content-Type-Options)
- Public access validated through the proxy only
- Behaviour verified via real requests
- DEVLOG entry exists documenting configuration, validation, and any issues encountered


### CI storage dependency follow-up

[First CI at 91ca11c](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38797545857) passed Go/native/race/vulnerability/lint/docs, but full-stack setup failed to pull the historical MinIO image. Its cleanup correctly refused an invalid resource identity caused by mixed stdout/stderr; the harness now captures IDs from stdout only. That run's disposable hosted runner was discarded; no local resources were involved. [Official community sources](https://github.com/minio/minio) are source-only and archived. The pinned source image and pending production maintenance decision are recorded in continuity. At that checkpoint full-stack acceptance was still pending the corrected CI run; a failed setup is not an application pass. The [PR handover](https://github.com/dreamingfree09/secure-file-drop/pull/21) carries the final revision and latest acceptance result.

### Full-stack restart and UI follow-up

The corrected pinned-source run built the real storage/runtime images and passed the API/security flow through the intentional storage outage, then failed waiting on the pre-restart ephemeral host port. The harness now re-reads and verifies the owned container's loopback binding after restart instead of assuming an ephemeral port remains fixed. The failed run is retained as evidence, not represented as acceptance.

Logout also clears previous file/link/input/metric UI state; generation checks prevent delayed list/upload responses from rendering into a later account. Dedicated Node session/escaping unit tests cover these contracts. They are distinct from actual browser and full-stack observations.

Final lint configuration also checks test files without an errcheck exclusion. Environment/multipart setup errors are checked; transfer/read errors remain assertions, with only routine teardown closes explicitly ignored.

### Verified real-container acceptance

[CI run 38799681132](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38799681132) at `8668a727c2dbf7bcc7980c125934b371b980bd0e` completed all three jobs successfully: Go/native/UI unit/vet/build/race/vulnerability/lint checks, documentation targets, and the mandatory real-container upload/native-hash/signed-download flow. Full-stack negative cases include ownership/roles, tampered and expired links/sessions, concurrency, native-tool failure, private storage, outage/retry, cleanup and attachment headers. Resource cleanup returned without ownership errors. Later documentation/test error-checking changes do not alter production behavior; the PR handover records the final revision's CI readback.
