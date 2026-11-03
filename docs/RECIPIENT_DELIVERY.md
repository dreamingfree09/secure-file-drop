# Recipient delivery — separate access code (development draft)

## Workflow
Sign up using a real email address, verify it through configured SMTP, and sign in. Upload a file with the web interface and wait for a hashed/ready result. Under **Send a protected file invitation**, select that file, enter a destination email, an optional description (maximum 1000 UTF-8 bytes) and one-, 24- or 72-hour expiry. The recipient receives a link to the code-entry page; the sender sees a new access code **once**, after SMTP acceptance and database commit. Give the code to the intended recipient **separately**, preferably by telephone. There is no code-recovery endpoint.

The invitation is a UUID link plus an independent 80-bit OS-random Base32 code. Only an HMAC-SHA256 digest of the code bound to the UUID is stored in the additive `file_invitations` table. The code is not included in an email, URL, server log or persisted browser storage. Do not share screenshots or copy the code to an untrusted clipboard. The recipient submits the code by POST; after eight incorrect attempts or three reserved downloads, the invitation can no longer download. The invitation and underlying file expiry are checked. A download slot is consumed even if the network connection subsequently fails.

The sender must own the verified file, must have an active verified account, and is limited to twelve invitations per hour. A further per-IP rate limit applies. The invitation email includes an HTML-escaped sender address, filename and description. The new transfer requires a configured `SFD_PUBLIC_BASE_URL` beginning with `https://`, a certificate trusted by recipient devices, functioning SMTP, and a download secret containing at least 32 bytes. `https://localhost:8443` is for local fixtures only; it will **not** reach another person's machine.

## API
`POST /invitations`: authenticated JSON `{"file_id":"UUID","recipient_email":"name@example.org","description":"Optional message","ttl_hours":24}`. HTTP 201 includes `access_code` (sender only), `expires_at`, `downloads_allowed`. This response is marked no-store.

`GET /receive?id=UUID`: public HTML code-entry page, with no file bytes and no code. `POST /receive-download`: bounded `application/x-www-form-urlencoded` body with exactly `id` and `code`; binary attachment only after database-locked code/expiry/status/download-count verification. The code never travels as a query parameter.

The pre-existing `POST /links` and `GET /download` endpoints are **bearer-link sharing** and do not require invitation codes. The sender must not send those links when claiming a recipient-code requirement.

## Limitations and release gates
SMTP acceptance does not prove inbox delivery. SMTP and PostgreSQL are not one distributed transaction; an email accepted before a failed commit may contain an unusable locator. The recipient is **not** required to register or prove ownership of their mailbox; anyone given both locator and code can download within the limits. This is not end-to-end encryption, malware scanning, or a guarantee against compromised mailboxes, browsers, operating systems or servers.

Migration 13 adds a separate table and indexes; no existing files or users are rewritten. Its down migration destroys **invitation metadata only**, and is not approved on real systems. Do not alter live schema lineage, Docker/WSL, storage, trust stores, CA or archives merely to test this branch. An operator must verify disposable schema11/12→13 migration, backup/restore, both SHA256 layouts, HTTPS, SMTP delivery, wrong recipient/owner, bad code, expired links, parallel download limits, deleted file, browser UI, and mail failure before deployment. The unsigned Windows tray companion still offers Quick Send/manual bearer links but has no new delivery form. The **web UI** is the first sender client.

**Status: draft implementation, not production readiness.** Existing RustFS image-security findings, real TLS/CRL/client checks, disaster recovery and packaging remain outstanding.
