# Non-signing CA passphrase recovery assessment - 2026-10-30

Producer Codex; independent recipient Aether. This assessment follows the Owner's clarification that the CA passphrase was not known and none was provided to the consumed signing attempt. The Owner subsequently authorised checking candidate values. Candidate values are excluded from source, context, commands, files, logs and GitHub. The Owner enters any candidate only at a hidden local OpenSSL prompt.

## Preserved incident

Original signing source27f51df1d6486528d97e6b7ca0e79bffae99ecf4/helper SHA3068ba2c960979e540b3d9b94fd977677dd2a5d351819fdc725b6614e9a37fe9 and private contextab8d9498a01ba0d75cca000c31037d935498522eb130cb58b31fa3045db27bc7 remain preserved. One invocation exited1 with empty PEM, no DER, counter1002 and all original issuer pins unchanged. No retry or publication. Exact OpenSSL exit diagnostic was not captured; the indispensable missing input is now confirmed by the Owner. [Actual incident](https://github.com/dreamingfree09/secure-file-drop/pull/39#issuecomment-6511538660).

## Safe investigation and limits

Known SFD TLS namespace metadata contains the original encrypted CA key and the separate server key; no alternate key/PFX bundle or filename suggesting a passphrase/escrow/vault record was identified there. Seventeen top-level SFD TLS preparation scripts were inspected for custody mechanisms without disclosing values. Historical LOCAL_TLS documentation describes hidden input and explicitly does not claim stored passphrases. This limited examination does not establish that no external record or backup exists. No password-manager access, home-wide secret search, shell-history search, private-key/bundle read, key hash, export or copy was performed by Python/Codex.

[OpenSSL pkey](https://docs.openssl.org/3.0/man1/openssl-pkey/) requires the existing input passphrase for an encrypted key; changing an output passphrase is not a reset of the input encryption. A duplicate of the same encrypted key still needs a usable unlocking secret. [OpenSSL passphrase input](https://docs.openssl.org/3.0/man1/openssl-passphrase-options/) supports hidden terminal entry. No guessing values, command-line/environment passwords, passphrase files or security fallback is installed.

Recovery options are (1) locate an existing Owner-controlled vault/offline recovery record or original custodian; (2) identify a separately preserved, authorised usable backup for the same CA identity, with provenance independently verified before use; (3) if signing capability cannot be recovered, prepare a separately reviewed CA/trust transition. No usable alternative is currently established. Option3 is a new trust change, not a CRL1002 renewal; it requires explicit trust/replacement authority and must preserve the current key, certificates, ledger, counters, CRLs, backups and evidence. No CA/key/certificate replacement, trust change or renewal attempt is authorised by this assessment.

## Separate local candidate check

[owner-check-ca-key.py](owner-check-ca-key.py) has a fixed non-signing operation: pinned OpenSSL pkey with private input, consistency check and public-key-only output. Python never reads/copies/hashes private-key bytes or handles a password. Public SPKI must match the existing root certificate. Key metadata and every bound public issuer/consumed-signing artifact hash are checked before/after each local attempt. Original signing helper/context/outputs and issuer are never edited.

The new private one-use check context has its own short deadline, source/binary/public-identity pins and maximum3 local attempts. Each password goes directly to OpenSSL's controlling TTY with echo disabled. After failure, a further attempt requires the Owner's local non-secret NEXT action; STOP/expiry ends the check. OpenSSL runs at most180 seconds per check, bounded by the total deadline. Only public output and sanitised exit/match receipts are retained. Key-valid status is checked separately from public identity; neither enables signing or publication.

[Tests](test_key_check.py) use a freshly generated synthetic encrypted RSA key and PTYs. [Results](key-check-synthetic-results.json):9 named checks passed, including correct/incorrect input, explicit NEXT/STOP, three-attempt ceiling, wrong public identity, changed preservation pin, expired window, noninteractive refusal and fixed public-only argv. Initial synthetic check failed because OpenSSL prepends its validity line to the public output; corrected framing preserves and validates that line before hashing only the DER SPKI. The failed fixture remains retained. No real CA key or Owner candidate was used in tests.

Current source is prepared under the Owner's direct non-signing verification authority. Actual local result is pending until the Owner enters candidates privately. Independent review/integration remains separate; the consumed signing authority is not reused or extended. Docker/startup/recovery/adoption gates remain closed.


### Actual local non-signing result - 2026-10-30

The separate key check at source b833741c2eb512893b075b71917a72dcf080df5e reached a real native TTY and timed out after one bounded OpenSSL pkey invocation: LOCAL_INPUT_TIMEOUT, no verified candidate. It did not sign or change the issuer; all13 preservation pins still match, counter remains1002, and no DER exists. This does not establish that any candidate is incorrect. [Exact sanitized receipt](key-check-execution-20261030.json). No chat-supplied candidate value was copied into an artifact or command. Nine synthetic PTY checks and20 signing-gate unit tests passed. The consumed check context/partial public output is retained and must not be reused. Fresh local input, in a fresh bound context, remains indispensable for further non-signing checks. Signing/publication and any CA/trust replacement remain separately gated.
