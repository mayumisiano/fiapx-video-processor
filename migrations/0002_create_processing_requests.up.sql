CREATE TABLE processing_requests (
    id                 UUID PRIMARY KEY,
    user_id            UUID NOT NULL REFERENCES users(id),
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
