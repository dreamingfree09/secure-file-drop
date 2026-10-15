# Secure File Drop deep defensive assessment

Work ID: SFD-DEEP-REVIEW-20261015-CODEX. Producer: Codex. Independent reviewer: Aether in “GitHub workflow implementation”.

Affected application revision for every finding below: `0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7`. Remote master matched that accepted revision at receipt. Review branch: `codex/sfd-deep-review-20261015`; [PR #22](https://github.com/dreamingfree09/secure-file-drop/pull/22) is the exact final-revision handover. Production source, module versions and accepted master are unchanged. Test/CI invocation changes and this report are proposals for review.

## Verdict and evidence limits

The rebuilt application supports bounded synthetic API workflows in disposable CI. It is not approved for public/server deployment or real-data adoption. Truncated uploads can be acknowledged as successful; JSON envelope validation is incomplete; storage has known affected upstream code and additional advisory matches. Trusted isolated development with synthetic data remains useful, subject to the failed scenarios and storage restrictions below. The tracked standalone backend is stale and must not be used as the rebuilt application.

Local Go/native/browser-script checks are distinct from container API checks. No authenticated real-browser upload journey ran. Starting the installed local Docker Desktop was attempted without repair/settings changes; the local engine was initially absent, then access-denied/stalled after normal startup, and its assignment-owned version probe was stopped without repairing the host. No live database, existing volume, real backup, user credentials or alternate checkout was used.

Two genuine regression failures are retained: malformed multipart acceptance and JSON envelope acceptance. A red check for either is a finding, not an infrastructure pass or an implemented correction. Tests named “observation” intentionally characterize existing defects; their passing status does not mean the behavior is safe.

## Confirmed findings

Priorities indicate correction order, not a claim that each issue is remotely exploitable in every deployment. P1: address before non-isolated use. P2: focused correctness/security/operations work. P3: lower-impact maintenance/privacy.

### F1 — P1 / high impact, high confidence: pinned storage includes affected authentication code

Affected files: [Dockerfile.storage](../Dockerfile.storage), [docker-compose.yml](../docker-compose.yml). Server source pin `7aac2a2c5b7c882e68c1ce017d8256be2feea27f` is dated 2026-02-12. Its [object handler](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/cmd/object-handlers.go#L1897) contains the signature-verification gating identified by [GHSA-hv4r-mvr4-25vw / CVE-2026-41145](https://github.com/minio/minio/security/advisories/GHSA-hv4r-mvr4-25vw).

Safe reproduction of applicability: compare that exact source with the advisory's affected component and version range; do not send a bypass request. Upstream describes object-write authentication bypass when storage is reachable and a valid access key is known. Expected: writes require credential proof. Actual: affected code is present. Runtime exploitation was not executed or verified, and no bypass test was written or published.

The advisory affects community releases from RELEASE.2023-05-18T00-05-36Z through the final community release; its fixed product is AIStor RELEASE.2026-04-11T03-20-12Z or later. It does not supply a public community correction patch. The introductory public commit is not a fix.

Recommendation: keep this storage isolated from untrusted clients; require Owner/Aether to choose supported storage or an independently reviewed maintained patch route. Do not silently adopt a paid product or another vendor. Validate compatibility, IAM, private access, data migration and recovery before adoption; use approved non-exploit regression methods. Private bucket policy alone does not repair authentication implementation.

### F2 — P2 / medium impact, high confidence: truncated multipart file returns success

Affected: [upload.go](../internal/server/upload.go), especially first-part selection and PutObject with unknown length. Reproduction: `assessment-upload-bounds` sends a three-byte file part without the terminating multipart boundary in the fresh CI stack. Expected: reject malformed/interrupted framing, remove incomplete bytes, persist a failed/retryable record. Actual: HTTP 200; the regression fails.

The SDK's unknown-size upload path treats io.ErrUnexpectedEOF as a permissible short read ([exact dependency source](https://github.com/minio/minio-go/blob/v7.0.97/api-put-object-streaming.go#L350)). Hashing then validates the bytes actually received, which does not establish that the request finished correctly. Impact: callers can receive success for a partial file; changing the declared-size contract alone would not solve framing correctness.

Recommended correction: preserve and check multipart reader errors independently of the storage SDK, validate the required closing boundary and explicit single-file/envelope policy before committing success, and compensate safely on rejection. Regression: retain this failing case; add interrupted/chunked transfer, malformed suffix and extra-part cases; verify no downloadable partial object. Do not reject legitimate declared-versus-actual size differences unless the documented contract is deliberately revised.

### F3 — P2 / low-to-medium impact, high confidence: JSON parser accepts invalid envelopes

Affected: [auth.go](../internal/server/auth.go), with the same single-Decode pattern in register.go, files.go and links.go. `TestAssessmentJSONEnvelopeRejectsOversizeAndTrailingData` calls the actual login handler with a valid synthetic admin object followed by either 17 KiB of whitespace or another JSON object. Expected: enforce the documented 16 KiB envelope and one JSON value. Actual: both return HTTP 200; the regression fails.

MaxBytesReader limits consumed bytes, while one Decode can finish before consuming the rest. The decoded credential fields remain bounded; this is not evidence of an authentication bypass. Recommendation: decode exactly one value, require EOF within the limit and map size/framing errors consistently. Validate all four JSON endpoints with valid-prefix/oversized-suffix, trailing-object and ordinary valid requests.

### F4 — P2 / medium impact, high confidence: SQL calls outlive canceled requests

Affected: [register.go](../internal/server/register.go) QueryRow/Exec/authenticateUser and [download.go](../internal/server/download.go) metadata QueryRow. Safe trigger: cancel a synthetic HTTP request before handler execution, with an observing SQL driver that returns a fixed error. Expected: canceled/bounded SQL context. Actual: each handler reaches the driver with an uncanceled context.

This confirms context propagation failure, not a measured database outage exhaustion. Slow/unresponsive database operations can retain handlers/connections after clients leave; the pool is limited to ten. Recommendation: pass request contexts into authentication and all SQL operations; add bounded operation deadlines, including login bookkeeping and registration. Follow-up: disposable PostgreSQL delay/cancel tests plus pool recovery and graceful shutdown under bounded in-flight work.

### F5 — P2 / medium impact, high confidence: migration runner retains pool connections

Affected: [internal/db/migrate.go](../internal/db/migrate.go). The runner creates a postgres.WithInstance driver and never closes its dedicated connection/source. [Driver ownership behavior](https://github.com/golang-migrate/migrate/blob/v4.19.1/database/postgres/postgres.go#L132) confirms the acquisition.

Reproduction: fresh legacy fixture, two successful RunMigrations calls, then DB.Stats().InUse. Expected: zero borrowed connections after completion. Actual CI: two. Repeated use can consume pool capacity; one normal backend startup loses one slot. No pool-exhaustion test was performed.

Recommendation: explicitly own/release a migration connection and source on success and error; preserve the application's shared DB pool. Calling this driver's Close through WithInstance also closes the shared pool, so that is not a safe mechanical fix. Validate repeated migrations with a small pool, failure paths, pool usability afterwards and startup concurrency.

### F6 — P2 / medium impact, high confidence: tracked backend artifact bypasses accepted rebuild

Affected: tracked [backend](../backend), Docker/build distribution hygiene. Safe trigger: `go version -m backend` and static `govulncheck -mode=binary backend`; never execute it. Actual: Go 1.24.11, dirty older build metadata, pgx v5.7.6 and x/text v0.31.0; scanner reports 33 advisory IDs with vulnerable symbols, versus zero reachable findings for current source on Go 1.26.9.

Examples: [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629) (x/text, fixed v0.41.0), [GO-2026-5004](https://pkg.go.dev/vuln/GO-2026-5004) (pgx, fixed v5.9.2), and standard-library advisories fixed by supported Go releases. Binary symbol presence is not proof each issue is reachable through this application.

Impact requires running/distributing the stale artifact. The reviewed Dockerfile rebuilds source and .dockerignore excludes this binary, so this is separate from the accepted container. Recommend remove tracked build output under separate approval and document supported reproducible artifacts. Regression: artifact provenance/source SHA checks and distribution scan.

### F7 — P2 / conditional impact, high confidence in version matches: runtime/storage dependencies remain affected

Affected: Dockerfile.backend, Dockerfile.storage and the pinned storage dependency graph.

Actual Alpine runtime package and storage-binary results: The corrected container observation at `7e78d1e581c44afe34c0b3b74f11bf34d63479d0` found zlib 1.3.2-r0 in both backend and storage images (17 Alpine packages each), with one applicable official package-version advisory per image. The copied actual storage binary produced 75 distinct Go advisory IDs across symbol, imported-package and module-only evidence. These 75 are not 75 demonstrated reachable vulnerabilities; the final scanner explicitly distinguishes those levels in CI.. Version/advisory matches require path-specific reachability assessment; they are not a blanket exploit claim. The official [Alpine v3.24 main secfix database](https://secdb.alpinelinux.org/v3.24/main.json) reports zlib 1.3.2-r1 as fixed for CVE-2026-85091; initial actual images contained 1.3.2-r0. [Upstream acknowledges the fix](https://github.com/madler/zlib/issues/1310#issuecomment-5706541034). This concerns native gzip writer behavior, not a demonstrated HTTP upload exploit.

The actual storage scan includes broader dependencies; examples below are inventory/path evidence, not remote exploit demonstrations. Some Go database entries for MinIO are explicitly unreviewed and need upstream correlation.

| Actual storage dependency | Matched area / prerequisite | Compatibility review target |
| --- | --- | --- |
| golang.org/x/net v0.39.0 | HTTP/2 and IDNA paths; actual protocol/hostname processing must be established | v0.60.0 covers the reported HTTP/2 batch; retest transports |
| golang.org/x/crypto v0.37.0 | SSH symbols; an exposed/configured SSH feature is a separate prerequisite | v0.56.0 covers listed SSH fixes; validate server build/features |
| google.golang.org/grpc v1.72.0 | Transport handling; feature/service reachability unverified | v1.83.1 for the listed latest transport fixes |
| github.com/go-jose/go-jose/v4 v4.1.0 | JWE processing; relevant identity configuration/input path unverified | v4.1.4; verify identity integration compatibility |
| MinIO server at the exact pin above | Vendor advisories, including F1 and conditional features | No public fixed community release established |

Full per-ID module/version/fix observations are retained in the linked CI job, with module/package/symbol levels distinguished. The application's Go scan does not cover that binary. Recommend separately approve compatible dependency/package updates and reproducible rebuilt images, then re-scan actual runtime layers and storage binaries; assess optional notification/identity/transport features before assigning application exposure. No dependency change was made.

### F8 — P2 / medium correctness impact, high confidence in configuration mismatch: proxy file/envelope limits disagree

Affected: docker-compose.yml buffering maxRequestBodyBytes uses exactly SFD_MAX_UPLOAD_BYTES; backend permits that many file bytes plus multipart overhead. Safe trigger: serialize a legal 1024-byte file multipart body; the complete body exceeds 1024. [Traefik's contract](https://doc.traefik.io/traefik/v2.11/middlewares/http/buffering/) rejects bodies above the configured envelope limit with 413. Expected: the advertised file limit works through the supported proxy. Configured behavior: an exact-limit file is rejected before reaching the backend.

This is a source/configuration proof; the actual Compose/TLS path was not executed. Recommendation: distinguish file and HTTP envelope limits consistently at both layers; test a real exact-limit upload through the proxy. Review buffering disk capacity, request timeout and bounded concurrency at the same time.

### F9 — P3 / low impact, high confidence: browser session cleanup is incomplete

Affected: web/static/index.html logout and upload XHR progress callbacks. Reproduction: the actual inline script under the bounded Node harness. Actual: registration password/confirmation values survive logout; old upload progress updates mutate a later session's progress text and the old XHR is not aborted. Expected: clear credential fields and prevent old-session callbacks from changing current-session UI.

No DOM injection or cross-user server authorization bypass was demonstrated. Registration fields and upload size/progress can linger on a shared browser session. Recommendation: clear all credential inputs after successful use/logout, cancel owned in-flight requests and guard every progress/callback by generation. Retain both observation tests but reverse their assertions with the correction; validate real login/logout/account-switch/upload in a browser.

### F10 — P3 / low impact, high confidence: command-line diagnostics can report misleading success/failure

Affected: scripts/validate-env.sh and native/sfd_hash_cli.c. Two separate bounded triggers:
- Synthetic complete config with a 32-byte session secret: the first warning exits 1 before the documented warning-only success summary, because set -e combines with post-increment returning status 1.
- Compile the real native CLI, hash a three-byte temporary file and direct stdout to /dev/full: exit is 0 although output failed. printf/flush errors are not checked.

Expected: validator accumulates diagnostics and applies its stated exit policy; native CLI reports output-I/O failure. Impact: misleading automation, not a demonstrated backend hashing bypass (the backend reads a pipe). Focused fixes: safe counter increments; check output/flush status. Regression: warning-only/multiple-error synthetic environment fixtures and native stdout failure. Validator placeholder diagnostics should also avoid echoing values.

## Risks requiring investigation, not confirmed exploits

- **Ambiguous COMMIT acknowledgement:** upload's failure defer removes storage on any commit error. If commit succeeded but its response was lost, metadata may remain hashed while bytes are removed. The known-rejected deferred-trigger commit test passes; it does not simulate an unknown commit outcome. Review compensation/reconciliation under ownership locks before deciding a correction. Do not implement a speculative lifecycle redesign in this PR.
- **Slow clients and capacity:** HTTP server sets only ReadHeaderTimeout; body/idle/write limits, admission control, account quotas and backend brute-force throttling are absent. Upload parsing before finding the file part precedes its five-minute operation context. Context deadlines alone do not necessarily stop blocked body reads. No exhaustion, long-running load or maximum-size test was performed. The configured proxy and 50 GiB default need representative capacity/time budgets.
- **Proxy origin/redirect:** forwarded headers are deliberately ignored. Without a validated public origin, Host controls fallback links. The Compose HTTP redirect omits the external TLS port 8443; confirm actual Location and correct external port/canonical-origin handling in a dedicated proxy fixture. Do not trust arbitrary Host/forwarding input in public deployment.
- **Readiness scope:** the existing bad-native-tool container becomes /ready=200 but uploads fail. /ready currently checks DB/bucket, not executable usability, disk headroom, schema drift or policy drift. Define readiness versus startup validation explicitly; do not infer full upload readiness from liveness or bucket existence.
- **Storage/deletion/recovery:** object deletion precedes metadata deletion, so a database failure after successful removal can leave a dangling row until retry. Cleanup retains pending/failed rows on storage failure, but session expiry deletion shares that cleanup transaction and can be deferred by storage failures/disabled cleanup. Hashes prove received bytes at upload time, not future immutability after storage compromise.
- **Unknown manual schemas/historical ownership:** only documented empty and early files-only upgrades were tested. Do not infer owners or auto-reconcile real installations. Existing schema, backup, ownership and deployment decisions remain with Owner/Aether.

Optional maintainability: replace legacy tests that recreate handler logic rather than calling production handlers; avoid historical test totals as acceptance evidence; pin mutable image/action identities where operationally appropriate; document counters that reset per process and unused active-session metrics. No roadmap feature was implemented.

## Endpoint and implementation coverage

| Surface | Actual chain and observed/inspected contract |
| --- | --- |
| All routes | Request ID → quoted path-only logging → Origin check → ServeMux. Query strings/cookies are not request-log fields. |
| /login, /register | Public POST; finite decoded JSON bounds, bcrypt bytes 8–72 for registration, ordinary signed role, DB sessions. Envelope/cancellation defects F3/F4. |
| /logout, /me | Session signature/expiry → persisted token hash → active ordinary user; logout revokes only that session. |
| GET/POST /files | requireAuth; owner-filtered listing / principal-owned metadata; administrator creates ownerless metadata. |
| /upload | requireAuth; owner/admin row query + lock; pending-only; stream storage → native hash/count → commit; F2 and uncertain-commit risk remain. |
| /links | requireAuth; owner/admin status query; UUID validation; default 300-second and maximum 86400-second TTL. |
| /download | Capability HMAC/expiry → allowed file status → object stat/stream; no session required. Replay until expiry/deletion is the documented contract. SQL cancellation F4. |
| /admin/files, /admin/files/ID, /admin/cleanup, /metrics | requireAdmin wraps requireAuth; ordinary/anonymous rejection exercised. Delete uses exact object key; cleanup uses SKIP LOCKED. |
| /health, /ready, /version | Public process liveness, DB/bucket readiness, build metadata; no broader deployment certification. |
| /, /static/ | Local asset directory; filename/error text uses escaping. DB-constrained UUID/status and numeric metrics limit the other templated fields. No demonstrated untrusted DOM injection. |
| Native C | Bounded stack buffer, OpenSSL context cleanup, checked reads/digest calls, correct known vectors; output-I/O diagnostic F10. No memory defect identified by supported checks. |
| SQL/migrations | Parameterized request values, ownership query predicates and schema constraints inspected; real fresh/legacy/idempotent upgrade executed in disposable fixtures. F5 and unknown-schema gate remain. |
| Build/operations | Non-root rebuilt backend/storage, no shell in native execution, excluded environment/credential build context, loopback test ports, ownership-checked teardown; Docker socket in Compose and root storage credentials remain local-development assumptions. |

No new authentication signature forgery, ordinary-to-admin escalation, cross-owner file access or anonymous private-bucket read was identified in the executed ordinary regression suite. This is limited evidence, not proof of absence.

## Operating matrix

| Scenario | Verdict | Evidence / boundary |
| --- | --- | --- |
| Fresh startup; documented legacy upgrade; repeat migration | Passed, with F5 defect | Real disposable PostgreSQL; schema version 4 clean; repeated calls retain connections. |
| Registered owner upload → native C hash → anonymous signed download | Passed for bounded API fixtures | Actual runtime/storage images; matching hash, byte counts and downloaded bytes. |
| Anonymous, ordinary/admin, cross-owner and ownerless boundaries | Passed | Real route matrix; malformed/nonexistent IDs; deactivation and revoked/expired sessions. |
| Cookie signing/roles, tampering, independent sessions/logout replay | Passed within tested cases | Real CI plus unit cookie attributes; no claim of browser/TLS-proxy acceptance. |
| Empty, 1-byte and configured 1024-byte direct API uploads | Passed | Declared size replaced by actual; native counts match; oversized bytes rejected. |
| Truncated multipart upload | Failed | HTTP 200; retained F2 regression. |
| Strict JSON envelope/16 KiB total limit | Failed | HTTP 200 for oversize suffix and trailing object; retained F3 regression. |
| Two concurrent uploads; upload/delete/cleanup locking | Passed | One upload wins, one conflicts; stalled tiny upload blocks delete, cleanup skips locked row. |
| Client cancellation during upload | Passed for bounded fixture | Transfer ends and failed metadata persists; different SQL cancellation defect F4 remains. |
| Known-rejected DB commit during upload | Passed | Deferred fixture constraint; object removed, failed metadata retained. |
| Lost COMMIT acknowledgement / process crash between systems | Unverified | Requires additional fault model; no atomic storage/DB claim. |
| Storage outage, delete retry, readiness/liveness and restart | Passed | Metadata retained on storage failure; storage recovers and exact object removal succeeds. |
| DB outage and restart | Passed for established service | /health=200, /ready=503, /me=500; readiness recovers after restart. |
| Missing native executable | Failed upload; readiness limitation | /ready=200 while native upload fails; failed row retained, no usable link. |
| Cleanup and owned-resource teardown | Passed in observed CI run | Failed upload cleanup; no ownership-refusal/removal errors. |
| Idle SIGTERM and backend restart | Passed | Exit 0, shutdown acknowledgement, readiness and persisted session after restart. |
| Graceful shutdown with active long transfer | Unverified | Idle result does not establish in-flight draining or compensation. |
| Disposable recovery | Passed narrowly | pg_dump/psql row-count/migration readback and one synthetic object-copy byte comparison. |
| Real backup/whole-service disaster recovery | Unverified | No real backup or existing data accessed; no coordinated snapshot/recovery-time certification. |
| Native compiler/analyzer/sanitizer vectors | Passed | Five vectors and input-I/O checks; standalone stdout failure is F10. |
| Source dependency reachability | Passed narrowly | Zero reachable application Go advisories; imported/module matches remain. |
| Runtime/storage dependency security | Failed clean-security criterion | Actual package/binary advisory matches; applicability levels distinguished. |
| Actual authenticated browser upload → link → download | Blocked locally | Docker engine absent/access-denied/stalled after permitted startup attempt; CI API/Node mocks do not substitute. |
| Supported Compose/TLS/proxy path | Unverified; F8 mismatch identified | No trusted-browser TLS/proxy execution; canonical-origin, external port, envelope and capacity gates remain. |
| Maximum configured upload size / throughput | Unverified | Only modest fixtures; no 50 GiB, disk exhaustion, unbounded fuzzing or load claim. |
| Existing/manual schemas and historical ownership | Unverified / separate approval | No real installation inspected or mutated. |

## Validation and reproduction record

- Local Go 1.26.9 before adding failing input regressions: `go test ./... -count=1`, `go vet ./...`, backend build and `go test -race ./internal/... ./cmd/... -count=1` passed. Full-stack package explicitly skipped without SFD_E2E; not counted as acceptance.
- Final local review-tree rerun: go test ./... and the race suite both exited nonzero solely on the retained F3 JSON regression; no race warning was reported. Independent go vet and backend build passed. These failed suites are not reported as passes.
- Node 24.19.0: existing three session/escaping tests and two new observation tests passed. They run the actual inline script in a mocked DOM/network, not a browser.
- GCC 13.3: five real SHA-256 vectors plus usage/input-I/O checks passed under -Wall -Wextra -Wpedantic -Wconversion -Werror -fanalyzer and separately AddressSanitizer/UndefinedBehaviorSanitizer with leak detection. No sanitizer finding. The distinct stdout-I/O case fails its expected exit contract (F10).
- Pinned golangci-lint v2.14.0: zero issues after fixture corrections; no new exclusions or masked checks. Initial d5f44f7 had genuine fixture lint failures, retained in run history.
- Fresh application `govulncheck@v1.9.0 -show verbose ./...`: zero reachable findings, 2 imported-package and 30 module-only advisories. Imported packages: compress/s2 v1.18.0 ([GO-2026-5841](https://pkg.go.dev/vuln/GO-2026-5841), fixed v1.18.7; requires crafted dictionary encoding) and x/net/idna v0.47.0 ([GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026), fixed v0.55.0; hostname conversion/authorization preconditions). No reachable application calls identified. Module-only SSH/HTML/HTTP2/Windows findings are not demonstrated application exploits.
- Archived-storage advisory/source review: F1; [SSE metadata advisory GHSA-3rh2-v3gr-35p9](https://github.com/minio/minio/security/advisories/GHSA-3rh2-v3gr-35p9) affects community releases after March 2024 and requires storage write authority; fixed AIStor March 2026. [ReadMultiple GHSA-xh8f-g2qw-gcm7](https://github.com/minio/minio/security/advisories/GHSA-xh8f-g2qw-gcm7) requires a distributed deployment and root cluster credential; the checked-in single-node standalone layout does not register that route. No runtime exploitation of either was attempted. These advisories identify AIStor fixes, not an available public patched community release.
- CI runtime scan reads actual owned image APK databases, checks official v3.24 main/community fixed versions using apk's comparator, and scans the copied actual storage binary. It excludes PostgreSQL's Debian packages, Traefik's actual binary and non-Alpine package ecosystems; those remain unverified. It is not a whole-image security certification.
- [Corrected container run 38816035828](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38816035828) tested `7e78d1e581c44afe34c0b3b74f11bf34d63479d0`. Go/native/security and docs jobs passed. The real container test executed in 228.70 seconds and failed on F2; nine other assessment subtests passed, including cancellation/locking, known commit rejection, dependencies as observations, fixture recovery, outage and idle restart. Scanner matches are reported, not treated as a clean-security pass. Initial d5f44f7 runs also contain the corrected-later shutdown-log capture and lint fixture failures. The later F3 failing test and advisory-level classification were validated locally; final-head CI/readback is maintained in the PR handover to avoid a self-referential report SHA.
- Interruption audit: no partial storage bypass file existed; no bypass operation executed or was published. The pending tool command never started after automatic approval review timed out. It was not retried. Subsequent work follows the Owner's source/advisory-only restriction. No lingering assignment process was found at that checkpoint.
- Existing original checkout/local documents, prior producer branch, WSL feature branch, project mapping/remotes and unrelated .qodo content were preserved. No reset, force push, branch deletion, production migration, storage operation on existing data, deployment or Docker repair occurred.

## Recommended correction package, pending Owner/Aether approval

1. Decide storage support/remediation and restrict exposure; review actual runtime/package/storage advisories and stale artifact distribution together (F1/F6/F7). No paid/vendor choice is implied.
2. Correct multipart error/framing handling and strict JSON envelopes (F2/F3); make the retained failing regressions pass and add interrupted/extra-part cases.
3. Repair SQL context/deadline propagation and migration connection ownership (F4/F5); investigate ambiguous commits with a bounded failure fixture before choosing lifecycle changes.
4. Align proxy envelope/canonical-origin/redirect behavior and operational admission/time budgets (F8 and risks). Validate the actual supported TLS/proxy path.
5. Clear/cancel browser session state, correct diagnostics and replace shallow legacy tests (F9/F10 and maintenance).
6. Re-run unit/race/lint/native/vulnerability/container checks, actual authenticated browser upload → link → download and bounded recovery. Existing-schema/ownership reconciliation, maximum configured file support and public release approval remain separate gates.

This PR completes assessment/publication only. It requests review of findings and the correction package; it grants no production-code or dependency implementation authority.
