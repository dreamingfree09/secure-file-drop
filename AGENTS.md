# Secure File Drop agent instructions

The Owner and Aether retain planning, architecture and significant decisions. Codex implements authorised work. Start at README.md, docs/CONTINUITY.md, docs/GHP-01.md, docs/SPEC.md and CONTRIBUTING.md. Keep the established lowercase docs/ directory.

Verify the actual Git root, origin, branch and local work before changes. Preserve private credentials, .env files, uploads, recovery items, unrelated checkouts and the existing feature/v2-enhancements branch. Never print credential-bearing remote URLs or publish secrets. Use one producer-owned branch per work package and one coherent PR.

Normal scoped commits, pushes and PR publication are authorised by the current assignment. Keep workflow installation in a separate commit from application changes. No master merge, force push, history rewrite, branch deletion, remote changes, paid services or public deployment.

Inspect test setup and deletion behavior before execution. Use only newly created, uniquely named disposable PostgreSQL databases, MinIO buckets and volumes, synthetic accounts and files. Verify local connection targets and resource ownership before migrations, cleanup or deletion. Existing Docker Desktop may be started; use the local engine, dedicated resources and loopback ports. No privileged containers, host-wide cleanup, existing-volume reuse, Docker settings changes or system installation.

For code changes run go test ./..., go vet ./..., relevant race checks, backend and native builds, SHA-256 known vectors and a real upload/hash/signed-download flow including matching downloaded bytes and negative security cases. Exercise browser and container builds where available. Run supported dependency vulnerability checks. Do not mask failures, count skips as passes, or substitute mocks for full-stack acceptance. Record unavailable checks as blocked.

Use GHP-01 exact revision handovers. Publish results in the existing tracker/continuity and a complete PR description, without duplicate reports. Read back the remote branch, PR contents and relevant records before reporting CODEX DONE. Producer completion, independent Aether review, acknowledgement and integration remain distinct.
