# Secure File Drop continuity

## Community storage evaluation — 2026-10-16

SFD-STORAGE-EVALUATION-20261016-CODEX is an Owner-authorised evaluation only, on separate codex/sfd-storage-evaluation-20261016 from exact reviewed application 4a9b6c5ee9a7b0f6390e62f0465636daa42a2a23. [PR #23](https://github.com/dreamingfree09/secure-file-drop/pull/23) targets the reviewed producer branch; PR #22, accepted master 0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7 and all preserved checkouts/work remain unchanged. The [canonical storage evaluation](STORAGE_EVALUATION_20261016.md) records pinned SeaweedFS 4.48, triggered RustFS 1.0.2 fallback, actual outcomes, advisory residuals and migration/rollback proposal. The bounded evaluation is complete: both candidates pass functional/permission/browser/recovery checks; SeaweedFS 4.48 is unsuitable for current adoption because shipped security updates remain, while RustFS 1.0.2 is compatible with specified conditions. Final exact SHA/CI/readback and pending independent Aether acknowledgement are in PR #23. Owner/Aether must decide whether to authorise a separate RustFS adoption/configuration and migration-rehearsal proposal; adoption, public deployment, real-data/schema/ownership reconciliation, capacity and whole-service recovery gates remain open. No production configuration or dependencies have changed.

## Focused corrections — 2026-10-16

Work ID: SFD-DEEP-CORRECTIONS-20261016-CODEX. The Owner approved focused application/runtime corrections and bounded validation following [Aether's assessment acknowledgement](https://github.com/dreamingfree09/secure-file-drop/pull/22#issuecomment-6204810242). Producer/branch owner: Codex; recipient: Aether in “GitHub workflow implementation”. Received assessment: `1f30bb374420d3285ebdaac0c192240897c5d0f4`; accepted master remains `0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7`. Work continues on existing `codex/sfd-deep-review-20261015` / [PR #22](https://github.com/dreamingfree09/secure-file-drop/pull/22); assessment commits and dated findings remain history.

F2/F3/F4/F5/F6/F8/F9/F10 corrections, compatible application/runtime remediation, conservative COMMIT handling and bounded native-readiness/transfer-shutdown corrections are implemented. The canonical [assessment correction appendix and operating matrix](DEEP_REVIEW_20261015.md#approved-correction-status--2026-10-16) records finding status, actual checks, source revisions and limits. Correction code/test/CI revision `709942ee7a490a9d36c647760a21c4c74bd2c51c` passed all jobs in both push and PR CI, including all 15 real-stack scenarios, the authenticated Chromium journey and actual proxy. Producer implementation/validation is complete; storage and integration decisions remain open. The final full SHA, complete scope, exact CI readback and independent-review state belong to the PR handover.

F1 and storage's part of F7 remain awaiting decision. Recommendation: separately authorise a SeaweedFS Community compatibility/security evaluation, with RustFS as an alternative; no replacement or migration is authorised here. Private-network/no-host-port isolation is temporary containment, not a MinIO patch. The bypass reproduction remains excluded and unexecuted. Existing-schema/ownership reconciliation, representative maximum-size/slow-client capacity, production storage, public TLS/ingress and whole-service recovery remain gates. No public-production readiness is claimed.

Original no-origin checkout at `853b865e30872964a556c225335eb2191928972b` retains one tracked-change entry and four untracked entries without exposing their contents. WSL `feature/v2-enhancements` remains clean at `2b8e56de4b79e2508125ca02c18455ae88dee7f6`. The previous producer branch remains `c00d6c48d57082bce51d4ca6096d5cf9aab3705f`; unrelated .qodo, remotes and project mapping were not changed. Normal commits/pushes only; no master write, merge, deployment, existing-data operation or host repair.

Producer completion/publication and independent acknowledgement are separate. Aether's exact-final-revision acknowledgement and Owner's later integration approval remain pending. Next action: independently review the published correction SHA and decide the separate storage evaluation; do not merge or deploy solely from this handover.

## Deep assessment handover — 2026-10-15

Work ID: SFD-DEEP-REVIEW-20261015-CODEX. Owner-authorised inspection, isolated validation, regression tests and findings publication only. Producer/branch owner: Codex; recipient: Aether in “GitHub workflow implementation”. Received/expected accepted master: `0c63d33b037f4720789353ecc0d0ecf6e2c5f9a7`, verified root/origin/branch/worktree, clean receipt and matching remote master before creating `codex/sfd-deep-review-20261015`. The earlier producer branch and preserved checkouts/local work remain untouched.

The canonical [deep assessment](DEEP_REVIEW_20261015.md) contains prioritised findings, reproduction evidence, advisory applicability, scenario verdicts and the recommended correction package. [PR #22](https://github.com/dreamingfree09/secure-file-drop/pull/22) carries the complete exact-revision publication handover and final CI readback. Assessment/publication is complete; production-code fixes, dependency updates and master integration remain pending Owner/Aether approval. Genuine failing input regressions are retained, not presented as green acceptance.

The storage signature-bypass reproduction was not written, executed or published; source/advisory applicability is distinct from an unverified runtime exploit. Local Docker/authenticated-browser gaps, production-storage maintenance, existing-schema/ownership reconciliation, maximum-size capacity and deployment/recovery gates remain open. No public-production readiness is claimed. Next action: Aether independently reviews the exact published revision and correction package before implementation authority is granted.

## Completed assignment — 2026-10-15

- Work ID: SFD-QUALITY-CLOSEOUT-20261015-CODEX.
- Producer/branch owner: Codex. Recipient: Aether; planning/review destination: “GitHub workflow implementation”.
- Scope: Owner-authorised repository review, GHP-01 installation, confirmed defect correction, isolated validation and one PR. Significant design decisions stay with Owner/Aether.
- Base: `1a0f7715faa89c53fcec266f59e43cad9b1a057b`, verified local master, fetched remote master and GitHub API.
- Preserved producer branch: `codex/sfd-quality-closeout-20261015` at `c00d6c48d57082bce51d4ca6096d5cf9aab3705f`; target: master. The Owner subsequently approved exact-revision integration, local adoption and bounded audit closure, as recorded below.
- Workflow installation remains a separate documentation commit in the accepted history. Application review and bounded validation are complete.
- Current claims in historical readiness documents and test totals require fresh evidence.

## Checkout discovery and preservation

The configured `E:\PROGRAMMING PROJECTS\Secure File Drop` is an older independent checkout at `853b865e30872964a556c225335eb2191928972b`, with no remote. Its only branch is master; its two committed changes are ancestors of the GitHub baseline. Its tracked tracker edits mark milestones 1, 5 and 9 in progress. Untracked native C source/header are byte-identical to baseline; its native Makefile is empty and README older. No missing local application implementation was found in cmd/, internal/ or web/. Private configuration files and local notes were identified by metadata only and remain unread/unpublished. Preserve all of this; no reset, branch switch, remote addition or import is authorised there. Local notes/documentation remain outstanding local reconciliation, not audit source.

`C:\Dev\secure-file-drop` contains only .qodo and was not modified. A clean existing WSL checkout at `/home/dreamingfree09/Secure File Drop` was discovered on feature/v2-enhancements (`2b8e56d`). Before the Owner's clarification, Codex created the named review branch there from the verified baseline without file changes. That checkout and its feature branch are preserved; no further implementation occurs there.

The Owner explicitly superseded the no-new-checkout restriction and authorised the unique sibling clone `E:\PROGRAMMING PROJECTS\secure-file-drop-quality-closeout-20261015-codex`. Its root and credential-free GitHub origin were verified before changes. This is the audit checkout. No project mapping changed.

## Handover state

Implementation and bounded quality audit: complete. Publication: [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21), whose handover records the exact approved proposal, complete scope and final CI readback. Acknowledgement: [Aether accepted the exact revision within stated limits](https://github.com/dreamingfree09/secure-file-drop/pull/21#issuecomment-6201929960). Integration: completed by Aether after Owner approval; local adoption verified below.

Next action: Aether independently reads back the continuity-only completion commit and acknowledgement on PR #21. Outstanding operational and product decisions below require separately authorised work.

## Review decisions recorded before implementation

The baseline supports public registration already; preserve it and enforce its intended private ownership. Environment administrator credentials retain administration. Registered UUID subjects are ordinary users; the administrator marker must be part of the signed session, never inferred from user-supplied names. Existing unowned rows remain administrator-only; ownership cannot safely be reconstructed.

Use the existing PostgreSQL dependency for hashed session-token revocation so logout invalidates replay across processes; expire sessions by the same signed expiry. Add a forward migration rather than editing applied migrations. Use row locks and transactional pending → stored → hashed transitions to serialize upload, deletion and cleanup; avoid a new externally visible lifecycle state. Storage deletion must succeed before metadata is forgotten, preserving retryability.

Keep bearer signed downloads usable without a session. Record actual uploaded/hash byte counts, enforce file limits independently of multipart overhead, sanitize filenames and format attachment headers. Local UI uses repo-relative assets with an explicit override. Retain the roadmap; do not import unmerged v2 features.

Baseline: Go 1.24.11 unit packages passed; go vet ./... and backend compilation passed; native C build passed. Legacy e2e not executed: fixed ports, unbounded resource cleanup and native Go fallback violate isolated/native acceptance. Replacement will fail on missing dependencies when explicitly enabled, and report a clear skip otherwise.

## Producer review results

Confirmed findings, fixes, reproduced baseline failures and distinct validation outcomes are maintained in [TRACKER.md](TRACKER.md). The published PR carries the exact proposal SHA, full changed-file scope, publication/CI status and remaining limitations. Historical snapshots and future ideas are retained.

Go/CI/native configuration is aligned to supported Go 1.26.9 and Alpine 3.24. [Official Go releases](https://go.dev/dl/) and [Alpine support](https://alpinelinux.org/releases/) were checked on the assignment date. Reachable dependency advisories were patched; see the tracker for paths and limitations.

Local Docker validation is blocked by Docker Desktop inference-socket startup failure. No settings were changed and no existing containers/volumes were used. The assignment-owned static loopback server was stopped after browser checks.

## Storage dependency decision — before source-image implementation

First CI at 91ca11c passed Go/native/race/vulnerability/lint/docs checks, but could not pull the historical minio/minio image. The official [MinIO server repository](https://github.com/minio/minio) is archived and documents source-only community distribution; the client is also archived. Do not silently switch storage vendors or paid services.

The authorised test/development image will therefore build the same community product from exact verified official archive commits: server 7aac2a2c5b7c882e68c1ce017d8256be2feea27f and client 77f82e18b5401a65958f1619df6ebb994634bd88. These are newer sources than the unavailable 2024 image; no existing volume is upgraded or reused. Preserve upstream licenses and source identities in Dockerfile.storage. Production maintenance/vendor/version adoption remains an explicit Owner/Aether decision; the isolated build is not support or vulnerability certification. First CI also exposed mixed Docker pull stderr/container stdout in the harness; capture IDs from stdout only so ownership-checked cleanup can work.

Storage-policy refinement: retain valid policies restricted to named principals; reject anonymous Allow grants and fail closed on unsupported principal forms. Deny policies remain valid. This preserves private-bucket configurations while enforcing privacy.

Browser follow-up: registration rejects a synthetic 73-byte password before network submission. The temporary browser tab was closed and the static server stopped after verifying its unique loopback listener and command line. No test accounts were created in an existing service.

The unused WSL checkout was restored to its original clean feature/v2-enhancements branch after verifying no intervening edits and the original commit. The unused local review ref remains, with no application changes or branch deletion. All implementation/publication occurs in the authorised sibling clone.

Producer code and documentation are published in [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21). Its handover is authoritative for the proposal SHA, scope and CI evidence; subsequent Aether acknowledgement and Owner-approved integration are recorded in the closure below.

Real-container acceptance passed at 8668a727c2dbf7bcc7980c125934b371b980bd0e; [the tracker](TRACKER.md) links the successful CI run. Final revision/CI status is maintained in the PR to avoid self-referential commit hashes. Local Docker Desktop/authenticated-browser blockers and upstream storage-maintenance decisions remain distinct from CI acceptance.

## Owner-approved integration and bounded audit closure — 2026-10-15

The Owner explicitly approved PR #21 integration, local adoption and bounded quality-audit closure with the documented limitations retained. [Aether's independent review](https://github.com/dreamingfree09/secure-file-drop/pull/21#issuecomment-6201929960) accepted source/CI evidence at proposal `c00d6c48d57082bce51d4ca6096d5cf9aab3705f` as a development/test quality proposal. Aether then integrated [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21) on GitHub at 2026-10-15 20:48:36 UTC. Codex verified that existing integration; no second merge was performed.

- Accepted merge: [`2125ff7a9d67a178c8acd6f7032d3f73e7457a92`](https://github.com/dreamingfree09/secure-file-drop/commit/2125ff7a9d67a178c8acd6f7032d3f73e7457a92).
- Verified ordered parents: previous master `1a0f7715faa89c53fcec266f59e43cad9b1a057b`, then approved proposal `c00d6c48d57082bce51d4ca6096d5cf9aab3705f`.
- Verified merge/proposal tree equality: `a354fc20961a7c52da02674ee8cc440715664fe8`.
- Local adoption: verified root `E:\PROGRAMMING PROJECTS\secure-file-drop-quality-closeout-20261015-codex`, origin `https://github.com/dreamingfree09/secure-file-drop.git`, clean working tree, branch refs and sole worktree before mutation. Local master was exactly the previous master above. After fetching and checking the merge, `git switch master` and `git merge --ff-only 2125ff7a9d67a178c8acd6f7032d3f73e7457a92` succeeded; HEAD matched the accepted merge with a clean tree.
- Producer branch remains at the approved proposal. The original no-origin Windows checkout and its local work, original WSL feature branch, and `C:\Dev\secure-file-drop/.qodo` remain preserved. Their reconciliation status above is unchanged; no project mapping or remotes changed.
- Closure publication: one normal continuity-only completion commit on master, with the accepted merge as its sole parent. Its exact final SHA and publication verification belong in the PR #21 completion acknowledgement because a commit cannot contain its own SHA. Completion requires matching local HEAD, origin/master and remote master, whitespace/link checks and exact published-content readback.
- The repository review, authorised fixes and development/test quality audit are explicitly closed within the accepted evidence and limitations. This is not public production readiness or a production release.

Outstanding gates retained after audit closure:

- Owner/Aether must decide production storage maintenance, vendor/version adoption and security support for the archived, pinned MinIO sources. Development/test reproducibility is not production maintenance approval or OS/storage-image vulnerability certification.
- An authenticated browser upload journey remains unverified locally. Real full-stack API CI acceptance, script tests and partial browser observations do not close that gap. Windows Docker repair remains outside this closure.
- Unknown existing/manual schemas, historical ownerless rows and any real-data ownership reconciliation require separate assessment and approval before migration or reassignment.
- Public TLS/proxy and deployment configuration, backup/restore and recovery evidence, maximum-file throughput and operational release approval remain open deployment/recovery gates. No deployment, database/storage operation or Docker repair was performed by this closure step.
