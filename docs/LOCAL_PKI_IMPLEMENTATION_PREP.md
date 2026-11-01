# Independent CA/trust-transition implementation preparation

Work ID: SFD-CA-TRUST-IMPLEMENTATION-PREP-20261101. Producer: Codex; recipient: Aether. Base: b14cc7b88989dc62edcf3c17139d8b315a788f0b. Status: fixture-only implementation proposal ready for independent exact-revision review. No real issuer, trust, publication, selection or integration acceptance.

[Aether6543555392](https://github.com/dreamingfree09/secure-file-drop/pull/41#issuecomment-6543555392), created/updated2026-10-31T12:47:59Z by dreamingfree09/94070428, accepts the design and bounded inert implementation preparation. Its exact UTF-8 body SHA256 is fa78aaa43878f31e0b177ceb065c618b08277e68c57a314d0c9f8743ade4a3a8. The [accepted design](https://github.com/dreamingfree09/secure-file-drop/blob/b14cc7b88989dc62edcf3c17139d8b315a788f0b/docs/LOCAL_PKI_TRANSITION_DESIGN.md) and manifest64e714994e831a42c8af3805518ab405ce166ad0fde621d26b8963dd691cbfb5 remain historical exact-revision references. Both design-head CI41197002822/41197006322 completed successfully; they are not this implementation's PKI execution.

## Delivered source and executable boundaries

[Source directory and reproduction](../tools/pki-transition-implementation/README.md) contains the implementation and evidence. No source in this revision can execute real approval, install/remove trust, select a live certificate or enable the accepted controller.

| Component | Actual behavior and limits |
| --- | --- |
| fence.py | Exclusive UUID Temp fixture roots, exact synthetic marker/owner/mode, bounded ordinary-file reads, symlink/hardlink/path checks, exclusive writes and executable/library pinning. real_transition always refuses. This is an accidental-use boundary, not isolation from a compromised Owner/root account. |
| context.py | Authority/base/source/toolchain-bound60-minute one-use synthetic leases, consumed before fixture key generation. Real issuer/trust/implementation-acceptance flags must remain false. Missing/stale/tampered/source-drift contexts fail. |
| issuer_fixture.py | Linux OpenSSL fixtures only: fresh RSA3072 root/server identities, encrypted PKCS8 CA key through anonymous pipe, secret FD rather than argv/env, independent ledger/counters and one-use signing markers. Files/operations fenced to new fixture roots; CA transactions and backup snapshots take an OS file lock. No old issuer inputs. |
| der.py / crypto.py | Bounded narrow-profile DER parsing; exact RSA/SHA256, KU/EKU/SAN/basicConstraints/CDP and issuer/SKI/AKI/serial/date/revocation/number checks. Windows uses public-only Crypt32 signature verification, including actual Boolean results, without any certificate-store write. Linux uses pinned OpenSSL isolated trusted files with revocation and explicit CRL signature success. |
| publisher_fixture.py | Windows-native Python standard-library HTTP and CryptoAPI; bind127.0.0.1 port0 only, unique fixture path, GET/HEAD, mutation/query/traversal refusal. Signature/currentness/identity/readback checks; unavailable/invalid publication503. Never binds old18081, proposed18082, live8443 or preview18443. |
| lifecycle.py | Read-only renewal plan from a validated signed fixture CRL and independent ledger counter,72/48/24-hour deadlines and max(24h, operation+6h) margin. No scheduled task, auto-signing or publisher startup. |
| approvals.py | Four phase-specific consent previews; creation identities are null/UNKNOWN and bind namespace/profile/custody/backup references. Later stages require exact root/leaf/store/consumer/proxy/adoption bindings. Only CurrentUser/Root is eligible. approve_real always refuses. |
| FixtureWindows.cs / configuration | Actual Framework4.8 target. Public read-only store snapshots, ACLs only on freshly marked disposable directories, disabled masked-input/consent previews, strict HttpClient/SslStream untrusted-root rejection. No visible form launch/Owner input, root installation or validation callback. |
| export_windows_fixture.py | Generates a new Linux fixture; copies public artifacts and only its disposable server key to separately ACL-protected Windows Temp storage. CA key/secret never exported. It cannot target an SFD path. |
| test_linux.py / test_windows.py | Actual cryptographic, custody, publication, consent and strict negative Windows TLS fixtures; no Docker or real application data. |

The real replacement root/key/leaf/CRL identities are still absent. The accepted design policy's future identities stay null. Generated fixture subjects explicitly contain SyntheticSFDFixture; no fixture is distributed for trust or installed. The temporary fixture CA secrets are process-memory test data, not Owner credentials; the source does not delete fixture outputs. Disposable Temp storage may expire: a later receipt-assembly attempt found an earlier Linux fixture directory absent. Persistent source/authority execution bindings are now captured before process exit; failure receipts remain in the private audit area. No claim of durable fixture-key custody is made. No real CA key is read, checked, hashed, exported or copied.

## Custody and lifecycle implementation evidence

The fixture CA key is encrypted PKCS8 using AES256-CBC/PBKDF2-HMAC-SHA256/600000 iterations; tests decode the actual encryption parameters. Plain CA generation output travels only over an anonymous pipe into encryption. The fixture vault is a separate in-memory object; backup/restore uses the recovered fixture credential, verifies ciphertext/ledger manifest, restrictive ownership/mode and exact public SPKI. Wrong credentials, altered ciphertext/manifest, insecure key permissions and consumed creation lock fail.

The implemented snapshot contains seven core custody/ledger files (encrypted CA key, root, configuration, index/attributes, serial and CRL counter). It is not a complete issuer-archive or server-key disaster-recovery backup. Immutable issued-cert/CRL/operation archives and separately encrypted server-key recovery remain required implementation gates before any real transition. These tests prove the nonproduction core copy/restore/public-identity path, not real escrow. They do not demonstrate a password-manager recovery record, independent human custodian, second physical location, approved encrypted medium or an approved live secret interface. Those remain indispensable before any real creation/trust authority. The preview's password box is disabled; it is not an approved live password-entry tool. No chat-supplied candidates are used or retained.

The fixture issuer serial and CRL counters are independent of old1002. Renewal planning uses actual signed nextUpdate and the new ledger counter; it never edits old metadata. Partial signing and lock state are retained, not rolled back or silently retried. No real renewal helper or unattended signing is enabled.

## Publication consistency and failure semantics

Publish one immutable bundle containing DER and complete verified identity/receipt, then atomically replace a small current-pointer file only after the bundle is flushed and an OS-locked highwater advances. Reader checks pointer-to-bundle hash, entire identity, signature/profile/currentness and exact highwater before serving. A fault before highwater leaves old accepted DER selected; a fault between highwater and pointer yields503; a fault after pointer cannot expose unbound DER because receipt and DER are in the same immutable bundle. Readback precedes success.

Tests cover each explicit fault point, corrupt bundle, nonincreasing number, expired/corrupt/wrong-AKI/wrong-issuer material, file-lock contention, publisher unavailability and cached historical DER versus actual publisher readback. Warm valid cached CRL is not evidence of a fresh fetch. No revocation cache or global clock is changed.

This is a fixture implementation, not a validated always-on production service. Crash injection is deterministic control-flow interruption, not abrupt power-loss testing or a full process-kill campaign. Windows filesystem metadata persistence, monotonic-floor recovery across OS/disk rollback, public-service ACL deployment, reboot/login startup, monitoring/alert delivery and actual18082 lifecycle require new reviewed implementation/operational acceptance. A local privileged attacker is outside this same-user file-integrity model. No fixture server remains running after the tests.

## Actual results and preserved failures

Final [Linux result](../tools/pki-transition-implementation/linux-results.json):40 tests,0 failures,0 errors,0 skips. Final [Windows result](../tools/pki-transition-implementation/windows-results.json):17 tests,0 failures,0 errors,0 skips. [Execution/preservation receipt](../tools/pki-transition-implementation/verification.json) binds exact source/probe/toolchain and the earlier failures. These57 logical cases are producer evidence, not independent Aether execution.

Actual OpenSSL isolated-file chain/revocation positives and negatives include hostname/IP SANs, server/client EKU, expired/revoked leaf, invalid/expired/future/wrong-issuer/wrong-AKI CRLs and corruption. Native Windows CryptoAPI distinguishes valid signatures from policy rejection; for example the expired and wrong-AKI CRLs have valid signatures and are still rejected. No private key is needed for the Windows public verifier.

Normal Schannel curl, Framework4.8 HttpClient and SslStream all reject an actual untrusted synthetic HTTPS fixture on an owned ephemeral port. HttpClient/SslStream retain revocation, OS protocol defaults and no custom callback. The native probe also confirms UntrustedRoot with normal chain policy. This is a pretrust negative test, not positive Windows trust acceptance or proof that Schannel distinguishes every revoked/expired case when the issuer itself is untrusted.

Preserve setup/build failures: two initial Linux setups stopped because the CDP configuration needed the URI: prefix; initial native build CS0165 needed explicit local initialization. A later fresh Windows setup correctly stopped on a future CRL time. Read-only clock bracketing found WSL ahead of Windows by approximately2.25–5.65s at that observation. A proposed synthetic CRL-backdating setup then caused10 Linux errors and one Windows setup error because revocation entries followed thisUpdate. It was removed, not hidden by relaxed validation.

Final fixtures retain normal monotonic issuance dates; the conforming expired CRL has a one-second lifetime and is allowed to expire naturally. Windows waits at most10seconds for its own disposable signed date/expiry boundaries, and otherwise fails. No clocks, ledger revocation dates, counters or validation tolerances are changed. Every final negative/positive test passes with strict time checks. Live issuance still needs fresh clock-agreement evidence; this fixture wait is not a live scheduler workaround.

## Toolchain/support evidence

[Toolchain](../tools/pki-transition-implementation/toolchain.json) pins executable and libcrypto/libssl bytes, package versions, Windows Python/Crypt32/curl/compiler and the Framework probe. OpenSSL processes use explicit fixture configuration and a process-local empty default config, not an Owner configuration change.

Observed Ubuntu OpenSSL is3.0.13-0ubuntu3.16. [Ubuntu's package record](https://packages.ubuntu.com/noble/amd64/openssl) identifies that exact security package; [USN8847-1](https://ubuntu.com/security/notices/USN-8847-1) lists that vendor fix level. This is a package/advisory correspondence, not a complete vulnerability clearance. [Upstream3.0 public support ended7 September2026](https://openssl-library.org/post/2026-09-16-eol30/); the version banner alone does not describe vendor backports.

Windows Python3.13.1 embeds OpenSSL3.0.15 for its disposable TLS server; the publisher's CRL validation uses Windows Crypt32, not Python TLS. Neither incumbent runtime is approved for a real issuer/service by these fixtures. Fresh supported-runtime/vendor-advisory/provider/OS-patch review and performance calibration remain gates; no OS/runtime install or upgrade is made on the Owner machine.

No third-party Python package is introduced. Crypt32 behavior is based on [Microsoft's public CRL/certificate signature API](https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-cryptverifycertificatesignatureex); [OpenSSL CA](https://docs.openssl.org/3.0/man1/openssl-ca/), [PKCS8](https://docs.openssl.org/3.0/man1/openssl-pkcs8/) and [verify](https://docs.openssl.org/3.0/man1/openssl-verify/) document the fixture commands. No authentication/storage bypass experiment is involved.

## Future Owner-visible approvals and remaining gates

1. New-root creation: exact reviewed implementation/profile/new private namespace, accessible custody and approved backup location first. Show root identities as UNKNOWN until actually created. Creation alone grants no trust, publication or service change. Verify actual recovered-secret reopen/restore/distinct custody before trust.
2. Trust addition: a separate exact new public DER/SPKI/thumbprint, Owner SID, fresh store-set diff and expiry-bound approval for CurrentUser/Root only. Explain account-wide effects on other applications. No LocalMachine/store escalation.
3. Endpoint selection: only after actual positive revocation-enabled Schannel/Framework4.8/F196 and public DER reachability; bind exact new leaf, CDP, proxy configuration and independent Docker/adoption review. No hash substitution into old PR38 contexts.
4. Old-root retirement: separate exact old DER+thumbprint, replacement acceptance, closed affected-consumer/store inventory and bounded overlap/deadline. Preserve every original artifact; unknown consumer means STOP. No expired-CRL rollback or root reinstall.

Real trust-positive Windows tests, physical custody, actual publisher/reboot/startup, new controller/StrictTransport/package implementation and review, disk/ext4/restart/backup recovery, backend adoption/schema/auth/data/file acceptance and Owner Quick Send/Explorer/clipboard remain unexecuted. Old CRL1001 remains expired; old signing capability is not established. None of these gates is waived.

PR38/39/40/41, protected master61195434bee5f9221a6f55469c719cb360504ea9, all original/producer/WSL checkouts, F196 and historical package, .qodo, old issuer/spent outputs, all backups and disk custody remain preserved. This proposal is not public-production readiness and may not be merged in this phase. The next action is Aether's exact-source implementation review, not Owner engineering commands.
