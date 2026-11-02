# Stage 1 installation proposal

This is a source-reviewed **proposal**, not installation authority. Start with
[the installation procedure](../../docs/STAGE1_INSTALLATION.md). No application
source, existing runtime, CA, trust store or original data is changed by preparing
this package. `policy.json` contains the exact proposed effects and reserved actions.

## Entry points

| Entry point | Effect |
| --- | --- |
| `install-stage1.ps1 -Phase Plan` | Prints effects only; no host collection or mutation |
| `collect-host.ps1` | Read-only process/service/volume/HCS metadata; unknown evidence fails |
| `guard.py --facts PATH --phase protect` | Pure evaluation, always `authorises_mutation:false` |
| `stage.py` | Creates a new Temp review copy and an explicitly incomplete bundle |
| `install-stage1.ps1` other phases | Reserved future mutator; two fresh external approvals and all live gates required |
| `save-lab.ps1` | Plan by default; `-Execute` can save only source-bound new guests, including after lease expiry |
| `guest-linux.sh`, `guest-windows.ps1` | New-guest UUID/MAC-bound offline configuration; not Owner host installers |
| `prepare-client.ps1` | New Windows guest only; verifies all seven files before exclusive extraction; no launch/registration |
| `ci-runtime.py` | GitHub-hosted Linux runner only; isolated real-stack compatibility and image export |
| `acceptance.py` | Collects hash-bound results; missing/skipped/automated Owner checks cannot pass |

Use the existing Python 3 and Windows Framework compiler; preparation installs
nothing. Run `python -I -B tools/stage1/run-tests.py`, `test-native.ps1` and
`test-powershell.ps1` for source tests. Native tests compile and execute only a
new disposable canary; they never probe a real Docker/WSL activation attempt.

## Source manifest

`python -I -B tools/stage1/manifest.py --write` regenerates the sorted LF-byte
manifest before committing. Without `--write`, it verifies. The manifest excludes
only itself and Python bytecode. Exact commit, manifest, bundle, host identity,
PR, effects, media terms and at most eight-hour UTC lease bind each future
approval. A producer-created JSON facts file or a green synthetic test never
substitutes for live authority or observed safety.

All candidate runtime and CI artifacts remain subject to independent security
review. In particular, the RustFS SBOM matches in `evidence/` are unresolved;
functional compatibility is insufficient to open the runtime execution gate.
