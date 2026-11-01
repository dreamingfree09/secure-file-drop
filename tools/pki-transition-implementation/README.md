# Fixture-only CA/trust implementation preparation

Read [scope, architecture, executed results and future authority](../../docs/LOCAL_PKI_IMPLEMENTATION_PREP.md). This is not a deployable replacement issuer or trust installer. Source entry points accept only newly marked UUID Temp fixtures; real transition and approval functions always refuse.

Public evidence: [toolchain](toolchain.json), [Linux40](linux-results.json), [Windows17](windows-results.json), [preservation/limitations](verification.json) and [source manifest](source-manifest.json). Exact final revision and CI run status belong in the PR description. No private fixture keys, secrets or CA ledger content are committed.

## Engineering reproduction (not an Owner instruction)

On the pinned Ubuntu fixture host, run python3 -I -B tools/pki-transition-implementation/test_linux.py. It creates private UUID directories under Temp, does not delete its fixture outputs, and writes linux-results.json with source bindings before exit. Temp storage can expire independently; persistent receipts, rather than Temp paths, are the evidence. No Docker/install or real SFD path is used. Binary/library drift fails rather than silently changing pins.

For Windows native tests, build FixtureWindows.cs using the pinned Framework64 csc compiler with references System.Net.Http.dll, System.Web.Extensions.dll, System.Windows.Forms.dll, System.Drawing.dll and System.Security.dll into a new private output directory. Copy FixtureWindows.exe.config alongside it. Run its --self-test, then pass the resulting executable path as the sole argument to test_windows.py using pinned Windows Python. The runner creates/ACL-verifies fresh fixture directories, creates only ephemeral loopback endpoints, exports a new synthetic Linux fixture through the existing Ubuntu WSL toolchain, and shuts down its owned servers. It never starts Docker or requests an Owner secret.

The native tool supports only --self-test, --store-snapshot (read-only), --secure-fixture-root (new marked Temp directories only) and --strict-reject (nonreserved ephemeral localhost TLS fixtures). There is no command to display an Owner form, install/remove a root, use a real issuer or adopt a backend.

FixtureWindows Preview forms have a disabled masked field and a Close-only button; approvals.py supplies phase-specific public identity text. A separately reviewed future bridge is required to display an actual approval and accept any genuine local secret. Do not reuse this preparation build as a live credential interface.

Synthetic lease contexts bind authority6543555392, accepted designb14cc7b, current implementation/source/toolchain hashes and fixture nonce. They last at most60minutes and cannot contain true real-issuer/trust/implementation-acceptance flags. They are not substitutes for an Aether implementation review.

## Claims deliberately not made

Positive trusted-root Windows TLS, actual Owner vault/second-location recovery, real CRL publication/startup/renewal, power-loss durability, real controller/package compatibility, backend adoption and physical Quick Send are unexecuted. OpenSSL fixture positives do not substitute for these. Native Windows validates public cryptographic signatures and strict untrusted-root rejection without adding trust. See the complete limitations and preserved failure history in the technical document.
