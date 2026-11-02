# Clean installation and end-to-end acceptance proposal

Work ID **SFD-CLEAN-INSTALL-ACCEPTANCE-PLAN-20261102**. Producer/branch owner Codex; independent reviewer Aether; Owner Konstantinos Aidinidis. **Plan ready for review; installation and user acceptance have not run.** No application, dependency, workflow or accepted PKI source is changed.

## Decision and current constraints

Use a **fresh Linux backend VM and a clean Windows test VM on an independently approved test host**, connected only to a private virtual switch during testing. The host must have no original SFD disks, Docker Desktop data, recovery images or engine sockets attached. This avoids the original engine's restart policies and backup scheduler altogether. It does not repair or start the old installation.

The Owner PC has63.9 GiB RAM/16 logical processors, but the observed C: free space fell from8.1 to7.8 GiB and E: has31.3 GiB. Full Hyper-V and its management module are disabled/unavailable; VirtualMachinePlatform/WSL and Windows Sandbox are enabled. Docker Desktop4.55.0 is installed, its service is Manual/Stopped, and no Docker engine process was observed. No VM, WSL distribution or Docker API was launched/contacted. Hypervisor-present CPU flags do not establish that SLAT is unsupported.

The proposed small-lab budget is8 GiB RAM per guest,32 GiB Linux and80 GiB Windows virtual disks,16 GiB input staging,16 GiB checkpoints,4 GiB fixture evidence/backups and12 GiB reserve: **160 GiB free lab storage**, plus20 GiB free on the chosen host's OS volume. These are planning limits, not vendor minimums. The current PC fails these disk gates. All five inventoried historical package folders together occupy **638209 bytes**, about0.61 MiB: deleting them cannot close this gap.

A separately available hypervisor/test host and storage are therefore the first unresolved execution decision. No such target has been verified available. Adding an external drive alone does not authorise enabling Hyper-V, rebooting this PC or changing original services. Windows Sandbox is a possible later optimisation, not the selected target: its current C: capacity and exact backend-network isolation have not passed. A renamed Docker context, new Desktop profile or different data-root on the original host is insufficient evidence of independence.

Microsoft distinguishes a private switch, with guest-only communication, from an internal switch that also reaches the host. Use the former and verify actual topology. Disable host drive/folder, clipboard and credential redirection; do not attach the primary host's disks. [Hyper-V network types](https://learn.microsoft.com/windows-server/virtualization/hyper-v/features-terminology), [Windows Sandbox controls](https://learn.microsoft.com/en-us/windows/security/application-security/application-isolation/windows-sandbox/windows-sandbox-configure-using-wsb-file), [Windows11 VM requirements](https://learn.microsoft.com/en-us/windows/whats-new/windows-11-requirements).

## Exact reusable components

[Evidence](../tools/clean-install-plan/evidence.json) records public package hashes, source and runtime candidates, read-only host/package observations and current gates. [Source selection](../tools/clean-install-plan/source-selection.json) binds130 runtime/build/client files. [Plan manifest](../tools/clean-install-plan/manifest.json) binds all proposal records; final plan commit/hash belong in the GHP-01 PR.

| Component | Verified identity and permitted reuse |
| --- | --- |
| Application | master **61195434bee5f9221a6f55469c719cb360504ea9**; all130 selected runtime/build/client files are byte-equal at accepted PKI head1805a108. Build the application from this frozen source, without original .env, Git credentials, binaries or data. |
| Client source | Exact desktop/windows tree **998e690a5239fc2de9bf4e4b5d259c8022e3611a**, equal to reviewed **c320861536914623ea22f28c1d4ea156d4c2132a**. Framework4.8 targeting, OS TLS defaults, revocation checks and strict session verification are retained. |
| Latest verified desktop package | [Run40403397291](https://github.com/dreamingfree09/secure-file-drop/actions/runs/40403397291), artifact12690070973, source61195434. ZIP SHA256 **f38b9f82dda5c0320c17a3d0a04de924c958750f8975be28d3914d0ec8b90c9e**; exe **8f98bd9dca095c827e8ccf38000ae28fecf45df60ec10c9b484ed479748bb28b**; configuration **4d83c0772f4b593d6e16657fb5be7e4478321cb9782ba744c93b90915fb7175b**. All seven files and published checksums/identity verified, including compiled Client.cs checkout bytes. ZIP sealed privately; not extracted, installed or executed. |
| PKI fixture implementation | Accepted **1805a108855817d7b36220a6253684bd6a018fb3**, manifest **19e4e5f5af3e782bf78dfca40930801c60d0e711690db6b230089325110c3eb9**. Reuse source fences, issuer/public CRL validation and V3 archive/history semantics within new disposable namespaces. Do not enable real-transition flags or use the stale PR38 controller. |
| Accepted evidence | [Aether6566129478](https://github.com/dreamingfree09/secure-file-drop/pull/43#issuecomment-6566129478), body SHA256 **31b4221cb4847eefe478d3e63aad1c38bef26bd2c797e28eac35e4753982fa16**. Existing98 fixture tests/12 hosted jobs accepted; finding4435269497 verified resolved. No repeat review/execution of unchanged code is requested by this plan. |

The new desktop binary is not F196: the source is identical, but this package has its own verified build identity. Existing F196 and historical97fc packages remain preserved. All seven distributed files are SecureFileDrop.exe, SecureFileDrop.exe.config, README.txt, LICENSE.txt, SHA256SUMS.txt, windows-identity.json and windows-results.txt. The hosted artifact expires2026-11-09T21:20:09Z; the sealed verified copy preserves these bytes independently of later hosted expiry. It is an unsigned preview. Keep SmartScreen/Defender and normal policy enabled; a policy block is a stop, not permission to bypass or sign with an unapproved certificate.

## Runtime refresh, not silent reuse of historical images

The accepted application's current CI still exercises a historical storage/proxy fixture. That is useful source evidence, not approval of a new supported runtime combination.

Traefik2.11 security support ended2026-09-07. Its current supported candidate is **3.7.17**, source04e8ccf84ada1dcb450cec06554fe27812903134. The historical RustFS c917317a image identifies1.0.2; the current candidate is **1.0.4**, source46337942d55f3a8e6a97b147ca98d9ea8115555d, including the vendor's certificate-trust patch. Both changes require fresh compatibility and advisory checks, not application features. [Traefik support](https://doc.traefik.io/traefik/v2.11/deprecation/releases/), [Traefik release](https://github.com/traefik/traefik/releases/tag/v3.7.17), [RustFS release](https://github.com/rustfs/rustfs/releases/tag/1.0.4), [RustFS advisories](https://github.com/rustfs/rustfs/security/advisories).

Read-only registry metadata resolved the following **linux/amd64 candidate manifests**; no layer was pulled or image run:

| Candidate | Immutable platform manifest |
| --- | --- |
| Go1.26.10-alpine3.24 builder | sha256:d1f3216ef30257c850cc136ba42a359ccd9d3e75f19b2c276e520c0b4ef327ce |
| Alpine3.24 backend base | sha256:c26612752c78b18faacd63c7d50f4592c2e7b0f25e0e20e82bd74e465c57ab0d |
| PostgreSQL16-alpine | sha256:cbf42f0edc946c0bc1b8eeed62403734e1f25dc67a3491455d07a92fdb558b28 |
| Traefik3.7.17 | sha256:c999a937a1bb51ba36c79adf2c9db1d1bb196896b66d524d30ef4945606d565f |
| RustFS1.0.4 | sha256:e287e99762a4a3e7b706526e01f2ff755f67db165e9be9f6b907adc6fb6d6ee1 |

Index/config digests and release metadata are in evidence.json. A label or digest proves identity, not provenance/security: RustFS's published title contains joined metadata rather than separate labels. Verify actual runtime version, available build attestations/SBOM and affected/fixed advisory ranges before admission. Do not reproduce storage signature-bypass experiments.

The backend OCI digest, archive SHA256 and backend/native binary hashes are **not yet known**: no build is authorised here. After execution approval, derive a lab-only build recipe from the exact Dockerfile.backend with only its two FROM inputs locked to these digests; record the recipe hash/deviations, actual APK package versions/hashes, go.sum and binary build metadata. Build/scan/test once in the independent engine, freeze the resulting image/export by digest, and validate an artifact-recovery copy. Do not claim identity with old local successor/recovery images or pull rejected fallbacks.

Choose supported Windows11 GA and Ubuntu24.04 LTS guest media with verified official checksums/licence. Exact ISO/build/Engine package versions remain a gate. Ubuntu24.04 preserves the accepted PKI platform; its OpenSSL executable/library pins must still match a supported, patched vendor baseline. Never downgrade security packages to satisfy old pins. Pin drift requires only the affected toolchain/source update and tests/review, not a ritual review of unchanged PR43. [Ubuntu lifecycle](https://ubuntu.com/about/release-cycle), [PostgreSQL support](https://www.postgresql.org/support/versioning/).

## One staged workflow

### P0 — bind the independent target and approvals

Select an Owner-controlled test host already able to create these guests, legal Windows media/entitlement, exact new storage paths/VM GUIDs, resource budgets and an accessible graphical console. No primary-host reboot, Hyper-V enablement, Docker reinstallation or original-engine start is included.

Independently inspect the selected VM disk/controller list before boot: only newly created OS/data disks, no original/sealed/working/disposable-child VHDX, physical-disk passthrough, host drive map or original backup share. Capture exact new guest/engine identities. The original host remains excluded even if its engine becomes available.

All future engine calls run inside the Linux VM against its exact local Unix socket and expected daemon ID. Require fresh empty storage and no pre-existing lab resources before pull/create. Verify both Docker and containerd stores belong to the new guest disk; data-root alone may not cover containerd. Never use the primary Windows Docker CLI/default context. All lab resources carry the approved unique run ID/owner labels and restart policy **no**. [Docker storage configuration](https://docs.docker.com/engine/daemon/).

### P1 — build a fresh application stack

Use a standalone, explicit lab service specification, not the root docker-compose.yml or a merge with it. The latter has historical MinIO, static container names, restart policies and unrelated monitoring. Generate all credentials inside the new guest, retain them only in protected guest files/UI and never print environment values. No existing account, session, SMTP credential or .env is imported.

| Service | Fresh boundary |
| --- | --- |
| PostgreSQL | New empty cluster/database/volume; current supported16 minor verified; no published port; initialise only the selected fresh schema12 profile. Verify version/dirty state and empty users/files before registration. No original schema migration. |
| RustFS | One fresh private object volume; distinct bootstrap-root and least-privilege application keys, real private-bucket/IAM checks; console and alternate protocols disabled; no public S3 port. No legacy MinIO data copied. |
| Backend | Exact61195434 build, native sfd-hash known vectors; only container-network HTTP8080; new session/download/admin secrets; production-like validation; no host mounts/socket. Current Dockerfile defaults to root: do not falsely claim a non-root image; any lab UID change needs compatibility validation. |
| TLS proxy | Supported Traefik candidate, file provider only, no Docker socket; Linux-guest loopback8443; exact read-only leaf/key/config mounts; request/header/rate controls validated for3.7. |
| Public CRL publisher | Accepted public-bundle validation/anti-rollback semantics on Linux-guest loopback18082; no private key in public tree. Supervised fixture service, not a claimed production Windows service. |
| Verification mail | Reuse tests/auth/smtp/main.go in the backend's own loopback namespace with fresh synthetic AUTH credentials. SMTPHost=localhost uses the existing standard loopback PlainAuth policy; this fixture is not external SMTP/TLS acceptance. Never expose that sink externally. |

No Grafana/Prometheus or production-storage migration is needed for the demonstration. Keep file cleanup disabled, backup retention30 days and schedule24h; use a new lab-only backup volume. Set file limit10485760 and total request envelope11534336 in backend/proxy/client consistently. A manual new-lab backup/restore can be tested; a six-hour demo does not demonstrate a full24h/30-day scheduler cycle. Backend-storage/DB transport security and maintenance remain production gates.

### P2 — establish new guest-only HTTPS

Propose a **supervised, at-most-six-hour disposable demonstration**, subject to Aether/Owner approval of this fixture scope. It is not creation of the real replacement SFD CA. Reuse accepted fixture-only issuer code with unique synthetic identities and loopback SANs. Its existing defaults are a730-day root, about90-day leaf and7-day CRL: the six-hour operational window is separately bounded. The old issuer/key/counter/expired CRL are never accessed.

Create a minimal positive fixture plus separately identified negative canaries. Record actual new public DER/SPKI/thumbprints, leafs, signed CRL numbers/validity, ledger and package/source hashes before trust consent. Use only fresh generated fixture secrets, not an Owner passphrase. The short demo has no continuing signing/custody promise: if durable reuse is wanted, genuine recoverable vault/two physical copies and live input/renewal gates must first be met. Do not quietly promote a fixture into a real installation.

Inside the Windows guest, pinned OpenSSH local forwarding carries **guest localhost:8443** to Linux loopback8443 and **guest127.0.0.1:18082** to its public CDP. Use a fresh restricted forwarding key, strict host-key verification, no agent/X11/host drive forwarding, loopback-only listeners and permitted destinations only. Both origin and CDP resolve in the Windows guest, not the Owner host. This preserves the accepted localhost/IP SAN profile without new certificate features. Bind failures stop; tunnel establishment alone does not prove the destination works. [OpenSSH configuration](https://man.openbsd.org/ssh_config.5).

After exact guest-only consent, add only enumerated fixture public roots to **CurrentUser/Root of the standard test user's SID in the named Windows VM**. Record before/after guest trust differences. No LocalMachine write, Owner-store inventory/change, host trust or old-root retirement. Accepted PR43 real-transition flags stay disabled; the small guest trust/bootstrap package and its target bindings require their own scoped review.

Before accounts or visual testing, require stock Windows Schannel curl and real Framework4.8/F196 success with normal hostname/certificate/revocation validation. Prove actual CDP GET/readback and signed DER identity in a cold guest profile; memory-only roots or a portable .NET10 pass cannot substitute.

Run unknown-root, wrong-host, expired/revoked leaf, invalid/expired CRL and cold-profile unavailable-publisher negatives with fresh case identities/checkpoints. Existing fixture SANs include127.0.0.1, so the old IP-hostname negative must not be blindly reused: use a name outside all SANs, with resolution changed only inside the guest. A separate read-only public canary server may serve deliberately invalid fixture bytes; do not weaken the accepted validating publisher. Warm valid cached CRLs can allow an offline publisher: record that behavior without calling it fresh fetch. No global Owner cache flush, clock change, insecure callback or curl bypass.

Only the Windows guest's pretrust state may be reset for cold-profile tests. Never roll back the Linux issuer ledger/highwater or reuse consumed contexts. A new case uses a new namespace; forward recovery follows accepted V3 rules.

### P3 — install the verified portable client in the clean guest

Copy all seven sealed package files to a new guest folder; verify ZIP/member hashes again and Framework4.8 availability. Confirm settings/session/Explorer paths are absent in that fresh profile. Launch unelevated, keep normal Windows protections, configure https://localhost:8443 and the matching limit. No autostart or host installation.

Show fresh synthetic administrator and ordinary-account credentials only in the private guest interface. Owner supplies no existing credential. Create ordinary accounts through real registration, successful local SMTP delivery and the actual verification route; no SQL verified flag, manual database admin user or authentication shortcut. Verification tokens stay inside the guest fixture process/UI, never public logs/chat.

Explorer opt-in is an explicit guest-only step after endpoint/trust acceptance; verify ownership and exact quoted command under guest HKCU. Host clipboard/drive redirection remains disabled; the real guest clipboard and Explorer are tested. This demonstrates a genuine Windows session, not installation into the Owner's everyday profile. Host-native deployment remains a separate objective requiring a normally trusted endpoint without violating the no-host-trust-change boundary.

### P4 — end-to-end acceptance, with automated and Owner observations separated

| Check | Required actual evidence |
| --- | --- |
| Readiness/TLS | PostgreSQL/storage healthy; public /ready200 through the selected proxy; strict Schannel and Framework success, actual CRL retrieval; negatives fail before credentials. |
| Administrator | Fresh env-admin login and /me200 with correct management role, no database admin record fabricated. Ordinary uploads use registered accounts. |
| Registration/users | Genuine delivered verification and login for two synthetic ordinary accounts; /me UUID/role correct; cross-account list/link creation denied or404. |
| Sessions | Invalid/expired/tampered session rejection, sign-out revocation, account switch isolation; remembered DPAPI session optional within guest only. No cookie/token in evidence. |
| File flow | Local fixture SHA256 equals native backend metadata and actual HTTPS downloaded bytes; empty, small binary and Unicode/space/metacharacter filenames; exact10 MiB and over-limit nonmutation. |
| Expiring links | Real client five-minute link works before expiry and returns410 after actual expiry; a60-second API case can shorten automated waiting but cannot replace the client case. Expiry leaves bytes/metadata preserved; a new valid link can still work. |
| Tray/picker | Owner sees one tray instance, usable menus/status, native Quick Send picker, progress/completion and visible errors. Programmatic menu creation is not a visual pass. |
| Explorer | Owner actually selects a guest fixture and invokes Show more options/Quick Send; spaces/Unicode survive; current-user/session instance forwarding is observed. |
| Clipboard | Owner observes a valid same-origin link, successful download and byte hash; own link clears after two minutes/sign-out/exit if unchanged, unrelated clipboard content survives. Do not log URLs. |
| Interruption | Cancel/response loss retains uncertain identity; reconcile existing transfer, no automatic resend or deletion. |
| Persistence/recovery | Restart only the new backend/guest runtime and verify accounts/fixture hashes; make a new lab backup and restore to a second empty owned database/volume, never over originals/current lab state. |

Agents perform technical checks and preparation. Owner performs only genuine graphical actions, using generated guest credentials. Emit a sanitised result ledger with PASS/FAIL/NOT RUN, actual revisions/images/versions, counts/hashes and actor/guest identity. Redact passwords, email/session/reset/verification tokens and signed URLs, including screenshots. Any failed gate stops dependent acceptance; never rename a skip as a pass.

### P5 — rollback, quarantine and separate cleanup

No original rollback target exists in this workflow: it never adopted the original backend. On failure stop only verified new lab processes/VMs, isolate their private network and preserve all new disks/volumes/logs for diagnosis. Never start the old engine, downgrade a database, restore an old CA ledger or substitute a rejected historical image.

An equal-source image/export covers artifact loss only. A shared software defect requires a preserved-data stop and reviewed forward correction. Restore backup tests into new targets. A pre-issuer clean OS checkpoint can seed a different new lab identity; it cannot rewind an already issued namespace.

After acceptance, propose exact new guest/client/root/resource retirement separately. Guest-root removal binds its exact SID/thumbprint. Existing installations/profile data, originals, sealed/working/recovery VHDXs, historical backups/CA/evidence and .qodo are never cleanup targets here. No prune, mass uninstall, folder sweep or WSL unregister.

## Cleanup inventory and phase-specific consent

All five exact historical package roots, bounded counts/bytes and public executable hashes are in evidence.json under OWNER_PROFILE.97fc is the historical preview; F196 appears in the fixed/Owner/staged packages. Recovery/bridge/masked-input executables are evidence, not a new installer. No contents of settings/session/private contexts were read. No SFD tray process or owned Explorer verb was observed; AppData/Local/SecureFileDrop exists and stays protected. “Superseded candidate” is not deletion authority; preserve original provenance and private recovery records. Cleanup would require a later exact path/hash/dependency proposal and specific approval.

| Future consent | Exact effect to approve; not approved by this preparation |
| --- | --- |
| A0 target/capacity/licence | Independent host, new VM/disk IDs/paths,160 GiB lab/20 GiB OS reserve, media checksums/Windows entitlement, console/network/download scope; no primary-host feature/reboot change. |
| A1 disposable fixture setup | Six-hour lab identity/window, accepted source/toolchain, fresh generated secret lifecycle, public loopback publisher and listed canaries. Durable real CA custody is excluded. |
| A2 guest trust | Actual enumerated public DER/SPKI/thumbprints, Windows VM GUID/test SID, CurrentUser/Root only, exact canary/reset/removal scope. No Owner/LocalMachine/old-root changes. |
| A3 endpoint/client/UI | New guest localhost origin, exact seven-file8f98 package, normal policy, private synthetic account UI, guest-only Explorer and clipboard; Owner visual testing. |
| A4 closeout only | Stop/quarantine new lab first; optional exact new-resource/root retirement after evidence preservation. Old-package deletion and real installation require separate authority. |

Required runtime locks still missing are the selected host/VM/media identities and licences, guest OS/Engine package versions, built backend OCI/export/binary hashes, final supported proxy/storage compatibility and advisory results, exact new fixture/trust/endpoint consent and real Windows acceptance. These are explicit execution gates, not a claim that installation is ready. No Owner command, password or routine approval is requested at this plan stage.

## Preparation completed and review request

Completed read-only source/ref/accepted-review verification;130-file equivalence; exact seven-file package checksum/identity sealing; host capability/capacity and bounded package metadata inventory; public registry/release identity resolution without layers/engines; original no-origin work-status digest preservation. The old PKI finding is resolved and accepted evidence is reused, not rerun. No SFD service/client, installer, VM or WSL distribution was launched; no new CA/certificate was generated or installed in this preparation.

The new branch contains documentation/evidence only. Aether should review this topology, scope, host/capacity and guest-only consent gates, supported-runtime configuration changes and acceptance/rollback sequence. No re-review of unchanged1805a108 is requested. Keep protected master and PR38–43 unchanged/unmerged. Plan publication, plan acceptance, execution consent, actual demonstration and production readiness remain separate.
