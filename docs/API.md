# API

Recipient delivery is an additive schema13 **draft**; it requires an authenticated sender, configured SMTP and an explicit HTTPS origin. The access code is never sent in email or a URL. The original signed-link download path remains a separate bearer-link mechanism.

JSON requests/responses are used except multipart uploads, file downloads and generic plain-text HTTP errors. Login, registration, metadata and link JSON bodies accept exactly one complete value within a total 16 KiB, including trailing whitespace. Oversized envelopes return 413; malformed or multiple values return 400. Ordinary authenticated requests require an active registered account and an unrevoked session. Administrative routes additionally require a signed administrator identity.

| Method and path | Authorization | Behavior |
| --- | --- | --- |
| POST /register | Public | email, username, password; ordinary account, 201; duplicate 409 |
| POST /login | Public | username or email and password; HttpOnly session cookie, JSON is_admin |
| POST /logout | Session | Revokes the current token hash, expires cookie, 204 |
| GET /me | Session | status and is_admin |
| POST /files | Session | orig_name, content_type, size_bytes; returns id/object_key/pending, 201 |
| GET /files | Session | Up to 100 of the current principal's files, newest first |
| POST /upload?id=UUID | Owner or administrator | multipart field file; synchronous store/hash, returns hashed |
| POST /links | Owner | id, ttl_seconds, optional share_page=true; legacy raw bearer URL by default or generic sharing-page URL when requested; **NOT code protected** |
| POST /invitations | Verified file owner | file_id, recipient_email, description?, ttl_hours?; emails protected invitation; returns one-time-displayed access_code |
| GET /receive?id=UUID | Public | Renders code-entry form; does not reveal file bytes |
| POST /receive-download | Public with UUID + code in POST body | Code-gated attachment; max eight incorrect attempts, three download reservations and finite expiry |
| GET /share?token=... | Bearer URL | Generic no-store interstitial; no file contents, original filenames or private metadata returned on GET. |
| POST /share-download | Signed bearer in URL-encoded form body | Explicit download with existing token, expiry, file-state and optional password checks. |
| GET /download?token=... | Valid bearer link | Legacy raw attachment bytes with optional single byte range. Expired 410, invalid/tampered 401, unsatisfiable range 416. |
| GET /admin/files | Administrator | Up to 100 files across owners |
| DELETE /admin/files/UUID | Administrator | Removes exact object key, then metadata, 204 |
| POST /admin/cleanup | Administrator | Removes old pending/failed records and objects; JSON deleted_count |
| GET /metrics | Administrator | JSON counters (uploads_total, downloads_total, login_success_total, etc.) |
| GET /health | Public | Process liveness |
| GET /ready | Public | PostgreSQL, bucket and native empty-vector readiness |
| GET /version | Public | Version/commit metadata |

Ordinary users receive 403 on administrative routes; inaccessible file IDs return 404 from upload/link endpoints. Reuploading a non-pending file returns 409. Metadata and actual file bytes must fit the configured limit (413); multipart overhead has a separate bounded allowance of 1 MiB. Exactly one file part named file with a filename and complete closing boundary is required; other fields/parts are rejected. Empty files are valid. MIME epilogue is permitted within the total envelope limit. Filenames are reduced to a basename, limited to 255 bytes, and reject controls. Declared size is replaced by the actual stored/hash size.

Registration preserves password bytes, requires letters and digits and accepts 8–72 bytes (bcrypt limit). Usernames are 3–50 ASCII letters/digits/underscore and cannot match the configured administrator name; emails are normalized. Registration does not grant administrator access.

Link TTL defaults to 300 seconds for missing/non-positive values and is capped at 86400; positive shorter TTLs are allowed. A token is expired at its expiry second. Configure SFD_PUBLIC_BASE_URL for production; untrusted forwarding headers are ignored when building fallback origins. No download token is logged by request middleware.

Signed bearer downloads with a storage validator advertise `Accept-Ranges: bytes` and a strong `ETag`. A valid single range returns 206 with the exact `Content-Range` and length. A matching `If-Range` resumes the representation. A stale validator, unsupported range unit or multiple ranges falls back to a complete response. Every resumed request still requires a valid link and the existing file authorization checks. This does not change the protected invitation download reservation policy.

The browser sends a same-origin Origin on state-changing requests; cross-origin values are rejected. Existing stateless sessions are deliberately invalidated when adopting migration 4. See [schema](DB_SCHEMA.md) and [validation](TRACKER.md).

A lost upload COMMIT acknowledgement returns a server error. A fresh row-lock reconciliation preserves hashed/ready bytes and all unknown outcomes; only an observed pending rollback permits compensation. A client error response alone does not prove that the database rolled back.

## Browser preview-safe sharing

Browser uploads explicitly request share_page=true and TTL 86400 seconds. Their copied links lead to the generic /share page; the POST /share-download form invokes the already-reviewed byte-stream handler internally and does not redirect the bearer into a new network URL. Third-party unfurlers fetching the sharing page do not receive file bytes or metadata. The legacy GET /download endpoint and clients omitting share_page remain unchanged. This is not a replacement for access-code protection: possession of an ordinary signed link is sufficient for access until expiry. Per-link revocation and more flexible expiry policies remain future work.

### HTTPS form Origin and referrer policy

The generic share page deliberately uses an origin-only referrer policy (`strict-origin`), not `no-referrer`: the Fetch standard makes ordinary HTML form POST requests report `Origin: null` under `no-referrer`, which would be rejected by Secure File Drop's existing same-origin middleware. `strict-origin` sends only the scheme/host/port on HTTPS-to-HTTPS requests, **not the sensitive token query or URL path**. The self-hosted Caddy origin uses the same policy. The server still rejects unrelated/noncanonical origins; there is no bypass or CSRF relaxation. The signed bearer is submitted in the bounded POST body only.
