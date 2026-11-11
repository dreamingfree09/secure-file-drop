BEGIN;

-- Opt-in, owner-scoped revocable bearer ledger. Never store plaintext tokens.
CREATE TABLE managed_share_links (
    id UUID PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_sha256 TEXT NOT NULL UNIQUE CHECK (token_sha256 ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (expires_at > created_at)
);

CREATE INDEX managed_share_links_owner_created
    ON managed_share_links(owner_id, created_at DESC, id);
CREATE INDEX managed_share_links_file_active
    ON managed_share_links(file_id, expires_at) WHERE revoked_at IS NULL;

COMMIT;
