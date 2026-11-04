# Railway staging: source ready, live provider acceptance pending

Work ID `SFD-PR46-RELEASE-CORRECTIONS-20261104`. Only a fresh disposable project
is in scope. No existing Docker/WSL/VM, PKI, trust, archives, credentials or real
data is an input. PR46 remains unmerged; exact source, CI and independent review
belong in its handover. Nothing in this guide authorises deployment or real mail.

## Deployment files and scope

Use [.railway/railway.ts](../.railway/railway.ts), the supported equivalent of
railway.json. Railway now deprecates the older config format and limits its
continued use to legacy services until 1 December2026. This partial manages only
`backend` in project `secure-file-drop-staging`, environment `staging`; other
contexts throw. It deliberately leaves database/bucket/domain and source binding
to the separately authorised account context. It does not install an app or
connect GitHub. Keep automatic deployment off for this unmerged branch.
Agent-operated CLI planning/application requires Railway CLI5.63+ and the
Railway TypeScript SDK3.12.0; no Owner terminal commands are required. The
committed .railway/package.json and lockfile pin it. The authorised agent uses
`npm ci --prefix .railway --ignore-scripts` in its disposable execution checkout
before planning; this preparation generated only the lockfile and installed no
packages on the Owner host. Use the
normal `railway config plan` readback against that exact staging scope, inspect
its diff and apply only after separately authorised platform access. No plan or
apply against a real account was executed here. The SDK contract tests evaluate
only a pure graph with external IO denied and its published source integrity
checked; they do not prove the live control-plane result.

The file selects `Dockerfile.backend` with the DOCKERFILE builder. Its existing
entrypoint remains `/app/backend`. Both `PORT=8080` and `SFD_ADDR=:8080` are set,
so the backend listens on all interfaces at the same port as the provider check.
The public domain target port must also be8080. `/ready` is the deployment check,
with a300-second timeout; it is not continuous monitoring. The25-second
provider draining time preserves the existing20-second application shutdown
budget before forced termination. One replica avoids
inventing untested shared-instance semantics. File cap10485760 and request
envelope11534336 give a10MiB trial. File cleanup remains disabled.

## Protected variables: no source edits or existing secrets

In the new Railway environment, provision fresh private PostgreSQL and a fresh
private S3 bucket before starting the backend. The authorised agent can map the
following service variables using Railway's protected UI/references. The IaC
preserves their existing private values; missing values cannot be invented by
code. Never put keys, password hashes, cookies, codes or invitation URLs in chat,
source, CI logs or public evidence.

| Backend variable | Staging value/source |
| --- | --- |
| DATABASE_URL | Private PostgreSQL connection reference, never its public proxy |
| SFD_BUCKET | Bucket's BUCKET value, not its display name |
| SFD_S3_ENDPOINT | Bucket HTTPS base endpoint without bucket/path |
| SFD_S3_ACCESS_KEY / SFD_S3_SECRET_KEY | Fresh bucket-scoped credentials |
| SFD_S3_BUCKET_LOOKUP | virtual for a new Railway bucket; already selected in IaC |
| SFD_PUBLIC_BASE_URL | Exact public HTTPS origin, without path/query/credentials |
| SFD_ADMIN_USER / SFD_ADMIN_PASS | New staging administrator name and bcrypt hash |
| SFD_SESSION_SECRET / SFD_DOWNLOAD_SECRET | Independently generated strong staging secrets, at least32 random bytes |
| SFD_EMAIL_PROVIDER | resend, already selected in IaC |
| SFD_EMAIL_FROM / SFD_RESEND_API_KEY | Verified sender address and a sending API key, protected in Railway |

Preserve SMTP as the application default/alternative; this staging partial
explicitly selects Resend HTTPS because Railway restricts SMTP on its lower
plans. Production transport uses https://api.resend.com/emails, normal certificate
validation, no redirects and no fallback. A provider acceptance ID does not prove
mailbox delivery; the smoke test checks the actual test inbox. Domain/account
authorisation is external; code does not create it or accept paid terms.

## Railway-only positive bucket privacy attestation

**RAILWAY-BUCKET-POLICY: the provider's authenticated GetBucketPolicy operation is
not accepted as proof of privacy when its response is unsupported.**
The existing `privateBucketPolicy` function and default strict policy mode
remain unchanged. A separate opt-in `SFD_S3_PRIVACY_MODE=railway-staging-probe`
is permitted ONLY for Railway project `secure-file-drop-staging`, environment
`staging`, `SFD_ENV=staging`, the exact HTTPS base endpoint
`https://t3.storageapi.dev`, virtual-hosted lookup and a bucket name beginning
`sfd-private-staging-`. No other host, bucket, production project or plain HTTP
is accepted; an unknown mode fails configuration validation.

Under the opt-in, a policy query that succeeds with anonymous-allow access
still causes startup rejection. Only documented-style `AccessDenied`,
`NotImplemented` or `MethodNotAllowed` errors permit the alternative check.
The app generates an unpredictable unique key and random harmless bytes,
requires authenticated upload and byte-equal download, then issues an entirely
unauthenticated, unsigned HTTPS GET for exactly the same object. It accepts
ONLY HTTP 403 containing a well-formed S3 XML `AccessDenied` error, never
a redirect, successful response, 404, malformed error or TLS failure.
Every attempt must delete the probe using the authenticated API; cleanup
failure also prevents startup. All operations are time bounded, and neither
object URL, object key nor credential enters application logs. Fixture tests
simulate both successful and adverse outcomes without claiming real Railway
storage acceptance.

The existing real staging service previously failed before this new code
existed. Set the opt-in only **after the corrected branch passes independent
review and full exact-head CI**. Redeploy only the already-authorised
disposable staging backend. Verify `/ready` passes on its actual fresh bucket
with the strict dynamic proof, and separately verify anonymous reads of the
probe object are denied. Do not claim a result without observing it. This is
a staging compatibility exception, NOT production storage approval. The
provider currently does not provide bucket versioning, object lock or bucket
lifecycle configuration, and does not provide automatic bucket backups.
Railway documents encryption at rest, but this is not end-to-end encryption,
an archival guarantee or an independently reviewed multi-region recovery design.

## Agent-operated live smoke runbook (not executed)

Prerequisites: exact reviewed/CI-passing revision, separately authorised fresh
Railway staging context, the protected variables above, verified sender domain
and two harmless mailboxes. Only the backend receives a public HTTPS domain;
keep PostgreSQL management/private connection data out of public networking.

1. Select the exact accepted commit/build with automatic deployment disabled.
   Read back the IaC plan, Dockerfile, port8080,10MiB cap and /ready. Do not modify
   existing deployments or use a historical recovery image to downgrade schema.
2. Close RAILWAY-BUCKET-POLICY as above. Require strict HTTPS /ready200 and the
   fresh database at additive schema13. Do not send credentials before TLS and
   readiness succeed. No migrations of original records or manual schema edits.
3. Register one ordinary sender through the browser and its real verification
   email. Log in and read identity/quota; the administrator is a distinct role.
   Use only generated staging credentials in the browser/private platform UI.
4. Upload one harmless known-byte file smaller than10MiB. Record its local and
   backend SHA256 and exact byte length. Also verify an over-limit upload fails
   without creating a completed object/record.
5. Choose that file, recipient mailbox and a harmless description; send once.
   Confirm one invitation email with the HTTPS locator and no access code. The
   sender sees its independent code once and conveys it using a separate channel.
   Keep the code and locator out of logs/screenshots/public evidence.
6. Recipient opens the locator. Wrong code must return401; then correct code
   downloads exact bytes/SHA256. Use an ordinary gzip-capable browser, confirm
   uncompressed attachment/Content-Length, and permit exactly three successful
   reservations; the next returns410. Interrupted transfers consume reservations
   by design. Never refresh a failed transfer into unlimited downloads.
7. With a separate invitation, exercise the supported one-hour expiry without
   changing database clocks/records; after expiry expect410. Exercise the eight
   wrong-attempt limit on another invitation with a paced test schedule so the
   independent IP limiter is not mistaken for code lockout. Verify deletion of
   the owned harmless file revokes its invitations and leaves other test files
   intact. The hourly sender ledger must survive deletion.
8. Log out during a delayed list/invitation, sign in as another synthetic account
   and verify no filenames, IDs, code or disabled Send state crosses accounts.
   Log out and verify old sessions fail. Do not infer this visual result from CI.
9. Agent tests a deliberately rejected email in a separate disposable context
   without exposing provider bodies/keys; it must not return an acknowledged
   invitation/code. Do not rotate or damage the working account key just to test.
10. Quiesce the new staging service; export its PostgreSQL plus object set and
    hashes and restore to a second fresh disposable target. Require rows,
    ownership and exact bytes to match before accepting anything beyond a trial.
    Railway ephemeral container files are not durable backups. Automatic backups default to disabled; the canonical retention default is
    seven days, not the original installation's separately approved30-day override.
    Before enabling them, set SFD_BACKUP_INTERVAL=24h and
    SFD_BACKUP_RETENTION_DAYS=30 in this new service, provision a persistent staging
    destination and prove restore. No original backups or settings are reused.

Record only revision, CI/run IDs, statuses/counts and harmless file digests.
Stop on readiness/storage-privacy/TLS/identity/byte-integrity/restore failure.
Rollback stops only the new staging backend and preserves its evidence; no
schema downgrade, destructive cleanup, original-host operation or automatic
merge. Actual account connection, private values and real harmless-mail trial
remain external execution prerequisites. This is not public production readiness.

## Authoritative references (checked4November2026)

- [Current IaC API](https://docs.railway.com/infrastructure-as-code/reference)
- [Legacy config deprecation](https://docs.railway.com/config-as-code/reference)
- [Dockerfile selection](https://docs.railway.com/builds/dockerfiles)
- [Readiness and port](https://docs.railway.com/deployments/healthchecks)
- [Private buckets and limitations](https://docs.railway.com/storage-buckets)
- [Outbound mail networking](https://docs.railway.com/networking/outbound-networking)
- [Resend acceptance API](https://resend.com/docs/api-reference/emails/send-email)
