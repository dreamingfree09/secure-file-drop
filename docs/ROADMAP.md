# Roadmap status — 2026-10-15

Existing registration, private per-user file ownership, embedded migrations and CI are implemented; see [current tracker](TRACKER.md) for validation. Resumable uploads, scanning, encryption, richer roles/user management and deployment examples remain future ideas. The unmerged feature/v2-enhancements branch is preserved and is not part of this master-based review.

The earlier planning list below is retained as history; check implementation before treating an item as future or complete.

---

# Roadmap

This file describes intended next steps and possible future features.

## Short-term (next milestones)

- Add CI: run `go test` and docs link checks on PRs ✅
- Add contributor templates and a clear contributing guide ✅
- Add small Makefile for developer convenience ✅

## Mid-term

- Add automated DB migrations (e.g., `golang-migrate` integration)
- Add end-to-end tests for the upload/download flow (CI job)
- Add optional resumable uploads support
- Add optional antivirus scanning integration

## Long-term

- Fine-grained user management and multi-user support
- Client-side encryption features
- Scalable object storage backing and replication
- Multi-region deployment examples

## Notes

This roadmap is intentionally light — discuss changes via issues and PRs and update this file as priorities evolve.