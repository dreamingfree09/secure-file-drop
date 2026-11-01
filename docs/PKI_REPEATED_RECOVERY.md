# Repeated issuer recovery correction — synthetic only

Work ID SFD-PKI-REPEATED-RECOVERY-CORRECTION-20261101. Existing PR43 branch: codex/sfd-pki-completion-preparation-20261101. Received revision fd38e852bc579f0083bf2241b791b7ff634e5bbf; accepted PR42 design/implementation-preparation base36685fc23e401a6ef3f549541b602df5f9e608a0.

[Aether6557257514](https://github.com/dreamingfree09/secure-file-drop/pull/43#issuecomment-6557257514), author dreamingfree09/94070428, created/updated2026-11-01T11:21:50Z; UTF-8 body SHA256 **3be2056c1ee1eaf23f5a163247826be68a43bb7641bf6a42dedaab767d89d061**, rejects fd38e852 as a complete recoverable issuer while retaining its bounded86-case evidence. It authorises this correction and synthetic regressions only. Independent acceptance of the corrected revision is pending.

## Confirmed defect and correction

A newly created disposable fixture reproduced the received restore → snapshot → restore failure: exclusive creation collided at the second target's restore.started. The partial fixture and exact failure receipt are retained privately. The first-generation13 archive tests missed this lifecycle. The old86 local cases and12 successful hosted jobs remain historical evidence, not acceptance of repeated recovery.

The correction separates two types of files:

| Location | Meaning |
| --- | --- |
| history/v1/source-fixture/artifact | Immutable-by-interface, authenticated, versioned evidence from that source generation: original fixture marker, source/recovery contexts and consumed markers, creation/restore/signing events, verification receipts, CRL DER/PEM artifacts, diagnostics and ledger preimages |
| New target root | Fresh exclusive restore.started, current restore.verified.json, and separately activated recovery-context.json/.used; restored encrypted issuer/current ledger, issued certificate archive, public leaf identities and recovered runtime server keys |

Old source/recovery contexts never occupy an active target path. Old recovered CRLs and signing locks also remain history, allowing a fresh target to create its own recovered.started and recovered.crl.* once. Preimages are preserved before later OpenSSL operations can replace active .old outputs.

recovery_history.py validates exact versioned paths, fixture identity, contiguous generation provenance, event hashes/lengths, issuer DER/SPKI, serial/CRL monotonicity, prior receipt hashes and complete prior-history inventory bindings. Existing historical bytes are copied unchanged. New generation provenance is added separately. History files/directories are read-only and all writer operations use exclusive creation; this is not an OS-immutable or same-user-compromise guarantee.

The archive profile is **SFD_SYNTHETIC_ARCHIVE_V3**, with a separate V3 authentication domain and the existing pinned PBKDF2/HMAC construction. Authentication, object/path/bounds and historical semantic checks precede target writes. Prior V2 fixtures are not silently upgraded or reinterpreted; their original archives remain preserved. There is no real archive migration entry point. The256-object/8MiB bound stops rather than truncating history.

## Current restore and signing authority

Each new restore binds its random operation ID, target fixture, source fixture, exact authenticated source manifest, generation, root DER/SPKI, historical inventory/prior receipt and exact next serial/CRL counters. Issued certificates and retained CRLs receive actual RSA/SHA256 signature verification; CRL DER/PEM pairs must match. Runtime CA/four server-key identities and current ledger are verified before the current receipt is exclusively written.

A fresh source-bound target-specific synthetic lease is required for activation. Its consumed marker binds the current receipt SHA256 and operation. A restored target cannot fall back to source-context authority. Missing/old/replayed/tampered authority, consumed markers, receipt changes and counter regressions stop before signing; the same operation cannot overwrite existing outputs. Every generation signs from exactly its retained next CRL number and advances it. No counter/status/record is reset. Fresh contexts still expressly deny real issuer, trust and implementation-acceptance authority.

The stronger regression performs restore → activate → forward-sign → snapshot → restore → activate → forward-sign. It checks both fresh leases/operations, all prior history, current root/four server public identities, issued ledger/serial preservation, all retained public signatures, CRL number advancement and absence of old active paths. A second regression exercises restore → snapshot → restore without activation.

## Regression evidence and limits

[Source-bound correction receipt](../tools/pki-transition-implementation/repeated-recovery-verification.json), [archive results including two-generation receipts](../tools/pki-transition-implementation/archive-results.json) and [complete source manifest](../tools/pki-transition-implementation/completion-manifest.json) bind the actual runs. Final commit, manifest hash, complete changed-file inventory, all exact-head hosted CI identities and Aether acknowledgement belong in the PR43 GHP-01 handover.

The original13 archive cases remain, including credential, MAC/KDF, manifest/object tamper, missing certificate, interruption and no-overwrite checks; the original source-context preservation assertion now checks the historical namespace. Twelve new cases cover both generation chains, replayed source/prior recovery contexts, tampered history, authenticated wrong history identity/generation/counter, a corrupt historical CRL signature even with a recomputed fixture MAC, interrupted second restore, reserved active paths, consumed activation markers, changed active receipt and resealed counter downgrade. All other current security groups are rerun without removing cases.

All execution uses newly marked disposable identities. Native probes are unchanged, hash-verified Framework4.8 executables. Windows positive evidence remains exclusive-memory trust with required revocation and SSL policy. It does not establish ordinary Schannel/Framework/F196 positive TLS or native CDP retrieval. Historical default-client tests requiring Owner certificate-store inventory are not authorised for rerun in this phase; the separately scoped disposable VM/profile proposal remains a gate.

No real CA/key/certificate-store access, real Owner prompt, trusted-root installation, real root/leaf/CRL, live publication, local Docker, original backup/disk operation, stale controller or protected merge is performed. The old CA/expired CRL/counter/signing evidence is preserved by nonaccess. Source/worktree/ref checks preserve the original Windows local work, clean producer and WSL feature branches, application master and existing PR38–43 history. Actual vault/physical custody, real phase consent, standard Windows/F196, startup/power-loss, Docker/archive recovery/backend adoption and Owner visual acceptance remain separate gates. The automated finding stays unresolved pending fresh Aether exact-revision review.
