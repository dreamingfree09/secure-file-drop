# Rorschach managed sharing — schema 15 (opt-in candidate)

The existing signed-link implementation remains unchanged on the default
schema 13 (Railway) and schema 14 (Rorschach resumable staging). This new
feature is **disabled unless explicitly opted in** on a separately owned,
qualified Rorschach VM.

## Configuration and database gates

Set both Rorschach resumable capacity variables to independently verified
positive values and use the guarded secret-file switch:

`R1_MANAGED_LINKS_ENABLED=true`

The private Compose environment passes it to `SFD_MANAGED_LINKS_ENABLED`.
Startup selects the additive migration 15 only when the resumable environment
is also qualified. The default is false. The application refuses to start on
schema 15 if the managed-link feature is subsequently disabled, avoiding a
silent loss of revocation checks. Rollback requires an independently verified
paired checkpoint/restore. **Do not apply the schema to Railway staging.**

Schema 15 stores file and owner UUIDs, an unpredictable per-link UUID, expiry,
revocation time and **SHA-256 of the signed bearer**, never its plaintext token.
The underlying HMAC token now has an optional signed link ID. Legacy signed
tokens omit this field and are byte-for-byte compatible with the old format.

## HTTP API (activated only in schema 15)

`POST /links` remains the existing owner-only endpoint. When schema 15 is
enabled it issues a durable managed link and adds `link_id` to the response,
alongside the existing `url` and `expires_at`. The `share_page: true`
option still selects the generic preview-safe HTML landing page. Clients
omitting it still receive a raw signed-download URL.

`GET /managed-links?file_id=UUID` requires an authenticated ordinary owner.
It returns at most 50 owner-scoped link metadata records, newest first,
including creation/expiry/revocation timestamps; no token digests or bearer URLs.
The browser candidate adds `file_name` and server-evaluated `state`
(`active`, `expired`, `revoked`). `X-Next-Cursor` indicates an older page;
pass its canonical UUID as `before`, retaining the optional file filter.
Ordering is descending creation time and descending UUID. The anchor is
owner-scoped too; foreign or missing anchors yield an empty page.

`DELETE /managed-links/{link_id}` requires the original owning account,
returns HTTP 204 and is idempotent for the same owner. Inaccessible or
cross-account link IDs return 404.

Every subsequent new `GET /download`, including byte-range resumptions,
checks the durable record. A revoked, missing or expired managed link returns
410 before opening private object storage; a backend/database failure returns
503 rather than a download. A token with a managed-link identity cannot be
treated as legacy after disabling the feature.

**Ongoing downloads:** a download already streaming when its link is revoked
may finish. Revocation prevents *new* whole-object and HTTP Range requests.
This boundary must be visible in the final browser management UI.

## Compatibility, privacy and remaining work

Legacy stateless links created before the schema15 upgrade remain valid until
their original cryptographic expiry (maximum 24 hours), and cannot be
individually revoked retroactively; this limitation is explicit.

The browser candidate adds **Your sharing links** for authenticated ordinary
users only when `/config` explicitly enables managed links. Choose a file to
create a one-hour preview-safe link, copy its one-time receipt, browse older
metadata, and confirm individual revocation in the keyboard-accessible dialog.
The list cannot recover old bearer URLs. It displays server-reported state,
refreshes after successful mutations and checks expiry again every minute.
Failed verification removes stale cards and shows a retry message. Logout,
expired sessions and account changes clear names, choices, receipt and dialog.
Nothing is stored in browser persistent storage. A clipboard copy already made
by the user is outside the application's session state; revoke its link to
invalidate subsequent use. Protected code invitations remain a separate flow.

Independent browser-source acceptance, retention/cleanup, production
off-host restore and further deployment review remain outstanding. The
disposable HTTPS fixture validates the source, including cross-account denial,
preserved legacy tokens, active/new link independence, revocation across backend
restart and paired database/object restore. A passing fixture is **not** proof
of the required 50 GiB/100 GiB real transfers or authorised Rorschach install.

Railway, live Rorschach resources, pre-existing files, passwords, tokens and
personal data must not be migrated or altered by source acceptance alone.
