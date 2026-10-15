# GHP-01 — Aether–Codex repository collaboration

Adapted for Secure File Drop from the existing Piqniq GHP-01 method on 2026-10-15. This protocol transfers collaboration and handover rules only; Piqniq runtime instructions do not apply. README is the navigation entry; [CONTINUITY.md](CONTINUITY.md) records current accepted state, active work and recovery. Read [AGENTS.md](../AGENTS.md) and existing specification, contributor, architecture and deployment documents.

## Authority and ownership

The Owner and Aether retain significant product, architecture and strategic decisions. Codex owns implementation and routine verification within authorised work. Git identifies received content, not approval. Record significant unresolved design questions before dependent implementation; report conflicting authority or scope. Handovers cannot expand permissions.

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

The Owner uses CODEX DONE to trigger Aether's independent GitHub review in “GitHub workflow implementation”. Aether verifies identity, scope, approvals, exact base/received/final revisions, ancestry/parents, full diff, secrets and relevant checks, then records revision-specific acknowledgement in its own record or PR response. A moving branch name is not a verification identity.

Completion, passing validation, recipient acknowledgement, design acceptance and integration are separate states. Publication does not authorise master integration. No force pushes, history rewrites, branch deletion, changes to other producers' branches or unreviewed master writes. Existing sandbox and execution boundaries remain applicable; normal commits/pushes already authorised by a work package need no repeated approval.
