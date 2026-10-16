# API

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
| POST /links | Owner or administrator | id, ttl_seconds; returns url/expires_at for hashed or ready files |
| GET /download?token=... | Valid bearer link | Attachment bytes; expired 410, invalid/tampered 401 |
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

The browser sends a same-origin Origin on state-changing requests; cross-origin values are rejected. Existing stateless sessions are deliberately invalidated when adopting migration 4. See [schema](DB_SCHEMA.md) and [validation](TRACKER.md).

A lost upload COMMIT acknowledgement returns a server error. A fresh row-lock reconciliation preserves hashed/ready bytes and all unknown outcomes; only an observed pending rollback permits compensation. A client error response alone does not prove that the database rolled back.
