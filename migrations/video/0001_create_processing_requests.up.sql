-- user_id is an opaque identifier copied from the JWT claim at request
-- time, not a foreign key: Identity and Video Processing are separate
-- databases (see docs/adr/0007), so referential integrity across them is
-- enforced by the trust boundary (a valid signed token), not by Postgres.
CREATE TABLE processing_requests (
    id                 UUID PRIMARY KEY,
    user_id            UUID NOT NULL,
    original_filename  TEXT NOT NULL,
    format             TEXT NOT NULL,
    size_bytes         BIGINT NOT NULL,
    video_storage_key  TEXT NOT NULL,
    result_storage_key TEXT,
    status             TEXT NOT NULL,
    failure_reason     TEXT,
    attempts           SMALLINT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_processing_requests_user_id ON processing_requests (user_id, created_at DESC);
