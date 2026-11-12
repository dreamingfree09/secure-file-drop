# Next milestone: administrator-issued registration invitations

This is the separate next assignment after managed-link acceptance. It is a
proposal, not implemented account onboarding or authority to change live data.

Build an opt-in administrator-controlled registration mode without external
email delivery. The environment administrator explicitly issues a random,
single-use invitation with a short bounded expiry, optionally bound to a
normalised intended username/address. Store a one-way token digest with issuer,
expiry, use/revocation timestamps and an audit identity; never store or log the
bearer. Disclose the receipt once for out-of-band delivery. Rate-limit issuance
and redemption and retain existing same-origin protection.

Redeem by choosing ordinary-user credentials through HTTPS. Validate all input,
hash the password with the existing approved bcrypt settings, and atomically
consume the invitation with account creation. Concurrent redemption must have
one winner. Roll back both on failure; uncertain commit must be reconciled
without a second account or disclosed session. Reject expired, tampered,
revoked, reused and unknown invitations without leaking account existence.

Verification means possession of an administrator-issued capability, not proof
that an email inbox is owned. Record the verification method explicitly; do not
silently set `email_verified` or imply inbox verification. Email-dependent
features must remain unavailable without separately verified email ownership.
An invitation can grant only ordinary registration, never administrator status,
roles, elevated quota or access to another user's files. Preserve environment
administrator/database-user separation, login lockout and session revocation.

Use a dedicated worker PR and additive gated migration. Preserve default
registration compatibility unless its mode is explicitly changed. Verify
real PostgreSQL concurrent redemption, rollback/uncertain outcomes, existing
schema13/14/15 data, full authentication/session/owner isolation, restart and
paired restore. Test actual browser issuance/redemption with no SMTP provider.
Publish exact source/CI evidence and obtain independent Aether acceptance before
normal integration. No Railway deployment, live account migration or Rorschach
installation is included. Paid email, 50/100 GiB qualification and off-host
recovery remain separate gates.
