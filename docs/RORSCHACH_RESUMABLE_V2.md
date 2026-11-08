# Rorschach R2 — resumable S3 part primitive (not yet active)

This is the first **implemented source prerequisite** for a new, secure, durable
100 GiB upload path. It does not enable the inherited TUS handler or change the
10 MiB staging cap. The live `/upload/resumable` endpoint remains HTTP 501.

The multipart primitive in `internal/server/resumable_v2_parts.go` caps each file at
**100 GiB** (107374182400 bytes) as its hard upper bound and
**exactly 32 MiB** (33554432 bytes) per non-final part. A final part may be
shorter. 100 GiB therefore requires 3200 consecutive S3 parts, below the
standard 10000-part limit. Each part's declared offset and body length must
match a predictable plan; no sparse offsets, unbounded chunks, duplicate or
missing completion records are accepted. The environment-configured file
maximum still applies and may be lower than the hard bound.

`internal/server/resumable_v2_s3.go` now contains usable private S3
operations for initiate, transfer a streamed part with a SHA-256 body check,
complete a fully validated ordered part list, and abort **only** an
independently confirmed owned multipart session. No private S3 endpoint or
presigned transfer URL is sent to browsers. Completion verifies the actual S3
object size independently of the completion acknowledgement. A browser's own
reported digest is *not* proof of the final object's native SHA-256.

**Not yet implemented or exposed:** authenticated, durable DB upload-session
leases; PostgreSQL part-number/ETag/digest ledger and ownership checks; request
timeout, exact Content-Length and per-part rate limiting; resumable browser
client; restart and storage-failure recovery; owned stale-session cleanup;
final native-C SHA-256 transition and unknown-COMMIT reconciliation; actual
1/10/50/100 GiB transfer acceptance. These MUST be added and tested before
enabling public upload endpoints or changing the 10 MiB production/staging
envelope. Merely merging this primitive does **not** constitute completion.

Implementation sequence: persist an upload session with a unique owner+file
reservation before initiating S3; record S3 upload ID securely; serialize
writes to one chunk offset under a session lock; write S3 part with exact
checksum; record its ETag/checksum/offset transactionally; report the committed
offset only; finalize after verifying all numbered parts and owner; stream
native-C hash from the completed object and commit the file state; reconcile
any uncertain completion with a fresh DB lock and S3 stat, never delete object
bytes merely because a response failed. All S3 identities stay private.
