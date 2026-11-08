BEGIN;

CREATE TABLE r2_uploads (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    request_id UUID NOT NULL,
    object_key TEXT NOT NULL UNIQUE CHECK (object_key = 'resumable/' || id::text),
    orig_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    total_bytes BIGINT NOT NULL CHECK (total_bytes BETWEEN 1 AND 107374182400),
    manifest TEXT NOT NULL CHECK (manifest ~ '^[0-9a-f]{64}$'),
    offset_bytes BIGINT NOT NULL DEFAULT 0 CHECK (offset_bytes >= 0 AND offset_bytes <= total_bytes),
    upload_id TEXT CHECK (length(upload_id) BETWEEN 1 AND 2048),
    state TEXT NOT NULL DEFAULT 'new' CHECK (state IN ('new','open','finalizing','complete','canceling','canceled','quarantined')),
    digest TEXT CHECK (digest ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '48 hours',
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, request_id),
    CHECK (state NOT IN ('open','finalizing','complete','quarantined') OR upload_id IS NOT NULL),
    CHECK (state NOT IN ('finalizing','complete','quarantined') OR offset_bytes = total_bytes),
    CHECK ((state = 'complete') = (digest IS NOT NULL))
);

CREATE INDEX r2_uploads_work ON r2_uploads (next_attempt, expires_at) WHERE state IN ('new','open','finalizing','canceling');
CREATE INDEX r2_uploads_reservations ON r2_uploads (owner_id) WHERE state IN ('new','open','finalizing','canceling','quarantined');
CREATE INDEX r2_uploads_owner_created ON r2_uploads (owner_id, created_at);

CREATE TABLE r2_parts (
    upload UUID NOT NULL REFERENCES r2_uploads(id) ON DELETE CASCADE,
    number INTEGER NOT NULL CHECK (number BETWEEN 1 AND 3200),
    start_byte BIGINT NOT NULL CHECK (start_byte = (number::bigint - 1) * 33554432),
    byte_count BIGINT NOT NULL CHECK (byte_count BETWEEN 1 AND 33554432),
    digest TEXT NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
    etag TEXT CHECK (length(etag) BETWEEN 1 AND 128),
    PRIMARY KEY (upload, number)
);

COMMIT;
