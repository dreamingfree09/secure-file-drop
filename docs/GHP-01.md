# GHP-01 — Aether–Codex repository collaboration

Adapted for Secure File Drop from the existing Piqniq GHP-01 method on 2026-10-15. This protocol transfers collaboration and handover rules only; Piqniq runtime instructions do not apply. README is the navigation entry; [CONTINUITY.md](CONTINUITY.md) records current accepted state, active work and recovery. Read [AGENTS.md](../AGENTS.md) and existing specification, contributor, architecture and deployment documents.

## Authority and ownership

The Owner retains scope, production, real-data and irreversible-risk decisions. Under SFD-OWNER-AUTONOMY-20261018, Codex owns bounded engineering implementation and execution; Aether owns independent technical review and acceptance recommendations. Git identifies received content, not approval. Record significant unresolved design questions before dependent implementation; report conflicting authority or scope. Handovers cannot expand permissions.

Use a unique work ID, explicit producer/recipient and one producer-owned branch. One writer owns each file; concurrent producers propose canonical changes in their own branches or handovers. Do not edit another producer's record. Preserve accepted evidence, dated history and roadmap ideas. Review intervening target changes before integration. No new agent, orchestrator or service is introduced by this method.

## Required handover

A complete PR description is the producer handover; do not duplicate it in another report. Include:

| Field | Required content |
| --- | --- |
| Identity | Work ID, assignment, producer, recipient and branch owner |
| Revisions | Exact base, received revision if applicable, owned branch, final published full SHA and revision-pinned link |
| File scope | Complete changed paths, behavior, relevant exclusions |
| Approval | Who authorised which scope; approved/pending/rejected/superseded decisions |
| Validation | Actual commands and passed/failed/skipped/blocked outcomes; distinguish unit, integration, browser, container and vulnerability evidence |
| Evidence | Accessible references, local-only evidence, verifier and exact revision; distinguish producer claims from independent review |
| Limitations | Known failures, unverified behavior, blockers and decisions |
| Git and recovery | Working tree, branch publication, preserved local work and recovery status |
| Completion | Active/completed/blocked and remaining work |
| Acknowledgement | Recipient, exact reviewed SHA and pending/accepted/rejected/superseded |
| Integration | Pending review/approved/integrated/rejected; target and resulting SHA when known |
| Next action | Exact next authorised step and decisions for Owner/Aether |

A commit cannot contain its own final SHA. Put the base in repository records; after committing, put the exact final SHA and pinned link in the PR. Do not rewrite history to add a self-referential SHA.

## Publication and independent review

Publish normal scoped commits to the owned branch and one coherent PR. Read back the remote branch SHA, complete PR handover, changed-file scope and published evidence links before reporting completion. Publication failures are blockers, not conversation-only completion. Intermediate commits need no separate handover.

Codex publishes revision-specific handovers and review requests directly through GitHub for Aether's independent review, without routine Owner relay. Aether verifies identity, scope, approvals, exact base/received/final revisions, ancestry/parents, full diff, secrets and relevant checks, then records revision-specific acknowledgement in its own record or PR response. A moving branch name is not a verification identity.

Completion, passing validation, recipient acknowledgement, design acceptance and integration are separate states. Publication alone does not authorise master integration. The standing Owner delegation permits normal dependency-ordered integration only after Aether accepts the exact revisions and all mandatory gates pass; preserve ordered merge parents, exact trees and all worker branches. Never treat producer self-review as Aether acceptance. No force pushes, history rewrites, branch deletion, changes to other producers' branches or unreviewed master writes. Existing sandbox and execution boundaries remain applicable; normal commits/pushes already authorised by a work package need no repeated approval.


## Standing Owner delegation — 2026-10-18

SFD-OWNER-AUTONOMY-20261018 applies exclusively to Secure File Drop. Owner Konstantinos Aidinidis delegates investigation, design, bounded corrections, dependency/test/workflow updates, synthetic integration/recovery, local configuration, normal worker commits/PRs, evidence/documentation and governed integrations. Codex should correct failures, publish revision-specific evidence to Aether through GitHub and continue approved objectives without routine approval loops. Aether may independently accept/reject evidence and direct corrections; no producer self-acceptance. Refresh live refs and review intervening changes before action.

Interrupt the Owner only for secure secret input, unavailable interactive access, work beyond scope, material risk to existing real data/credentials/backups/unrelated systems, production/public exposure, irreversible/destructive action, or an external service/payment/new trust boundary lacking authority. Prepare all independent work first; give one exact minimal interaction and continuation point. Resume dependent work when that action is completed in the supported session; do not imply persistent background agent execution.

The immediate sequence is local TLS/CRL correction, actual Windows acceptance, then dependency-ordered repository close-out. A mandatory unresolved gate keeps its affected PR unmerged; continue independent safe work. The [TLS runbook](LOCAL_TLS.md) records the secure signing requirement and actual unexecuted gates. Preserve the original no-origin Windows checkout, WSL feature branch, .qodo and all unrelated projects. Do not rewrite history, force-push, discard work or delete branches. DEVLOG is strictly append-only.

Standing delegation does not authorise public production deployment, real-data migration/destructive data operations, unreviewed production storage adoption, security weakening, administrative exposure, unapproved paid services or unrelated-project changes. Retain storage maintenance/security ownership, backend-storage TLS, capacity, rotation, topology, historical reconciliation, combined cross-host recovery and public validation gates.
