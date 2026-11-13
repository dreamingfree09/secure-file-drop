BEGIN;

CREATE TABLE registration_invitations (
    id UUID PRIMARY KEY,
    token_sha256 TEXT NOT NULL UNIQUE CHECK (token_sha256 ~ '^[a-f0-9]{64}$'),
    issuer TEXT NOT NULL CHECK (length(issuer) BETWEEN 1 AND 256),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    redeemed_at TIMESTAMPTZ,
    redeemed_by UUID UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    CHECK (expires_at > created_at),
    CHECK ((redeemed_at IS NULL) = (redeemed_by IS NULL)),
    CHECK (revoked_at IS NULL OR redeemed_at IS NULL)
);
CREATE INDEX registration_invitations_issuer_created ON registration_invitations(issuer,created_at DESC,id DESC);

ALTER TABLE users ADD COLUMN registration_method TEXT NOT NULL DEFAULT 'email'
    CHECK (registration_method IN ('email','administrator_invitation'));
ALTER TABLE users ADD COLUMN registration_invitation_id UUID UNIQUE REFERENCES registration_invitations(id) ON DELETE RESTRICT;
ALTER TABLE users ADD CONSTRAINT registration_identity_method CHECK (
    (registration_method='email' AND registration_invitation_id IS NULL) OR
    (registration_method='administrator_invitation' AND registration_invitation_id IS NOT NULL)
);

CREATE TABLE registration_invitation_audit (
    invitation_id UUID NOT NULL REFERENCES registration_invitations(id) ON DELETE RESTRICT,
    event TEXT NOT NULL CHECK (event IN ('issued','revoked','redeemed')),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    issuer TEXT NOT NULL,
    account_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    PRIMARY KEY (invitation_id,event),
    CHECK ((event='redeemed') = (account_id IS NOT NULL))
);
COMMIT;
