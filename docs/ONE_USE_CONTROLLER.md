# Reviewed one-use local controller

Work ID SFD-PR37-INTEGRATION-CONTROLLER-20261026; producer/branch owner Codex; independent reviewer Aether. The Owner authorises bounded local recovery/adoption and exactly one ordinary acceptance account. This package starts at actual merged master `61195434bee5f9221a6f55469c719cb360504ea9`. It is a new privileged implementation requiring its own exact-revision review before live execution.

## Accepted integration and separate review gate

[PR #37](https://github.com/dreamingfree09/secure-file-drop/pull/37) was accepted at `47be19fa5ed0ffddd9ffea71fec84719f2a2b5cc` by [Aether](https://github.com/dreamingfree09/secure-file-drop/pull/37#issuecomment-6427391643), then normally merged under the [execution instruction](https://github.com/dreamingfree09/secure-file-drop/pull/37#issuecomment-6427498111). Ordered parents of actual master are `ae45cb816f9ec05e334e4620a4b428cebe631901`, then `47be19fa5ed0ffddd9ffea71fec84719f2a2b5cc`; its tree `0424c1d4cd375c21801e33775f28f73ab06b151b` equals the accepted proposal. No original history or worker branch was removed.

Actual-master [root CI](https://github.com/dreamingfree09/secure-file-drop/actions/runs/40403320828), [operator CI](https://github.com/dreamingfree09/secure-file-drop/actions/runs/40403320822) and [Windows CI](https://github.com/dreamingfree09/secure-file-drop/actions/runs/40403397291) passed. Both PostgreSQL layout artifacts report 94 backend checks, 27 browser checks and 244 Go test events, zero skips/failures, preservation and owned cleanup true. Windows additionally reports 17 portable checks. These results apply to the actual merge, not merely its proposal.

This PR does not request PR #37's review again. Aether must review the entire new controller, broker, driver adaptation, storage probe, native launch and failure behavior. A review of the prior Go operator or GUI alone cannot authorise this controller.

## Source and artifact binding

The source bundle is the canonical JSON SHA-256 of the relative-path-to-SHA-256 map enumerated by runtime.CODE_PATHS. The private context binds that bundle, exact published controller revision, reproducibly built probe, current master, fixed earlier acceptance bodies and a new exact controller acceptance comment. The comment must contain exactly one JSON object of kind SFD_CONTROLLER_ACCEPTANCE_V1 in a JSON fence, with accepted=true, revision, bundle_sha256, probe_sha256, master, scope_plan_sha256 and operator_review_comment=6427391643. The revision and hashes must be those Aether actually reviewed; the producer cannot fill in an acceptance on Aether's behalf.

The runtime retrieves the comment and exact Git blobs, verifies the comment body hash and source bytes, checks all current controller checks, rechecks actual-master ancestry and its three successful CI identities. A mutable branch, arbitrary URL or old operator approval is insufficient. Master movement or changed review requires reconciliation. The controller package may remain on its worker branch while reviewed; merging it first would change its deliberately pinned adoption master.

The original PR #37 driver remains untouched. The separately owned staged_driver.py changes only MASTER, decision and new_json: actual-master identity; new exact controller/owner/lease authority and accurate post-Submit credential-request flags; atomic exclusive state publication. Registration, controlled-mailbox verification, quota, audit and F196 logic stay identical. An AST regression enforces that boundary. The original driver hash and retained binaries stay independently checked.

Retained artifacts: Go quota operator 43eec7c68d480e4ebc3dfd7063fe37ecd3f0f6ba0e6917003cc5e3f5604944bc; native form 91bec8d81a9b3918c0b0cfda09d0182cc0b7b9329541a680c09c5b609c79300d; transport 11a12387243855c29064e7cda9d029161c495cc73e05d435b4dbfa1df829726a; F196 f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca. The complete distributed preview and original copies remain preserved.

## Private preparation and Owner interaction

Codex prepares a new private context with prepare_context.py. It defaults to disabled. Enabling still validates remote source, CI and independent acceptance; main.py repeats those checks. No private context, credentials, original environment, actual object keys or recovery source files are published.

The one-use session binds an Owner-owned sealed source directory, nonce, expiry, exact visible GUI parent and global work lease. No authority denial may abort another session. Before the form opens, require exact live service/config/image identities, three-loss scope, original custody, no new archive or running backup job, at least 5,400 seconds of seven-day scheduler margin, sampled activity, private storage, bounded object hashes, fresh isolated PostgreSQL restore and strict Windows TLS. Code and tests never infer Owner IDLE from inactivity.

Codex opens the verified native form. The Owner confirms IDLE and enters only the existing administrator password into its masked local field. After Submit, the broker refreshes technical receipts and holds that password only in redirected private child pipes and process memory. The model, terminal, logs, command arguments, environment and context files never receive it. The broker holds the generated ordinary-account password until final preservation succeeds and the controller approves the matching nonce/master. Credentials then appear only in the verified local GUI. Clipboard use, if the Owner selects it, is a local Owner action and is not read by Codex.

## Live transaction and failure boundaries

1. Recheck exact review, source, visible parent, nonce, expiry, original service identities, activity and scheduler. Restore only the three accepted missing archives. Each publication is exclusive, checksum/gzip/mtime verified and fsynced; the group is not atomic. Retain any completed subset and stop on failure. Do not overwrite existing paths or conceal the historical deletions.
2. Recheck again, cleanly stop only the incumbent backend with 25 seconds, then recreate only backend using --no-deps --no-build --pull never. Permit only the reviewed successor image, 25-second stop timeout and retention 7 to 30 days. Daily backups stay enabled and file cleanup stays disabled. Preserve exact environment, mounts and the other five service identities/configuration.
3. Require clean additive schema12, original rows/columns/objects, all eleven original/custody-union archives plus the legitimate new startup backup, original archive mtimes and strict TLS. Never downgrade metadata or restart a rejected fallback.
4. Reserve the global one-account work ledger before provision. Register normally, consume only the exact controlled alias's new verification message, genuinely verify the account, apply 1 MiB quota with the accepted quota-only operator, and execute all 27 real F196 acceptance assertions. Do not request/reset an existing registered-user password.
5. Verify ordinary role, active/verified state, quota, work audit, exactly one new account and one 512-byte file, original data/object preservation and strict HTTPS. Only then release the private local credential display.

Any fourth archive loss, new incumbent backup cycle, changed baseline or failed gate stops execution. An account failure retains its exact private identity and stops: no automatic second account, password reset or deletion. After migration, a failure stops only a verified candidate backend and keeps data/archives intact for investigation; there is no old-image or schema rollback. A post-release error records possible credential disclosure conservatively. The GUI expires after its bounded local display period.

Successor: sha256:084afb9ff9ecd6244f24e795e86f35cd8d328130915bc44aa0d96ebfe0e356f2. Isolated recovery: sha256:4c60abc87c6d5b2b9d56f95b3d3255f1b5900695de7554ab2ca4cfe5dde47c07. Scope plan SHA-256: 0ffabb47d13d9aaa46508b7c88192307be007d18628b20dc9674a60cc58c4573. Approved retention overlay: e083dd03d918127b8dda6d73c9cc8171777b9eb2d2031112c48b5b56d4e70dfd. No superseded image is an operational rollback.

## Executed evidence and remaining gates

Before publication, 26 Python tests passed, including authority denial without side effects, one-use reservation, partial archive retention, review binding, post-Owner recheck, exact Docker update arguments, private handoff withholding and driver adaptation. Nine Go pass events under race checking, vet and two-root deterministic builds passed in owned isolated MinIO fixtures. The actual production probe ran twice inside the accepted successor runtime on a synthetic 512-byte object; snapshots and policy hashes matched. Probe SHA-256: bd1a6f1403231edf5fcb9fd1b63f7ae8cc7ad6166909d6fcd549bb5d9c5b4702. Configured nested-module lint reported zero findings; govulncheck reported no reachable/imported-package vulnerabilities, with unused module-only GO-2026-5932 retained.

One first isolated fixture failed its loopback-port prerequisite before storage writes; it was stopped and retained, not counted as a pass. Corrected uniquely owned fixtures passed and only their own resources were removed. No real backup was restored, backend changed, account created or password form opened during this package's preparation. The Windows script was syntax-parsed; live execution remains gated.

Before live use, require this controller's exact independent review and successful published checks, then repeat all fresh live gates. Current CRL renewal deadline is 2026-10-28T18:10:29Z; no signing, trust-store or revocation bypass is included. Actual Owner-operated Quick Send, Explorer, clipboard, visual behavior and file-integrity acceptance remain unexecuted.

Retain archived-MinIO isolation/maintenance, existing exposure and backend-storage TLS, capacity, credential rotation, production topology, historical ownership reconciliation, cross-host recovery and deployment gates. This is bounded local development acceptance, not public production readiness.
