# Stage 1 F1-F3 corrective design and execution boundary

Work ID SFD-STAGE1-INSTALLATION-20261102. Reviewed baseline
`ff7fcb853981c39b3171c54ea802fa792880e3cc`; accepted feasibility base
`8a469d7c04de964254b9ee7021ba46812eb1443d`. This responds to
[Aether review5742686756](https://github.com/dreamingfree09/secure-file-drop/pull/45#pullrequestreview-5742686756)
and [source-only instruction6590992637](https://github.com/dreamingfree09/secure-file-drop/pull/45#issuecomment-6590992637).
Neither is an executable approval. Final SHA, manifest, complete inventory,
actual test counts, exact-head CI and preservation readback are in PR45.

## F1: blocked independent containment proposal

A PowerShell finally block cannot run after its process is killed. The guardian
inhibits activation; it does not save guests. Previous six-hour wording must be
read as conditional on controller survival, not a host-enforced guarantee.

The safe source contribution is `supervisor.ps1`'s pure Invoke-SupervisorTick
fault model and the deliberately uninstantiable task XML. There is no scheduler
registration code, live process observer, monitoring loop or operational runner.
-Execute rejects unconditionally. All startup-mutating installer phases are also
unconditionally fenced before their first host mutation. The collector reports
independent_supervisor_verified:false; producer bundle flags cannot promote it.
**No independent supervisor is deployed or claimed accepted.**

The proposed separate scheduler process would run save-only checks once a minute
and on host boot, using an explicitly approved Owner SID/token and native
PowerShell5.1 path. An immutable private capsule would bind committed source and
manifest, exact two new VM IDs/roles/names/disks, controller PID plus process
creation time and executable hash, guardian identity/challenge and the earlier
of lease expiry or six hours. PID alone is insufficient; the real identity
adapter, monotonic/freshness checks and capsule staging remain unimplemented.
A dirty checkout or advanced HEAD must not replace the recorded committed code.

A future adapter must load that capsule, query actual identities/state, then
invoke only the same Save-OwnedLab path. Every VM scope is validated before any
save. Off/Saved are noops; Running/Paused permit Save-VM; transitional peers or
save errors are failures, while the other verified eligible guest is attempted.
No force-off, boot, delete, service release, original disk or unrelated VM
operation exists in this containment authority. Immutable CreateNew receipt
files record prevalidation, per-target outcomes and completion/failure with
Flush(true) under the trusted private root. Post-start receipt failure still
attempts the second guest and reports failure. Receipt I/O failure cannot promise
durable evidence on a broken/full disk; pre-save receipt failure stops before any
save. Validation failure saves none. Missing custody rejects rather than writing
an untrusted failure path.

The pure tick model tests controller crash/kill/PID-reuse observations, guardian
loss/expiry and deadline/stale lease, committed source/manifest/target failures,
dirty source, first-save and receipt-I/O failure, retry bounds and bad clocks or
state types. It has a three-attempt save budget and reports terminal failure;
a real adapter must persist attempt state atomically across scheduler invocations.
The callback is a test injection seam, **not** an operational authorization API.
No live process was killed to establish these mocked results.

| Failure | Honest bound |
| --- | --- |
| Main controller exits | A living, correctly installed independent runner could save within poll delay plus scheduling/save latency; not tested on real VMs. |
| Supervisor exits | Scheduler restart may recover after its retry delay. Killed/hung scheduler/runner is not guaranteed; no absolute six-hour enforcement. |
| Host sleep/power loss | No code can save without a running host. WakeToRun is false; recovery on resume/boot is conditional, and forced power-off consistency is not promised. |
| Privileged host compromise | Outside the isolation boundary; administrator/kernel can defeat tasks, locks, receipts or VM identity. |

**Exact new consent delta, after independent source review:** creation of one
named save-only scheduled task, its triggers/restart policy, private immutable
capsule/state/receipts, validated S4U HighestAvailable Owner token and required
Hyper-V rights, lifecycle/retention and separately reviewed removal. S4U access,
owner-logoff/boot behavior and actual scheduler failure modes require isolated
Windows qualification first. Task XML has UNSET identities and is not an import
or registration instruction. No credentials are collected. No service, task,
watchdog, privilege change or host effect is authorised by this correction.
Installation remains NO-GO until this design is implemented, independently
accepted, live-qualified and included in a fresh scoped Owner decision.

## F2: one emergency containment authority

StopGuests dispatches immediately to Invoke-RecordedContainment from save-lab.ps1,
before startup approval, host collection, clean-source, media or expiry gates.
The standalone default is still Plan; -Execute is explicit recovery. Both use
the fixed private root's recorded source/manifest/ownership and load the exact
committed policy and containment bytes. Current working source/HEAD is not an
emergency prerequisite. The loader itself must come from the reviewed source;
a maliciously edited loader is not trusted. Missing objects, mismatched hashes,
wrong root custody or wrong VM scope reject; no fallback to current working code.

The old direct foreach Save-VM path is removed. New V2 receipts contain no
credentials or exception payloads. Lifecycle tests exercise the actual dispatch
AST plus the shared committed function with mocked VM commands; Off/Saved twice,
wrong/duplicate VM/name/disk/extra disk, transitional peer, first-save fault,
receipt fault and dirty/advanced checkout are covered. These are synthetic
Windows PowerShell5.1 tests, not actual VM save acceptance.

## F3: identity-separated V2 authority, currently blocked

V1 packets are permanently rejected. The real committed policy pins repository
1123523317, PR45 identity5122311666 and Owner94070428/dreamingfree09. Reviewer
id/login are null, enrollment_accepted:false. The producer uses the Owner account;
that account cannot fill the reviewer role. **No independent reviewer identity
has been enrolled; every real approval attempt remains NO-GO.** Synthetic tests
use explicit in-memory fictional IDs only, never a committed fake enrollment.

V2 requires two distinct direct authenticated GitHub User comments with pinned
numeric IDs/logins, suitable repository association and no GitHub App proxy.
Roles in text do not confer identity. Exact repository/PR IDs, current open PR
head, source, manifest, bundle, host identity, effects, media terms, paired nonce,
UTC issue/expiry (at most eight hours), unedited creation/update times and Owner
reference to the exact reviewer comment ID/raw UTF8 SHA256 are checked. The
Owner packet may not precede or outlive the reviewer packet. Metadata/body/ref
are read again after validation; edit/deletion/drift fails closed. Receipts retain
both author IDs, comment IDs and hashes. Subsequent drift and one-use boot markers
remain mandatory; a public API read cannot guarantee atomicity against a later
edit or account compromise.

Before this can open, a separate independent reviewer account must be enrolled
through reviewed source and authenticated identity/custody verification. Distinct
account IDs alone do not prove different humans, private credential custody or
MFA. Establish that independence explicitly; do not create a second account for
the producer or reuse shared credentials. Existing Aether review and App-authored
source instructions are governance evidence only, never V2 execution packets.
Enrollment does not itself grant installation: a new context-bound reviewer
acceptance and Owner GO are still required after all gates close.

## Retained gates, preservation and testing limits

No change to application61195434, PKI1805a108, accepted client or root Compose.
New issuer port18082 only; legacy18081 remains forbidden. C14GiB initial/10GiB
running headroom, H168GiB budget/reserve, elevated complete HCS/disk metadata,
lawful signed offline media/licensing/setup and RustFS23 advisory matches remain
unresolved. Green compatibility is not runtime/security acceptance. Exact-image
applicability/remediation or justified VEX still requires independent review.
Later guest-only issuer/bootstrap, root/SID trust consent, cold revocation-enabled
Schannel/Framework/client, private networking, logical restore and real Owner
Quick Send/Explorer/clipboard acceptance remain NOT RUN.

All real VM/VHD, host watchdog/task, engine, feature/service, trust/PKI, media,
original data/backup and merge operations are outside this source-only stage.
Original repositories, CA, disks, backup evidence and Docker/WSL state remain
preserved. Private originals are protected by nonaccess, not a fabricated content
scan. Local test fixtures create only new Temp directories/canaries; no original
ACL or attribute is changed. Historical failed CI is retained. Baseline red/green
results are in tools/stage1/evidence/corrective-red-green.json; final CI/receipts
and any new failures belong in the revision-specific PR handover.
