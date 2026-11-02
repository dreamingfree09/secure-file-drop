# Secure File Drop Stage 1 — managed local lab installation proposal

Work ID `SFD-STAGE1-INSTALLATION-20261102`. Producer Codex; independent reviewer
Aether. This package is preparation under [Owner decision6576173510](https://github.com/dreamingfree09/secure-file-drop/pull/44#issuecomment-6576173510),
not a GO to install. Accepted feasibility base8a469d7c04de964254b9ee7021ba46812eb1443d
and application master61195434bee5f9221a6f55469c719cb360504ea9 remain preserved.
The complete revision-specific GHP-01 handover, CI identities and acknowledgement
belong in the producer PR. No self-acceptance or production-readiness claim.

## Decision and actual blockers

Select a managed Ubuntu24.04 LTS amd64 VM plus a clean Windows11 Enterprise LTSC2024
x64 evaluation VM on one Hyper-V **Private** switch. This is the smallest selected
configuration with an explicit host-independent *network*, separate Windows trust
scope, bounded disks and guest-only clipboard/Explorer testing. It shares the PC,
hypervisor and physical disks. Owner accepts that residual hardware/administrator
failure risk. It is not a boundary against a hostile host kernel/administrator.

A separate Windows account with WSL still shares host WSL services/swap and does
not prevent original Docker activation. Sandbox adds C-backed ephemeral storage
and network/clipboard qualification. Neither is used. There is no Owner WSL,
Sandbox, SSH relay, Default Switch, external NIC or host port forwarding in this
selected route. Microsoft documents that a Private switch has no host networking
[here](https://learn.microsoft.com/en-us/windows-server/virtualization/hyper-v/plan/plan-hyper-v-networking-in-windows-server).

**Execution recommendation today: NO-GO.** Preparation can be reviewed now.

| Gate | Actual evidence / required resolution |
| --- | --- |
| C headroom | Read-only14:53UTC snapshot9.4766GiB; proposed minimum14GiB before staging and10GiB running. No cleanup or deletion performed/authorised. Safely add capacity or separately approve unrelated space management before GO. |
| H budget |275.67GiB observed,168GiB initial threshold. Root `H:\SFD-Isolated-Lab-20261102-8df9c4a1` absent. No reservation/write performed. Recheck exact volume and available space at execution. |
| Administrative evidence | Current automation token not UAC-elevated; HCS enumeration incomplete, HRESULT0x8037011B (HCS_E_ACCESS_DENIED in installed Microsoft SDK26100 winerror.h). Fresh elevated read-only attachment/compute qualification required. Never start vmcompute merely to make this check pass. Unknown is NO-GO. |
| Original prevention | Native exclusive-handle canary passed; no real Docker/WSL lock or service change performed. Exact preventive implementation needs Aether review, then explicit informed Owner GO. |
| OS media / rights | Neither ISO is staged or checksum/signature-qualified. Host Pro is not a guest licence. Microsoft evaluation registration, eligibility, activation/account requirements and offline provisioning compatibility remain unresolved. Do not bypass OOBE/activation or attach Internet temporarily. |
| Runtime security | Official RustFS1.0.4 upstream SBOM has23 package/advisory matches,22 distinct IDs,1151 components. Image-to-SBOM correlation, affected source paths and remediation/VEX are unresolved. `runtime_acceptance_verified` stays false. No exploit attempted. |
| PKI | Accepted fixture stays fenced. Lab-only bootstrap and guest SID/root-specific approval are required before real issuer/trust/HTTPS. Old expired CRL cannot satisfy acceptance. |

No one should interpret the title “installation ready” as those gates passing.
If these cannot be resolved within this topology, retain NO-GO rather than booting
a partly isolated lab. The script itself requires two fresh external approvals,
verified install bundle and live facts before its first mutation.

## Frozen components and storage

| Component | Exact identity |
| --- | --- |
| Application |61195434bee5f9221a6f55469c719cb360504ea9 |
| Accepted PKI source |1805a108855817d7b36220a6253684bd6a018fb3; existing live/trust fences unchanged |
| Client source |c320861536914623ea22f28c1d4ea156d4c2132a; application desktop tree998e690a5239fc2de9bf4e4b5d259c8022e3611a |
| Seven-file preview ZIP |f38b9f82dda5c0320c17a3d0a04de924c958750f8975be28d3914d0ec8b90c9e |
| Preview executable |8f98bd9dca095c827e8ccf38000ae28fecf45df60ec10c9b484ed479748bb28b |
| Preview config |4d83c0772f4b593d6e16657fb5be7e4478321cb9782ba744c93b90915fb7175b |

Official preview run40403397291/artifact12690070973 expires9November. Preserved
sealed archive is not launched or overwritten. F196 is an older distinct build
of the accepted transport; do not call the new hash F196 or binary-identical.
All seven member hashes/lengths are in [client-members.json](../tools/stage1/client-members.json).

[policy.json](../tools/stage1/policy.json) defines Linux8GiB/2vCPU/32GiB fixed disk
and Windows8GiB/2vCPU/64GiB fixed disk. Host minimum24GiB RAM; recheck available
RAM, supported CPU, virtualization/SLAT and Windows feature availability before
installation. Current source does not claim these metadata checks are a boot test.
The H budget is32+64GiB disks,8GiB capacity-only new-lab logical recovery reserve,24GiB inputs,
4GiB bounded logs,16GiB saved guest RAM and20GiB reserve =168GiB. Initial fixed
allocations and recovery reservation must physically allocate; sparse/compressed
or parented disks fail. Free H must stay at least20GiB. The extra8GiB file is capacity-only quarantine, **not a completed backup or a
whole-VM recovery promise**. It is not automatically consumed/released/overwritten.
Whole-VM copying is outside this proposal; a saved VM disk alone is not a
consistent standalone backup. Rollback saves the new guests and preserves their
files in place. Mandatory logical PG/object/issuer export and restore uses only
small new-lab synthetic data and the accepted recovery tests; record actual total
bytes, consistency and hashes, and stop if they exceed the8GiB bound. Authorise
any later host-custody transfer separately by exact new-owned paths. Original
archives and VHDX copies are never inputs or part of this reduced *new-lab* budget.

C budget14GiB is a conservative proposal (8GiB OS margin,4GiB servicing estimate,
2GiB guard/transient), not Microsoft's exact servicing requirement. No WSL swap
or Sandbox allocation is permitted. H VM paths include configuration, paging,
checkpoints-disabled state and saved memory. If measured servicing/host pagefile
commit needs more, increase the gate and obtain revised exact approval; never
silently lower it. The host feature's servicing/reboot effects require disclosure.

New lab root ACL is created atomically with only the executing Owner SID and
SYSTEM, inheritance protected. Existing paths are refused; no broad ACL is fixed
in place. Every future source/input/disk path is restricted and checked for
reparse aliases. The proposed offline inputs live under the private producer
`.git/sfd-stage1-installation-20261102-64e8b793/inputs`, not original checkout,
AppData, VHDX, backup, `.env`, certificate key or PFX files.

## Preventing original-engine activation

A stopped engine, new Compose project name or watchdog alone is insufficient.
The proposed reserved ProtectHost phase requires no Docker process, no WSL
workload, complete HCS/attachment metadata and original Docker already stopped.
It creates immutable new-root ownership, source/approval receipts and original
SCM state before touching an allowed service. It then disables/stops only
`com.docker.service`, `WSLService`/`LxssManager` where present. A running Docker
backend is a rejection; it is never stopped by this installer. Stopping an idle
WSL manager still changes Owner WSL availability and is explicitly an Owner GO
effect, not routine metadata collection.

The native guardian opens approved installed Docker/WSL activation executables
with **FileShare.None**, hashes through retained handles, and exposes a
SID-private nonce challenge bound to PID/start time/executable hash/target digest.
Only fixed public installed binary paths are allowed, not original data. A
newly compiled harmless canary demonstrates launch/read denial and recovery after
release; wrong hashes release partial handles. This is preventive maintenance
protection against ordinary activation, not proof against privileged tampering,
updates replacing installation trees, or arbitrary uninstalled runtime binaries.
Any drift invalidates execution. No real binary-lock acquisition was tested.

SCM inhibition persists if guardian/controller dies, its eight-hour lease ends,
or the host reboots. It is **not automatically restored**. A separate preservation
review must approve original-engine release because original backup pruning can
otherwise recur. Original service state is recorded, not silently discarded.
No engine metadata/restart policy/original schedule changes are proposed.
AppLocker is not used; no AppIDSvc or application-control policy changes occur.

## Future installation procedure — Codex-operated, one bounded GO

1. **Read-only preparation now.** Verify source manifest, exact GitHub authority,
   source clean state, hashes, signed OS/package metadata, evaluation rights and
   runtime advisories. Stage only public inputs into the private preparation
   folder. `stage.py` can make a new Temp review copy; its bundle is intentionally
   incomplete. It cannot fabricate a media/security receipt. Complete an exact
   bundle only from qualified inputs and independent review.
2. **Fresh Aether acceptance and Owner GO.** Bind two distinct GitHub comment IDs
   on the proposal PR, exact head/manifest/bundle/host identity, ordered effects,
   approved media terms and at most eight-hour UTC lease. `common.ps1` fetches
   actual public GitHub comments; arbitrary local JSON cannot authorise action.
   Current Owner preparation decision does not match this execution packet.
3. **ProtectHost, then EnableHyperV.** Run the reviewed controller with those
   exact IDs and bundle. Fail before root/service changes if any initial gate
   fails. Persist receipts, inhibit original services, acquire and challenge
   guardian. Enable only Microsoft-Hyper-V-All with NoRestart if needed. Never
   auto-reboot. If a reboot is required, stop for the specifically disclosed
   reboot boundary and resume with fresh qualification/lease. Do not start WSL.
4. **CreateGuests.** Require no existing VM configuration, one-use creation
   marker, sufficient C/H, exact inputs and guard. Create only new fixed H disks,
   one Private switch, two Gen2 VMs, static MACs, Secure Boot and a Windows guest
   local vTPM. Automatic start Nothing; checkpoints/Guest Service Interface off;
   no pass-through/shared disk, host share, ISO outside input whitelist or extra
   NIC. Local vTPM key protection creates host-managed VM key-protector material;
   it must not add/change Owner Root stores. Fresh Root-store before/after
   identities are a live acceptance gate, not already proved here.
5. **BootGuests and offline OS installation.** Recheck exact IDs, disk allocation,
   all media hashes, NIC protections, vTPM, guard and headroom immediately before
   each Start-VM. One-use boot marker prevents automatic replay. Use basic
   VMConnect only; no enhanced-session drive/printer/clipboard/redirection. No
   host setting is changed to enable enhanced access. Controller watches five-
   second preservation/headroom checks for at most six hours, ending30minutes
   before lease expiry, then saves only new guests in finally. Partial provision
   is retained; no automatic delete/reset/retry. A controller crash does not
   restore original services. Offline OS setup is Codex-operated graphical
   work; unattended OS installation is **not implemented or claimed**. If legal
   media cannot complete setup offline, stop; do not add an egress NIC or bypass
   account/activation controls.
6. **Guest-only bootstrap.** Attach source-bound SFDIDENTITY ISO containing the
   actual BIOS UUID (not an assumed VM UUID) and reviewed public payload ISO.
   Linux script verifies role/UUID, one MAC, no mounted host filesystems/default
   route, exact packages/signature receipt and all hashes. Use only reviewed
   offline DEBs; default Docker/containerd units masked and package auto-start
   inhibited. New daemon sockets, roots and namespaces are listed below. Windows
   bootstrap verifies role/BIOS/MAC/no H drive; guest-only static network and
   loopback portproxy; no root import. These scripts have not run in actual VMs.
7. **Stop before trust/application launch.** Stage1 never creates the real SFD
   replacement CA or installs a root. A separate exact lab-only PKI bootstrap
   review plus public-root/SID-specific guest consent is necessary. Accepted
   PR43 fixture fences remain unchanged. Complete strict cold revocation tests
   before any credentials. Prepare seven client files only inside new Windows
   guest; no automatic launch/Explorer registration. Proceed to the E2E workflow
   below only when its prerequisite gates are accepted.

These are future Codex actions; no Owner terminal commands are requested.
Use `install-stage1.ps1 -Phase Plan` for the effect inventory. Every mutating
phase requires ReviewComment/OwnerGoComment/BundlePath; zero/missing IDs reject
before collection. There is currently no valid execution packet or complete
bundle. The proof-bound booleans in a reviewed bundle represent external media,
network-payload and security review, **not independent live guest observations**.
Live guest routes, packet denial, root scopes and actual bootstrap remain required.

## Runtime and network contract

The exact candidate image digests and five official Noble package identities are
in [runtime-lock.json](../tools/stage1/runtime-lock.json). Go1.26.10/Alpine3.24,
PostgreSQL16, Traefik3.7.17, RustFS1.0.4, Engine29.8.2/containerd2.4.2 are selected
candidates. No superseded historical successor/recovery image is reused.
Hosted browser/runtime compatibility uses Ubuntu22.04, the accepted canonical browser-runner platform, with Chromium sandbox still enabled. Source/Compose safeguards separately use Ubuntu24.04. A first Ubuntu24.04 runtime run reached84 checks per layout but browser launch failed at fixture guard; no TLS error was reported. This is retained as a platform limitation, not a passed Ubuntu24 browser check. Actual Ubuntu24 guest qualification is still mandatory.

CI builds frozen application611 and a recovery artifact from the same source;
binary/image results belong to that actual CI run. A source pin is not a signed
package or vulnerability acceptance. Exact offline dependency closure, Ubuntu
signed InRelease/Packages verification, Windows servicing and full image scan
remain prerequisites to a complete bundle. Do not silently substitute “latest”.

New Linux engine: `/var/lib/sfd-lab/docker`, `/run/sfd-lab/docker`,
`unix:///run/sfd-lab/docker.sock`, `/run/sfd-lab/dockerd.pid`.
New containerd: `/var/lib/sfd-lab/containerd`, `/run/sfd-lab/containerd`,
`/run/sfd-lab/containerd/containerd.sock`, namespaces `sfd-lab`/`sfd-lab-plugins`.
No TCP engine endpoint or host socket mount. Default units stay masked; transient
new units and **every container restart:no**. Guest has no Internet route.

[compose.yml](../tools/stage1/compose.yml) is a separate four-service specification,
never historical root Compose. It uses imported config IDs plus pull_policy:never,
new labelled PG/object/backup volumes, internal bridge, bounded logs/memory/PIDs,
dropped capabilities and no-new-privileges. No original `.env` or stored file.
Lab backups retained30days/scheduled24h; file cleanup disabled. Recovery image
is artifact-only, never automatic downgrade/rollback of schema metadata.

Private subnet172.31.250.0/30: Linux.1, Windows.2, no gateway/DNS/DHCP/RA. New
Windows guest localhost8443 and18082 forward only to Linux.1 same ports; no TLS
termination at portproxy. localhost certificate remains valid for the unchanged
client transport. New issuer CDP is **18082**; legacy18081 is forbidden. No
host listener, host trust, LAN route or SSH private key is needed. An offline
public CRL endpoint in the Linux guest still requires separately reviewed lab
bootstrap; the existing Windows-native fixture publisher is not silently
repurposed or unfenced. Guest Windows IP Helper/IPv4+IPv6 localhost forwarding,
firewall, actual route/cold fetch and refusal of external traffic require live
verification; unresolved forwarding is a stop, not insecure fallback.

## End-to-end acceptance workflow

[acceptance.json](../tools/stage1/acceptance.json) is the machine-readable19-step
ledger; [acceptance.py](../tools/stage1/acceptance.py) refuses missing/hashless
passes, skips-as-passes and producer claims of Owner visual acceptance.
It is a collector, **not an implementation of every scenario**.

1. Repeat preservation/guard/source/guest-disk and network packet-denial checks.
   Snapshot host Root stores without importing anything; prove unchanged after.
2. Verify imported runtime config IDs, offline package signatures/versions,
   complete image SBOM/advisory review and both source-derived image identities.
   Run actual selected source schema12 fresh/BYTEA/BIGINT upgrade/idempotency
   tests against new PG only. No original data restoration or schema downgrade.
3. After separate guest issuer/root approval: create only the unique lab issuer,
   recoverable encrypted custody/archive, leaf SANlocalhost/127.0.0.1/::1 and
   signed CRL at18082; root/ledger/AKI/serial/validity identities recorded. CRL
   must cover entire test window (at least36h remaining for six-hour session).
   Prove repeat recovery/forward signing on disposable issuer before using it.
   Install public root only into the approved new guest CurrentUser SID scope.
4. Use fresh guest profiles/cold caches for stock Schannel curl, explicit
   Framework4.8 handler and actual companion: valid chain/CDP succeeds; revoked,
   expired, wrong-name, unknown-root, bad-signature/issuer/stale CRL and unavailable
   publisher fail. Warm-cache success alone is insufficient. No bypass callbacks,
   `-k`, revocation disable, root installation on Owner host or expired old CRL.
5. Deliver ordinary test registration/verification through a new private lab
   SMTP fixture. No external mail, original credentials, direct record injection
   or verification shortcut. Require admin and two distinct ordinary identities,
   login/me/ready, lockout, invalid/expired/tampered/unknown/revoked sessions,
   logout and cross-user ownership isolation. SMTP/UI delivery adapter is still
   to be bound into the offline guest bundle; hosted harness covers real delivery.
6. Upload synthetic files through browser/client, download via signed links,
   compare original and downloaded SHA256, enforce1MiB account quota and bounds,
   preserve bytes/rows on failures. Require a real five-minute expiry→HTTP410.
   Verify lab backup hash/restore in a separate new owned PG/object target,
   retention policy and24h schedule; no historical pruning/cleanup is enabled.
7. Owner operates only the final new-guest GUI: tray login/status, picker,
   progress/cancel, Explorer Quick Send for synthetic file, guest-only link
   clipboard/two-minute clear, downloaded bytes/SHA256 and clean exit. Explorer
   registration is explicit per-user **new guest only**, not automatic. Host
   clipboard is not used for credentials/files/links; Basic VMConnect provides
   display/input only. Record actual human results independently of hosted tests.
8. Require the new-lab logical backup/restore results from step6, actual byte
   totals within8GiB and independent hash readback. Then save both new guests
   and compare preservation metadata. No full-VM clone or disk-only backup from
   a saved VM is claimed. The host capacity reservation remains untouched by
   this controller. Do not delete packages or retire old CA. Report remaining production/storage/TLS/capacity
   gates explicitly. Genuine visual acceptance is never inferred from green CI.

## Rollback and one proposed Owner decision

Before any VM boot, failure retains evidence/new partial objects and starts
nothing. After boot, `save-lab.ps1 -Execute` validates source/ownership/VM IDs and
exact new disk paths, then Save-VM only; it needs no unexpired startup lease.
No force power-off, file deletion, old-engine start or automatic SCM restoration.
New feature/switch/VM definitions remain for review; no auto-uninstall. If save
cannot complete, report the exact failure and preserve all files rather than
forcing shutdown. Original service inhibition remains deliberately visible until
a separately reviewed original-preservation release. Old trust retirement is
outside Stage1. There is no destructive “cleanup” prerequisite.

**After Aether accepts this exact source and the blockers above are resolved,
ask ONE specific Owner GO/NO-GO:** permit creation only of the named H root and
two new fixed disks; temporary durable Docker/WSL service inhibition plus native
binary locks (original services stay inhibited until separately reviewed release);
Hyper-V feature staging without automatic reboot; one Private switch, local
new-VM vTPM protector, two new guest boots for at most six hours, offline OS and
runtime installation under the exact bundle/media rights. Disclose any required
host reboot as a separate indispensable scheduling boundary; no unattended reboot.
**Exclude** Owner host trust, guest root installation/CA creation, old-root
retirement, original disks/data/engine, egress, deletion and merges. The later
exact guest root/SID/issuer approval is phase-specific and cannot be bundled as
implicit consent. Do not ask this question before Aether review.

## Preparation evidence and preservation

[authority.json](../tools/stage1/evidence/authority.json) records exact comment
body SHA25633e3fed9e1af63490b870e044b0c67524899b3b20d2cc3ae9108e2f8385c5597.
Source tests and native canary are preparation evidence, not actual VM/network/TLS
acceptance. CI identities, actual image outputs and final file inventory are
reported in the PR after publication; initial failures are retained and corrected
normally, never hidden by skips. The accepted PKI98fixture/12job result is prior
accepted evidence, not a claim of newly run live PKI.

Original no-origin Windows checkout853b865e30872964a556c225335eb2191928972b
has8 expanded status records, raw LF-byte SHA256
cfd90c22d5da74d22009f003ac3906402f957be535b37cf7217a111689e3a41f.
Producer6d076944a0874a39124f7e8d7ee921c32ef77411, protected master611,
PR38–44 heads, WSL feature history, `.qodo`, old CA/CRLs, VHDX copies and original
backups are preserved. WSL/private CA/disks/backups are protected by nonaccess;
this is not a new full-content integrity scan. No original byte access, VM boot,
engine invocation, H write, service/feature/host firewall/ACL/trust change,
original restore, package deletion or merge occurred during this preparation.

SBOM findings are stored in [OSV query results](../tools/stage1/evidence/rustfs-SBOM-OSV.json)
and [advisory records](../tools/stage1/evidence/rustfs-advisory-records.json).
They include zstd/zstd-safe memory-safety records, russh, rsa, lru, pageant,
crypto-bigint and pulldown-cmark. No runtime exploit was reproduced. Applicability
needs exact image/source proof and fixes or justified VEX, not a blanket dismissal
because optional services are disabled. Full runtime acceptance remains closed.
