# Quality review — 2026-10-15

Security and correctness fixes: explicit administrator sessions, private file ownership, revocable logout, exact link expiry, bounded uploads and actual byte counts, safe filenames, retryable object deletion/cleanup and serialized lifecycle transitions. CI and runtime/native dependencies are reconciled; disposable full-stack tests replace unsafe fallback-based tests. See [the tracker](docs/TRACKER.md) and [PR #21](https://github.com/dreamingfree09/secure-file-drop/pull/21) for actual evidence and limitations.

The historical changelog follows unchanged.

---

# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
- Add/expand project documentation: README, ARCHITECTURE, USAGE, API, CONTRIBUTING, DEPLOYMENT
- Add DB schema summary and native tool documentation
- Add frontend and deployment notes
