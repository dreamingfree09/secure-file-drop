# Secure File Drop – Native Integrity Utility

This directory contains a small C-based command-line utility used to compute
cryptographic hashes for file integrity verification.

## Purpose

The utility is responsible for:
- Reading a file from disk
- Computing a SHA-256 hash
- Producing deterministic output for backend consumption

It is designed to be:
- Minimal
- Deterministic
- Easy to audit
- Safe to call from other services

## Planned Interface


sfd-hash <file-path>


### Output (stdout)

```json
{
  "algorithm": "sha256",
  "hash": "<hex-encoded-hash>",
  "bytes": <file-size>
}

Exit Codes

0 – success

1 – usage error

2 – file I/O error

3 – hashing error