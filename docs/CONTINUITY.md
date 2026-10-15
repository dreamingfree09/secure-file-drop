# Secure File Drop continuity

## Active assignment — 2026-10-15

- Work ID: SFD-QUALITY-CLOSEOUT-20261015-CODEX.
- Producer/branch owner: Codex. Recipient: Aether; planning/review destination: “GitHub workflow implementation”.
- Scope: Owner-authorised repository review, GHP-01 installation, confirmed defect correction, isolated validation and one PR. Significant design decisions stay with Owner/Aether.
- Base: `1a0f7715faa89c53fcec266f59e43cad9b1a057b`, verified local master, fetched remote master and GitHub API.
- Owned branch: `codex/sfd-quality-closeout-20261015`; target: master. Integration is not authorised.
- Workflow installation is a separate documentation commit. Application review and validation follow.
- Current claims in historical readiness documents and test totals require fresh evidence.

## Checkout discovery and preservation

The configured `E:\PROGRAMMING PROJECTS\Secure File Drop` is an older independent checkout at `853b865e30872964a556c225335eb2191928972b`, with no remote. Its only branch is master; its two committed changes are ancestors of the GitHub baseline. Its tracked tracker edits mark milestones 1, 5 and 9 in progress. Untracked native C source/header are byte-identical to baseline; its native Makefile is empty and README older. No missing local application implementation was found in cmd/, internal/ or web/. Private configuration files and local notes were identified by metadata only and remain unread/unpublished. Preserve all of this; no reset, branch switch, remote addition or import is authorised there. Local notes/documentation remain outstanding local reconciliation, not audit source.

`C:\Dev\secure-file-drop` contains only .qodo and was not modified. A clean existing WSL checkout at `/home/dreamingfree09/Secure File Drop` was discovered on feature/v2-enhancements (`2b8e56d`). Before the Owner's clarification, Codex created the named review branch there from the verified baseline without file changes. That checkout and its feature branch are preserved; no further implementation occurs there.

The Owner explicitly superseded the no-new-checkout restriction and authorised the unique sibling clone `E:\PROGRAMMING PROJECTS\secure-file-drop-quality-closeout-20261015-codex`. Its root and credential-free GitHub origin were verified before changes. This is the audit checkout. No project mapping changed.

## Handover state

Implementation/validation: active. Publication: pending. Acknowledgement: pending Aether review at exact published SHA. Integration: pending review, not authorised. Final revision and full changed-file scope will be published in the PR handover after committing.

Next action: inspect source and destructive test setup, establish baseline on disposable local resources, reproduce confirmed defects and add focused regressions.

## Review decisions recorded before implementation

The baseline supports public registration already; preserve it and enforce its intended private ownership. Environment administrator credentials retain administration. Registered UUID subjects are ordinary users; the administrator marker must be part of the signed session, never inferred from user-supplied names. Existing unowned rows remain administrator-only; ownership cannot safely be reconstructed.

Use the existing PostgreSQL dependency for hashed session-token revocation so logout invalidates replay across processes; expire sessions by the same signed expiry. Add a forward migration rather than editing applied migrations. Use row locks and transactional pending → stored → hashed transitions to serialize upload, deletion and cleanup; avoid a new externally visible lifecycle state. Storage deletion must succeed before metadata is forgotten, preserving retryability.

Keep bearer signed downloads usable without a session. Record actual uploaded/hash byte counts, enforce file limits independently of multipart overhead, sanitize filenames and format attachment headers. Local UI uses repo-relative assets with an explicit override. Retain the roadmap; do not import unmerged v2 features.

Baseline: Go 1.24.11 unit packages passed; go vet ./... and backend compilation passed; native C build passed. Legacy e2e not executed: fixed ports, unbounded resource cleanup and native Go fallback violate isolated/native acceptance. Replacement will fail on missing dependencies when explicitly enabled, and report a clear skip otherwise.

## Producer review results

Confirmed findings, fixes, reproduced baseline failures and distinct validation outcomes are maintained in [TRACKER.md](TRACKER.md). The final PR will carry the exact final SHA, full changed-file scope, publication/CI status and remaining blockers. Historical snapshots and future ideas are retained.

Go/CI/native configuration is aligned to supported Go 1.26.9 and Alpine 3.24. [Official Go releases](https://go.dev/dl/) and [Alpine support](https://alpinelinux.org/releases/) were checked on the assignment date. Reachable dependency advisories were patched; see the tracker for paths and limitations.

Local Docker validation is blocked by Docker Desktop inference-socket startup failure. No settings were changed and no existing containers/volumes were used. The only local UI service is an assignment-owned static loopback server, which must be stopped after browser checks.

## Storage dependency decision — before source-image implementation

First CI at 91ca11c passed Go/native/race/vulnerability/lint/docs checks, but could not pull the historical minio/minio image. The official [MinIO server repository](https://github.com/minio/minio) is archived and documents source-only community distribution; the client is also archived. Do not silently switch storage vendors or paid services.

The authorised test/development image will therefore build the same community product from exact verified official archive commits: server 7aac2a2c5b7c882e68c1ce017d8256be2feea27f and client 77f82e18b5401a65958f1619df6ebb994634bd88. These are newer sources than the unavailable 2024 image; no existing volume is upgraded or reused. Preserve upstream licenses and source identities in Dockerfile.storage. Production maintenance/vendor/version adoption remains an explicit Owner/Aether decision; the isolated build is not support or vulnerability certification. First CI also exposed mixed Docker pull stderr/container stdout in the harness; capture IDs from stdout only so ownership-checked cleanup can work.
