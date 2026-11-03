# Secure File Drop: Railway staging deployment (NOT YET LIVE)

Purpose: qualify one small, fresh, disposable first-user deployment without
touching the Owner's Windows Docker/WSL, local certificates, original data, backups,
PR #45 Stage 1 VM lab or Proxmox. PR #46 must pass fresh exact-head CI and
independent security review before real users or sensitive documents.

## Platform requirements

Use one Railway project with one backend service built from the reviewed PR
revision using `Dockerfile.backend` (the Dockerfile is not named `Dockerfile`,
so explicitly set `RAILWAY_DOCKERFILE_PATH=Dockerfile.backend`), one PostgreSQL
database and one **private** S3-compatible Railway Storage Bucket. Use separate
staging credentials and disposable files; do not copy existing data/secrets.
PostgreSQL uses the private `DATABASE_URL` variable. The bucket's credentials
supply `SFD_BUCKET`, `SFD_S3_ENDPOINT` (HTTPS), `SFD_S3_ACCESS_KEY` and
`SFD_S3_SECRET_KEY`. The app's strict `GetBucketPolicy` private-policy check
and bucket URL addressing MUST be validated against the actual provider before
claiming this integration works. Do not disable that security check merely
to reach readiness.

Set `SFD_ADDR=:8080`, `PORT=8080`, and a Railway healthcheck at
`/ready` (checks PostgreSQL, storage and the native SHA-256 utility).
Restrict all database credentials and S3 keys to the backend service. The
bucket and database should have no publicly exposed management endpoints.
Only the backend has a public HTTPS domain. Use Railway's TLS termination
and a canonical HTTPS `SFD_PUBLIC_BASE_URL`; never accept HTTP for real
credentials or file transfers.

The base backend requires `SFD_ADMIN_USER`, a bcrypt-hashed
`SFD_ADMIN_PASS`, `SFD_SESSION_SECRET` and `SFD_DOWNLOAD_SECRET` (at least
32 random bytes each). Do not reuse past local values or publish them.
For a bounded trial, set `SFD_MAX_UPLOAD_BYTES=10485760` and
`SFD_MAX_REQUEST_BYTES=11534336`. Use the application's reviewed
configuration and do not run old schema rollback or manual ledger edits.
Do not enable automatic deployment from an unmerged draft PR.

## Email from Railway Hobby

Outbound SMTP is disabled on Railway Free/Trial/Hobby. The app now supports
a reviewable HTTPS-only Resend API transport. Add
`SFD_EMAIL_PROVIDER=resend`, `SFD_EMAIL_FROM=no-reply@YOUR_VERIFIED_DOMAIN`
and `SFD_RESEND_API_KEY` as protected service variables. The Resend API
request is pinned to `https://api.resend.com/emails` and does not redirect.
Do not supply a key in chat, GitHub, log output, test fixture or code.
A verified sender domain and approved sender address are mandatory for
real recipient mail. SMTP stays the default for existing deployments.

Sending emails through HTTPS does not protect an invalid recipient address,
compromised mailbox or user device; the code is still sent only to the sender
in the authenticated HTTP response and passed separately.

## Release acceptance (fresh non-sensitive data only)

A successful public smoke test requires: distinct sender/recipient
test mailboxes; verification email; registration/login/logout;
bounded file metadata and upload; stored SHA-256 and byte-equal download;
description/recipient invitation; separate sender-only access code; recipient
wrong-code rejection; correct-code download; download exhaustion;
expiry/revocation and file deletion; failed mail delivery; clean reboot;
a complete database backup and S3 restore into an *independent staging
environment*; no original data or credentials modified.

Railway Buckets are S3-compatible and private, but their documentation
currently excludes object lock, bucket versioning, server-side encryption,
and bucket lifecycle policies. Do not advertise them as equivalent to an
independently qualified regulated/sensitive-data archive. Ordinary request
paths through backend→S3 count toward Railway service egress. Review
spending caps, rate limits and a separate backup before accepting traffic.

Operational gate: no live deployment or public DNS, real user email or
merge is authorised by writing this guide. Connecting Railway and setting
up a verified-domain email API and staging deployment requires the Owner's
explicit platform action/approval.

Source references:
- https://docs.railway.com/storage-buckets
- https://docs.railway.com/networking/outbound-networking
- https://docs.railway.com/builds/dockerfiles
- https://docs.railway.com/deployments/healthchecks
- https://resend.com/docs/api-reference/emails/send-email
