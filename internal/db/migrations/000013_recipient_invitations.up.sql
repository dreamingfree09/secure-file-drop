-- Additive invitation records, without altering established file/owner metadata.
CREATE TABLE file_invitations (
    id UUID PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_email VARCHAR(254) NOT NULL,
    description TEXT NOT NULL DEFAULT '' CHECK (octet_length(description) <= 1000),
    code_digest VARCHAR(64) NOT NULL CHECK (code_digest ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    failed_attempts SMALLINT NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 8),
    downloads SMALLINT NOT NULL DEFAULT 0 CHECK (downloads BETWEEN 0 AND 3),
    CONSTRAINT file_invitations_future_expiry CHECK (expires_at > created_at)
);
CREATE INDEX file_invitations_sender_created ON file_invitations(sender_id, created_at DESC);
CREATE INDEX file_invitations_expiry ON file_invitations(expires_at);

-- Hourly account-wide delivery quota survives deletion of the related file.
-- This table has no foreign key to file_invitations or files: deleting a file
-- invalidates access, but cannot reset already accepted SMTP usage.
CREATE TABLE invitation_send_ledger (
    id UUID PRIMARY KEY,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX invitation_send_ledger_sender_created
    ON invitation_send_ledger(sender_id, created_at DESC);
