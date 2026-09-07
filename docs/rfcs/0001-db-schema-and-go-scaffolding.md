# RFC-0001: PostgreSQL Schema & Go Repository Scaffolding

> Status: accepted (2026-09-07). Translates already-approved decisions from `docs/domain-modeling.md`, `docs/core-domain.md`, `docs/technical-architecture.md`, and `docs/api-contract.md` into a concrete database schema and Go module layout. This is the last open item before backend implementation can start (see "Suggested next steps" in `docs/domain-modeling.md`).

## 1. PostgreSQL schema

Two tables, matching the `User` (Identity & Access) and `ProcessingRequest` (Video Processing core aggregate) concepts.

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
```

Decisions:
- `status` and `failure_reason` are `TEXT`, not native Postgres `ENUM` types — the state machine and the failure-reason enum (`docs/core-domain.md`, `docs/domain-modeling.md` §7.5) are validated in the Go domain layer, not the database. A native enum type is painful to alter later (`ALTER TYPE ... ADD VALUE` has restrictions); keeping it as `TEXT` with Go-side validation keeps the single source of truth for the state machine in the aggregate, per the "not just a jobs table with a status column" principle in `core-domain.md`.
- No separate table for `VideoMetadata` / `ProcessingResult` value objects — they're embedded as columns on `processing_requests` since they have no independent lifecycle or identity (textbook value object treatment).
- Migration tool: **`golang-migrate`**, versioned `.sql` files under `migrations/`.

## 2. Go repository layout

```
cmd/
  api/main.go
  worker/main.go
internal/
  identity/
    domain/
    postgres/
    http/
    jwt/
  videoprocessing/
    domain/
    postgres/
    storage/
    queue/
    http/
    ffmpeg/
  notification/
    domain/
    smtp/
    consumer/
  platform/
    config/
    postgres/
    rabbitmq/
    logging/
migrations/
```

Decisions:
- Each bounded context (`identity`, `videoprocessing`, `notification`) is a top-level `internal/` package, mirroring `docs/bounded-contexts.md`.
- Each context's `domain/` subpackage holds the aggregate/entities and defines the ports (Go interfaces) it needs (`Repository`, `Publisher`, etc.) — no I/O, no framework imports. This is where `ProcessingRequest`'s invariants and state-machine methods live (`StartProcessing()`, `RecordFailure(reason)`, `RetryProcessing()`), per `core-domain.md`.
- Sibling subpackages (`postgres/`, `storage/`, `queue/`, `smtp/`, `ffmpeg/`) are adapters implementing those ports.
- `cmd/api` and `cmd/worker` only wire concrete adapters into use cases — no business logic.
- `internal/platform/` holds cross-cutting infrastructure (DB connection pool, RabbitMQ connection, structured JSON logger, config loading) shared by both binaries.

## 3. Scope of this change

Delivers the schema + migrations + empty (but wired) package skeleton. Does not yet implement any business logic (that's the next phase: Identity context first, then Video Processing, then Notification, per the roadmap already discussed in chat).
