# Railway staging private-object attestation

Work ID: SFD-RAILWAY-POSITIVE-ATTESTATION-20261104. Implements [Issue50](https://github.com/dreamingfree09/secure-file-drop/issues/50), from master4db7fe592072e24e193a20b2cc9574f15297f47b. Exact final source, executed tests and independent review belong in the PR handover. No live success is claimed by this source document.

## Explicit scope and defaults

The existing empty or `bucket-policy` SFD_S3_PRIVACY_MODE retains the historical BucketExists/GetBucketPolicy/privateBucketPolicy path. It is not changed into an error-tolerant fallback. Unknown mode names fail. This new **opt-in** mode is `railway-positive-probe` and requires all of:

- SFD_ENV=staging;
- RAILWAY_PROJECT_ID=d8d1c061-84be-4302-91a2-e24e8ec1326a;
- RAILWAY_ENVIRONMENT_ID=85219452-c05b-487d-8cbe-7c3366697ca3;
- SFD_S3_ENDPOINT exactly https://t3.storageapi.dev;
- SFD_S3_BUCKET_LOOKUP=virtual and a single DNS-label bucket name from the provisioned bucket's BUCKET reference.

No HTTP, custom endpoint/port/path/query/userinfo, alternate environment, auto/path addressing, environment proxy, TLS bypass or redirects are accepted. Existing bucket-scoped credentials remain private Railway references. The IaC preserves the mode and environment; it never silently opts in. Railway injects project/environment IDs. No new keys, account or services are created by this correction.

## Startup transaction and preservation

Before backend readiness, a 15-second overall context performs:

1. Independently generate a 256-bit random object key under `_sfd_privacy_attestation/` and harmless random probe bytes. This namespace is separate from application uploads and database records.
2. Authenticated, single-part conditional PUT with If-None-Match:* and no-store. No public ACL/grant or bucket configuration is requested. A known precondition collision is a failure and never deletes the existing object.
3. Authenticated exact-byte GET and authenticated listing of the exact probe prefix, verifying its key and length. Missing, corrupt, oversized or unexpected results fail.
4. Fresh unsigned GET of **the same HTTPS host, bucket and exact key**, through an independent client with no credential provider, cookie jar, signed query, redirects or proxy. Only HTTP403 containing one complete, bounded S3 Error/AccessDenied XML document is accepted. Duplicate fields, conflicting key/bucket/resource, malformed/truncated/oversized bodies and read/close errors fail. Optional identity fields, when present, must match; request identity plus authenticated controls bind denials when the provider omits them. HTTP200,3xx,401,404,429,5xx, network/TLS errors or a generic Forbidden page are not privacy proof.
5. Both unsigned S3 list forms (V1 and V2, full bucket, max-keys1) must return the same explicit denial. A public listing is not excused because the sampled object is private.
6. Unsigned PUT of different harmless bytes and DELETE of that same new probe must also return explicit AccessDenied. Public write/delete grants fail. Re-read exact authenticated bytes afterward; a denial that secretly mutates or removes the object cannot manufacture success.
7. Always attempt bounded cleanup in a separate fresh 5-second context, including after an uncertain PUT. A successful PUT establishes probe ownership; if PUT failed, require a matching authenticated read before deletion, or an authenticated NoSuchKey404 to prove absence. Never remove foreign bytes. DELETE must succeed and authenticated HEAD must confirm NoSuchKey404. Any uncertainty or cleanup failure keeps startup failed; no blanket cleanup or automatic retries are performed.

Each startup owns only its random key. No shared marker, list-and-delete sweep, database migration for the probe, historical archive, pre-existing application object or credential change is involved. Concurrent starts cannot share a predictable name. Response bodies and temporary anonymous connections are closed; failed clients close idle connections. Provider exception text, bucket/key, credentials, URLs and payloads are never included in returned startup errors. Only fixed failure stages are logged. A failed cleanup can leave one harmless probe; preserve diagnostics and reconcile only its privately established identity, never delete by prefix.

## Security meaning and limits

This is positive data-plane evidence at startup, combined with the provider's documented private-bucket model. It does not parse an unsupported policy operation, accept its error, change an ACL, or prove every historical/future key is private. The sampled read/write/delete/list denials do not prove policy immutability, archival durability or full provider security. A compromised provider or later control-plane changes are outside this sample. This exception is restricted to the named isolated staging environment with harmless data. Requalification is mandatory before public exposure or changed storage configuration; general production/storage acceptance remains separate.

[Railway's bucket documentation](https://docs.railway.com/storage-buckets) describes private buckets without a supported public-bucket setting, virtual-host addressing and S3 object/list operations. Read-only connection metadata on4November2026 confirms this bucket's endpoint and virtual-host style. The existing deployment2a8942d5-076f-44df-9010-ddf4d805212f logs only `operation_unsupported`; no unsupported policy call is retried by the new mode. Backend/Postgres are in sfo and bucket in ams: assess latency, residency, recovery and egress before real data; do not silently move resources.

## Governed application and STOP conditions

1. Require exact-head full CI and independent Aether acceptance of the complete source. Keep master unchanged until acceptance. Do not commit pending Railway patch f193f9cc-77bb-4352-9353-7193afd587eb before verifying its actual non-secret mode value and all other staged changes. Current source4db7fe59 does not implement it.
2. After accepted normal integration, verify the actual merged source/build, preservation of Postgres deployment/volume/bucket, private database networking, all protected variables and no public domain. Pin the backend source to the accepted revision. Apply only the verified privacy mode and required unchanged staging scope; never assume an OAuth-redacted value matches.
3. Conduct the authorised backend-only harmless staging startup. Read back fixed attestation result and /ready. A single complete successful startup establishes same-object/list denial and confirmed cleanup. On any failure stop; do not weaken status/XML/TLS/cleanup checks or publish an endpoint to investigate.
4. Only after independent security acceptance and privacy success continue the [staging HTTPS/email/recipient runbook](RAILWAY_STAGING.md). Public domain, sender verification/Resend key, actual mailbox delivery, registration/login, separate code, exact bytes/hash, expiry/attempts, deletion/ledger and fresh-target backup/restore remain distinct gates. No real documents, existing-host changes or production declaration.

Agent owns engineering commands. Owner input is limited to indispensable private platform authorisation/domain/secret entry when required; never put credentials in chat. Rollback stops only the new staging backend and preserves its diagnostic/probe evidence. Do not reset PostgreSQL, downgrade schema, delete or move the bucket, change credentials, mount old images or alter original backups/CA/trust.

## Reproducible tests

`go test -race ./internal/server -run '^TestRailwayPrivacy' -count=1 -v` exercises five test methods with deterministic SDK fault cases, request boundaries,12 concurrent independent starts and real loopback HTTPS success/untrusted/wrong-hostname/expired negatives. Certificate trust exists only in disposable test clients; no Windows/root store changes. Full hosted acceptance retains all existing no-skip contracts and adds these five methods:254 to259 Go events. Backend124, browser27, portable17 and native Windows32/protocol16/policy6 remain unchanged. The focused workflow publishes formatting and test receipts, including on failure. Passing synthetic tests is not live Railway acceptance.


## XML document review correction

Review4467498559 found that the standard XML Decode call skips non-XML characters before the root. Explicit token validation now precedes DecodeElement. Only literal XML whitespace, valid comments and one initial XML1.0 UTF-8 declaration are permitted before Error; non-XML prefixes, character references or Unicode spaces outside the root, directives, arbitrary processing instructions and malformed/duplicate/late declarations fail. The suffix retains the same strict whitespace-only rule. Four legitimate prolog cases plus12 negatives expand the SDK matrix46 to62 without new Go test methods or weakened old assertions. The focused workflow requires a pinned139490c overlay failure and corrected-source pass. This is a source correction, not live Railway acceptance.
