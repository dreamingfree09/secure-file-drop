# CRL 0x1002 renewal preparation

Work ID: SFD-CRL-1002-PREPARATION-20261027. Codex produces; Aether independently reviews. Base master: 61195434bee5f9221a6f55469c719cb360504ea9. [Stage A/B and preparation authority](https://github.com/dreamingfree09/secure-file-drop/pull/38#issuecomment-6454489528). This is separate from the unchanged accepted PR38 controller ff2f9a74bcbf2a347eed61516508910acadadfb2.

## Executed preparation

The actual retained issuer has two valid ledger rows, published CRL 0x1001, next counter 1002. Its current DER SHA256 is 69bbe78994b7e6abe9849e65fd8a56fe59dbe0803e6db498f3e8c82ad70f6f1e. Existing NextUpdate is 2026-10-29T18:10:29Z; the hard-pinned renew-by is **2026-10-28T18:10:29Z**. Neither has been extended by this preparation.

Prepared helper SHA256: 3068ba2c960979e540b3d9b94fd977677dd2a5d351819fdc725b6614e9a37fe9. The exact same bytes are held locally in the new private issuer subdirectory renewal-1002-20261027-1da96c45. Private context SHA256: ab8d9498a01ba0d75cca000c31037d935498522eb130cb58b31fa3045db27bc7. OpenSSL executable SHA256: b4f547c0f16272d1cc7f870330a6f2c243710c335e7a99c734ff7a0c6fdb2503. The full private context remains local; it contains filesystem identities and public/ledger pins, not a passphrase or key contents.

The real read-only context check passed, including signature verification of existing CRL0x1001. An actual invocation requesting signing **without** an acceptance binding refused with SIGNING_REVIEW_REQUIRED before any signing operation. All pinned ledger/publication hashes remained unchanged; no started marker, new PEM, DER or signing verification report exists. Preparation never opened, hashed or copied CA private-key bytes; only regular-file ownership, mode and stat identity were examined.

Isolated standard-library tests are in test_renewal.py. They mock OpenSSL where needed and do not establish a new real signature. No synthetic or real signing occurred. Run them with Python3 -I -B and the absolute test path. Application/backend/browser/Windows acceptance is unchanged and is not claimed by these helper tests.

## Future signing procedure requiring independent acceptance

1. Aether reviews this exact source, its hash, private-context binding and this mutation plan. No current signing-acceptance file exists. Before enabling anything, Codex must retrieve an explicit later Aether signing acceptance, verify its comment body/hash and exact source/context identities, and create a one-use private acceptance file with a bounded validity window. The helper checks this local binding; it does not independently authenticate GitHub or turn a fabricated local file into approval.
2. Recheck the issuer, current CRL, counter, key metadata, helper and OpenSSL identity. If any pin changes, stop and reconcile. Arrange a separately verified local interactive launch; the Owner must not run technical commands or send secrets in chat. Only the existing encrypted-key passphrase goes directly into OpenSSL's hidden local TTY prompt.
3. The helper acquires the existing issuer-wide flock, repeats the pins, refuses any prior signing outputs, preserves both counter and crlnumber.old preimages in exclusive new private files, and creates a one-use started marker. OpenSSL then generates only a CRL using the existing exact config/root/key/ledger. It writes PEM to an exclusive new private output, advances crlnumber from 1002 to 1003, and normally replaces crlnumber.old with 1002. No certificate is issued, revoked or regenerated.
4. Verify issuer/signature/CRL0x1002, seven-day lifetime, recent LastUpdate, active leaf hostname and CRL check, counter1003, expected old-counter preimage, unchanged config/index/serial/root/leaf/publication and unchanged key stat identity. Convert and independently verify DER; publish only its actual resulting SHA/validity. No new signed identity or expiry is predicted here.
5. On interruption, passphrase failure or any postcondition failure, retain all outputs and issuer state for investigation. **Do not retry, decrement the counter, overwrite outputs or automatically roll back the issuer ledger.** OpenSSL counter mutation and PEM output are not one transaction; a partial attempt can consume a number. The preserved preimages are evidence, not permission to restore them.

## Publication and controller continuation remain separate

This helper has no publication, listener, trust-store, cache or Docker operation. After legitimate signing and independent acceptance of the exact new DER, Codex must prepare a same-filesystem exclusive staging publication, verify signature/identity/validity, atomically replace only the public CRL, read back exact bytes over loopback and execute strict Windows Schannel plus revocation-enabled .NET checks. Preserve the old public DER as evidence and preserve root, leaf, key, TLS settings and services. Never roll back to an older CRL number to conceal failure. A publication failure is a stop requiring evidence, not a reason to disable revocation.

The existing ff2f9a7 controller pins CRL0x1001/hash/deadline. New signed material makes that binding stale. A renewal-compatible controller/TLS continuation needs its own changed exact-revision review before execution. PR38 remains unmerged; Docker start, three-archive restoration, backend/schema adoption, credentials form, account creation and Windows Quick Send remain on hold. No production-readiness claim.


## 2026-10-30 separate non-signing key check

The consumed CRL1002 signing invocation remains spent, with no new DER. [Recovery assessment](PASSPHRASE_RECOVERY.md) and the separate owner-check-ca-key.py provide Owner-authorised hidden local candidate verification without signing or issuer changes. Nine synthetic checks pass; actual local input remains necessary. This does not reopen or alter owner-renew-crl.py, its context or outputs. Exact execution/source evidence belongs in PR39.


### Actual local non-signing result - 2026-10-30

The separate key check at source b833741c2eb512893b075b71917a72dcf080df5e reached a real native TTY and timed out after one bounded OpenSSL pkey invocation: LOCAL_INPUT_TIMEOUT, no verified candidate. It did not sign or change the issuer; all13 preservation pins still match, counter remains1002, and no DER exists. This does not establish that any candidate is incorrect. [Exact sanitized receipt](key-check-execution-20261030.json). No chat-supplied candidate value was copied into an artifact or command. Nine synthetic PTY checks and20 signing-gate unit tests passed. The consumed check context/partial public output is retained and must not be reused. Fresh local input, in a fresh bound context, remains indispensable for further non-signing checks. Signing/publication and any CA/trust replacement remain separately gated.
