# PKI completion preparation — synthetic and isolated only

Work ID SFD-PKI-IMPLEMENTATION-COMPLETION-PREP-20261101. Producer Codex; independent recipient Aether. Base/accepted PR42 revision36685fc23e401a6ef3f549541b602df5f9e608a0. This continuation prepares reviewable source and actual disposable recovery/Windows verification evidence. It does not authorise or perform real SFD issuer/trust/publication/adoption actions.

[Aether6556054845](https://github.com/dreamingfree09/secure-file-drop/pull/42#issuecomment-6556054845), author dreamingfree09/94070428, created/updated2026-11-01T09:22:05Z, exact UTF-8 body SHA25683c4d3a6ccbd559b214fe61b5e2acae97ad3d4d269016d92794fe759c54560dd, accepts PR42 as fixture-only and authorises bounded continuation. The accepted source manifest c375f27f033b529f2334fe4e9adc4203e2b4535ab8e00e027c66c600192620b9,24 referenced files and10 checks were verified at that base. Historical manifests/results must be read at their declared Git revision; they do not describe changed continuation files.

## Complete fixture archive and server-key recovery

[Implementation directory](../tools/pki-transition-implementation/README.md) contains the source; [completion evidence](../tools/pki-transition-implementation/completion-verification.json) and [source manifest](../tools/pki-transition-implementation/completion-manifest.json) bind this revision. Final commit identity belongs in the PR description.

archive_recovery.py replaces the seven-core-file limitation with a complete bounded fixture-format snapshot:

- encrypted CA PKCS8, public root, issuer configuration, index/attributes, serial/CRL counters and retained metadata preimages;
- every issued-certificate archive reconciled one-for-one to ledger serial/expiry/issuer, with actual RSA/SHA256 public-signature verification;
- all fixture leaf certificates/CSRs, signed CRL DER/PEM history, source/used/started operation receipts and recovery receipts;
- every known synthetic runtime server key, separately encrypted into PKCS8. No plaintext server key is present in the archive.

The CA unlock credential and archive/server-wrap credential are separate fixture vault values. CBC ciphertext is not described as intrinsically authenticated: each immutable object has hash/length binding, the canonical inventory is authenticated with HMAC-SHA256, and a domain-separated PBKDF2 key has a random128-bit salt and the reviewed work factor. The trusted expected manifest identity is supplied separately. Header/schema/count/path/size bounds and constant-time MAC comparison apply before any restore target is written.

Restore authenticates all objects before writing a new empty, owner-restricted fixture directory. It preserves original ledger states and consumed receipts, reopens the encrypted CA with the recovered fixture credential, decrypts server backups only into protected disposable runtime files and matches their public keys to issued certificates. It rechecks archived CRL numbers below the preserved next counter. A separately minted receipt-bound recovery lease is required before the restored issuer can sign; the new CRL uses exactly the retained next number and advances it. It never resets a ledger, replaces an existing backup or silently reruns a consumed attempt.

Thirteen tests cover complete inventory, two disjoint copy locations, all four server keys, CA/server identity, forward signing, wrong credentials, ciphertext/header/manifest tamper, missing object, partial restore and no-overwrite behavior. The two copy locations are separate Temp directories, not proof of physically independent recovery media. Fixture archive limits are256 entries/8MiB; a real retention/generation policy must stop or be reviewed when those bounds are reached, never silently truncate.

No actual Owner password manager, backup medium or original SFD archive is opened. MemoryFixtureVault is deliberately not asserted to be an actual escrow. Before real creation, the Owner must select accessible independent credential records and two physical key/ledger locations, and an approved recovered-credential restore must be verified. The readiness model refuses to infer that fact from these tests and never grants real authority.

## Local masked input, private transport and consent

SecretEntryFixture.cs is an active, masked two-entry WinForms candidate rather than the old disabled placeholder. The fixture build explicitly warns not to enter real secrets and cannot enable a real operation. No Owner window was displayed or real credential requested.

Controls require matching32–128-character entries and an initially unchecked phase-specific consent checkbox; clipboard shortcuts are disabled. A cancel action and expiry clear controls. Passwords never belong in arguments, environment, a file, stdout, stderr or the public audit record. A private pair of inherited anonymous OS pipes carries bounded messages; only the selected handles are inherited. Parent PID/session and current SID hash are verified. A newly marked Temp context has an Owner/SYSTEM-only protected ACL; its request is consumed with exclusive creation before input. Cancellation/completion cannot replay the same lease.

Ten native tests exercise masked controls, mismatch/consent gating, secret transport without stdout leakage, expiry/user/parent/real-flag/scope rejection, cancellation and replay. Tests use synthetic values and unshown controls, not actual human acceptance. Managed TextBox strings and a standard Windows desktop do not offer protection from a compromised same-user process, debugger or malicious local software; byte buffers/control values are cleared but complete managed-memory erasure is not claimed. Real Owner interface availability, accessibility/vault auto-type and operational launch require exact review and separate authority; no input prompt is launched here.

approvals.py renders four separate future boundaries. Creation has UNKNOWN/null DER/SPKI/thumbprint and binds namespace/profile, custody/archive/input/runtime evidence and KDF. CurrentUser trust requires actual public identities/SID/store baseline and isolated default-TLS evidence; it discloses effects on other applications in that user. Selection binds actual leaf/CDP/proxy/trust/F196/traffic/adoption evidence. Retirement binds exact old identity, replacement/consumer/store closure and a deadline. No blanket approval, LocalMachine change or real-action execution is enabled.

## Publication process-death recovery and monitoring

publisher_crash_worker.py carries public fixture data only and blocks at explicit after-bundle, after-highwater and after-pointer checkpoints. The tests verify the exact checkpoint PID against their own Popen child, terminate only that child, and retain its files. No name-based process killing, Docker service action or key operation occurs.

Six tests prove old verified selection before commit,503 after an interrupted highwater/pointer transition, complete binding after pointer commit, wrong counter/bundle/corruption rejection and no rollback to an older still-valid CRL. publisher_recovery.py requires a fresh source-bound synthetic lease, exact reviewed bundle hash/current highwater, signature/identity/currentness/margin and an exclusive lock. It can only reconcile equal/forward state; an uncommitted candidate requires an explicit additional fixture permission. A recovery marker is exclusive and cannot be reused. Success requires DER readback.

These are actual abrupt process-death tests. They are not hardware power-loss, filesystem rollback or reboot tests. NTFS/power-failure flush ordering, operator custody, deployment ACLs and unattended service reliability remain unverified. Do not restore an old highwater/pointer or reuse a stale controller to make availability appear successful.

monitor_fixture reports current/renew-due/warning/critical/STOP without signing or restarting anything. readiness.py produces only a disabled, least-privilege, interactive-token synthetic monitor-task proposal; no task is registered and no password stored. Before any live publisher, independently review the actual per-user startup executable/ACL/path, sign/publish separation, reboot/login behavior, alert delivery and repair approval. Signing remains a deliberate separate local interaction, never auto-unlocked by the monitor.

## Positive Windows validation without an Owner trust installation

MemoryChainFixture.cs creates a nondefault native chain engine with exclusive application-memory anchors and an empty exclusive peer store. All explicit store-provider calls are MEMORY; no application call opens/enumerates an Owner system certificate store or adds/removes an installed trust entry. It supplies synthetic public objects to the engine and requires:

- leaf/chain revocation checks excluding only the trust anchor;
- cache-only/no-AIA/no-root-auto-update flags, no clock override or ignore flags;
- serverAuth usage and the normal SSL chain policy with exact hostname;
- zero chain error AND zero SSL policy error.

Twelve tests include a positive trusted chain/current CRL and rejection of revoked/expired leaf, absent/expired/corrupt/wrong-AKI/wrong-issuer CRL, foreign anchor, wrong hostname and client-only EKU. Explicit public HTTP DER readback is tested separately. A warm in-memory valid CRL can remain valid while a publisher is unavailable; that is not evidence of fresh network retrieval. Native-chain network CRL retrieval is disabled; no global cache is cleared.

The initial fully restricted engine configuration was rejected with E_INVALIDARG and all12 cases failed at API setup. The corrected documented exclusive-anchor configuration retains required revocation/SSL/cache-only validation. Windows chain internals are not instrumented: the source proves its explicit provider calls and exclusive trust decisions, not absence of every OS internal read of public metadata. No original SFD certificate/key is supplied.

This is positive Windows CryptoAPI certificate/SSL-policy validation. It is not a Schannel TLS handshake, stock Framework4.8 HttpClient/SslStream positive, or actual F196/backend acceptance. Those are separate gates. The prior PR42 default-client untrusted-root tests remain dated evidence and were not rerun against the Owner stores in this phase.

The CI Windows job receives only an allowlisted public certificate/CRL packet, verifies every byte length/hash, and adds roots only to process memory. No private key, archive or password is uploaded. Local mode uses a matching ephemeral fixture CDP; cross-job packet mode uses pre-generated CDP metadata and separately verifies publisher bytes, so no end-to-end native CDP-fetch claim is made.

## Separately reviewable standard Windows TLS/F196 proposal

No synthetic root may be installed in the Owner's profile to obtain a passing test. The next exact proposal must identify a disposable Windows VM/hosted job, isolated profile and source/runner/image identity, with no Owner directories/disks mounted, credentials inherited or existing CA data supplied. Before any fixture trust write, obtain the distinct review/authority required by Aether6556054845 for that isolated scope. Current workflows perform no such write.

The future test procedure is explicit:

1. Generate a new synthetic issuer and unique leaf/CDP for that VM/case; bind actual public DER/SPKI/thumbprints, package/source/probe and VM/profile identity. Keep keys in that disposable environment and keep the entire profile away from the Owner.
2. Verify pretrust rejection with normal Schannel/Framework4.8. Under the separately accepted fixture-trust scope only, add precisely that synthetic public root to the disposable profile CurrentUser/Root and verify the exact diff. No LocalMachine or real SFD anchor.
3. Run real TLS success with normal Schannel and Framework4.8 HttpClient/SslStream revocation enabled, OS defaults, no custom callback or permissive flags. Verify exact current public CRL bytes and served leaf. Repeat with revoked/expired/unknown-root/invalid/absent/unavailable CRL cases; use fresh disposable issuer/profile instances to separate cache behavior, not a global cache flush or clock change.
4. Copy the unchanged reviewed F196 seven-file package into that VM, verify its provenance/hashes and test its actual login/identity/readiness/file protocol against an isolated synthetic backend with disposable accounts/files. This is still not real backend adoption or Owner visual acceptance.
5. Record every executed/skipped/failed case, exact trust-add/removal receipt and fixture custody. Retire only that fixture root/profile under its approved lifecycle. Retain evidence; never remove/reinstall the Owner's old root or claim an expired CRL is a fallback.

This source/test proposal is ready for review; actual standard-client positive execution remains unexecuted because the current phase forbids trust installation. It must not be inferred from the memory-engine result.

## Runtime, KDF and evidence qualification

Observed fixture OpenSSL/libcrypto/libssl are the exact Ubuntu3.0.13-0ubuntu3.16 vendor binaries already pinned. [Ubuntu24.04 standard security support runs through2029](https://ubuntu.com/about/release-cycle); the [vendor package](https://packages.ubuntu.com/noble/amd64/openssl), [USN8847-1](https://ubuntu.com/security/notices/USN-8847-1) and [CVE2026-84782 status](https://ubuntu.com/security/CVE-2026-84782) identify the vendor fix level. Upstream3.0 EOL is not ignored and the banner alone is not a vulnerability verdict. The selected default provider and exact executable/library bytes are recorded; no software is installed/upgraded on the Owner machine.

A random/public-input PBKDF2-HMAC-SHA256 calibration measured median163.084ms at600000 iterations and selected1000000 for a250ms target, bounded600000–3000000. Actual fixture root PKCS8 and archive/server encryption use the pinned candidate work factor and the existing DER test checks it. This is a derivation benchmark, not a password hash disclosure or exact end-to-end PKCS8/provider performance clearance. Repeat/calibrate on the final approved deployment baseline before real creation.

Public OS metadata reports display26H2/build26300.9818 and Framework release534296; [Microsoft's release table](https://learn.microsoft.com/en-us/windows/release-health/windows11-release-information) maps that build to Windows11 26H2. The registry's historical Windows10 product label is not used to declare the system unsupported. Exact native probe/source, compiler/Crypt32 and fixture host pins remain recorded. Version correspondence is not a whole-OS zero-CVE clearance. Python3.13.1 is a test coordinator/HTTP fixture runtime, not an approved long-running production service; its old TLS library is not used for the new positive memory test. No package updates, certificate checks against real SFD material or security-setting changes occur.

Final source-bound local results, hosted CI identities and every correction are in the GHP-01 PR and [completion receipt](../tools/pki-transition-implementation/completion-verification.json). Source maps are captured before/after tests; no source drift may pass. Earlier failures include the missing guarded PKCS8 role, used-archive exception mismatch, rejected engine configuration and an isolated receipt-import path error; each was corrected without weakening cryptographic/temporal checks. A portable preflight also ran before its public packet producer completed (zero tests/one setup error); waiting for the dependency and rerunning the affected12 cases passed. No failure is converted into a skip.

## Preservation and decision

Protected application master61195434bee5f9221a6f55469c719cb360504ea9 and PR38–42 histories/heads remain unchanged/unmerged. All changes use a separate owned dependent worker. Original no-origin local work, WSL feature checkout, F196, .qodo and unrelated projects are untouched. No real CA/key/certificate-store inventory, original backup/disk/database/object/credential access, Docker start, stale-controller execution, service selection or merge is performed. Preservation is by scope and ref/metadata checks, not a new inspection of forbidden real artifacts.

Actual Owner vault/physical two-location custody, an approved real secret-entry launch, actual replacement-CA creation, real trust/publication/endpoint/retirement, standard Windows/F196 positives, durable startup/power-loss recovery, separate disk/ext4/restart/backups/schema/adoption and physical Quick Send remain gated. No public-production readiness. Aether must review the exact continuation source/manifest/tests; genuine phase-specific Owner authority is still required for all real changes. No Owner command or password is requested for this review.
