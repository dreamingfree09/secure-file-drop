# Administrator-issued registration — schema16 candidate

This opt-in source is an implementation candidate, not live deployment or
independent acceptance. The canonical design is
[the separate R4 assignment](REGISTRATION_INVITATIONS_NEXT.md).

## Activation and preservation

`R1_REGISTRATION_INVITATIONS_ENABLED=true` passes
`SFD_REGISTRATION_INVITATIONS_ENABLED=true` through private Rorschach Compose.
It requires the already enabled schema15 managed-link/private-resumable gates
and an explicit canonical HTTPS origin. The default remains false. Default
schema13, resumable schema14 and managed schema15 migrations exclude migration16.
Existing16 refuses a configuration that would select an older runtime; the down
migration raises rather than discarding provenance or authorising a downgrade.
Use only an independently accepted source, verified private storage/capacity and
paired checkpoint/recovery procedure before any eventual real installation.

Migration16 adds a registration ledger, append-only-by-application audit rows
and explicit account registration provenance. Existing accounts retain their
credentials, roles, quotas, inbox-verification state and file ownership.
No existing account is silently reclassified or marked email-verified.

## Operator and recipient workflow

Only the signed **environment administrator** may issue or revoke invitations.
A database-backed administrator cannot impersonate this separate issuer.
The administrator's **Invite someone to Secure File Drop** panel lists status
and expiry, offers one-hour/24-hour invitations, and shows a private receipt
once for copying. Listing never returns a token, token digest or recoverable URL.
Confirmation is required before revocation; established accounts are not revoked
or deleted by revoking an invitation. Keep the out-of-band delivery private.

The link uses `/join#token=...`: browsers do not send its fragment in HTTP
requests. GET/HEAD serves generic, non-cacheable, no-referrer HTML with a strict
self-only content policy. The page removes the fragment from browser history,
never automatically checks or redeems it, and submits only after the recipient
explicitly chooses credentials. It has no external scripts, email or identity
provider. Refresh/navigation clears its in-memory authority: reopen the original
invitation if needed. No browser persistent storage holds invitation secrets.

Recipients choose an ASCII username (3–50 letters/digits/underscores), contact
email and a bcrypt-cost12 password (8–72 bytes containing letters and digits).
The environment administrator name is reserved. A successful registration
creates an ordinary account with an explicit10GiB quota and **no session**.
The recipient then signs in normally using their username and password.

## Verification and email boundary

`registration_method=administrator_invitation` records administrator-authorised
registration; `email_verified` remains false. Authentication/session resolution
checks active account, ordinary role and the matching redeemed ledger identity.
Unverified contact email is not a login alias. Old unverified email registrations
remain unable to log in. Deactivation, existing session/logout revocation and
ordinary file ownership checks remain effective.

No registration request can select role, quota, identity or verification flags;
unknown fields are rejected. Invited users can upload and create managed bearer
links. Existing protected code-invitation delivery still requires verified email
(including the current manual-delivery endpoint). Email-based password recovery
is unavailable in the accepted baseline. Upload/download/deletion notifications
are also withheld from unverified contacts in enabled R4 mode. An administrator
invitation is not proof of email ownership or an administrator account-recovery
tool. No new inbox-verification/recovery shortcut is introduced.

## API, atomicity and uncertainty

| Endpoint | Contract |
| --- | --- |
| `GET /admin/registration-invitations` | Issuer-scoped50-item metadata array; server state and dates; `before`/`X-Next-Cursor` pagination |
| `POST /admin/registration-invitations` | Environment issuer, strict HTTPS Origin, JSON `ttl_seconds`60–86400;201 one-time fragment URL |
| `DELETE /admin/registration-invitations/{id}` | Environment issuer, strict Origin;204 idempotent unused revoke;409 already redeemed;404 unavailable identity |
| `POST /registration-invitations/redeem` | Strict Origin; only `token`, `username`, `email`, `password`;201 ordinary account, no cookie |
| `GET/HEAD /join` | Generic page, no capability consumption or account metadata |
| `POST /register` when enabled |403 invitation required; legacy SMTP registration unchanged when disabled |

Capabilities contain32 cryptographically random bytes; only SHA-256 digests are
stored. Issuance has an issuer-serialised durable20/hour and100-active cap in
addition to existing request limits. Redemption uses the existing10/minute
authentication category. Requests have bounded JSON/body/SQL budgets. Neither
application errors nor audit entries contain passwords or plaintext tokens.

A row lock, ordinary-user INSERT, conditional expiry/revocation check, ledger
consumption and audit INSERT share one transaction. Concurrent redemption has
one successful account creation. Duplicate details or earlier failures roll back
without consuming the invitation. Password hashing occurs before the lock.
An unknown COMMIT returns503 `outcome_unconfirmed`; it never deletes a possibly
created account, restores a consumed invitation, retries automatically or issues
a session. Try signing in with the chosen credentials before reopening/retrying
the original invitation. A committed redemption remains unavailable on replay.

## Acceptance and remaining gates

The dedicated hosted workflow selects the exact candidate SHA and creates only
new labelled PostgreSQL/RustFS resources with normal validating HTTPS and a
disposable browser trust profile. It verifies schema13/14/15 preservation,
real browser issuance/registration/login/upload/download, race201/410, quota,
roles, ownership, expiry/revocation, SQL permission/deferred-COMMIT failure,
backend recreation and paired restore. A separate test-only reporting driver
uses real PostgreSQL transactions to report failure before commit and to lose
acknowledgement **after actual commit**; this is not an injected wire-level
PostgreSQL acknowledgement loss. It verifies no compensating deletion and real
runtime login after both reconciled outcomes. Source has no fault-injection hook.

Terminal successes/failures, exact source, artifact identities and all remaining
review gates belong in the producer PR. No local Docker/WSL/VM start, real
account/data/trust operation, Railway change, Rorschach installation or50/100GiB
acceptance is included. Independent Aether review remains mandatory; this
assignment does not authorise merging the new draft PR.
