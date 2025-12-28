# Secure File Drop – Native Integrity Utility

This directory contains a small C-based command-line utility used to compute
cryptographic hashes for file integrity verification. It is intentionally
minimal and auditable.

## Purpose

- Read a file from disk (streaming)
- Compute SHA-256
- Emit deterministic JSON output for backend consumption

## Build

The implementation uses OpenSSL for digest primitives. Build using:

```
gcc -o sfd-hash sfd_hash.c sfd_hash_cli.c -lcrypto
```

(If you prefer a Makefile target, we can add one; currently the repo's `native/Makefile` is empty.)

## Usage

```
sfd-hash <file-path>
```

Example:

```
./sfd-hash ./example.txt
```

Output (stdout):

```
{"algorithm":"sha256","hash":"<hex-encoded-hash>","bytes":123}
```

Exit codes:
- 0 – success
- 1 – usage error
- 2 – file I/O error
- 3 – hashing error

## Integration notes

- The backend calls a hashing routine against objects stored in MinIO (via a streaming read) and stores `hash` and `bytes` in the DB.
- Keep the interface stable: JSON output and exit codes are used by surrounding processes.

If you want, I can add a Makefile target and an example test harness that runs the tool against a temporary file and asserts the output format.