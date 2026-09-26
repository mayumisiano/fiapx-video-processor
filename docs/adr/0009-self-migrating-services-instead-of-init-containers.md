# 0009. Self-migrating services instead of init containers

Status: accepted

## Context

`docker-compose.yml` had three one-shot jobs — `migrate-identity`, `migrate-video`, `createbuckets` — gated by `depends_on: condition: service_completed_successfully`. They exit `0` and stay in `docker compose ps` as `Exited`, which is correct behavior for a run-once job, but reads as dead/leftover containers in a demo and isn't the most idiomatic pattern for services that are each supposed to own their own data (`docs/adr/0007`).

## Decision

Each Go binary now bootstraps its own dependencies on startup instead of relying on a separate init container:

- `internal/platform/migrate` wraps `github.com/golang-migrate/migrate/v4` (the same `postgres` driver and database URL scheme the removed `migrate/migrate` CLI image already used, so no config format changed) against migrations embedded at compile time (`migrations/identity/embed.go`, `migrations/video/embed.go`, `//go:embed *.sql`). `identity-api` migrates its own DB; `video-api` and `video-worker` both migrate the shared video DB.
- `internal/videoprocessing/storage.Client.EnsureBuckets` checks/creates the `videos`/`results` MinIO buckets, called by both `video-api` and `video-worker`.
- `docker-compose.yml` drops `migrate-identity`, `migrate-video`, `createbuckets` entirely, along with the `service_completed_successfully` `depends_on` entries that referenced them.

Both operations are idempotent and safe under concurrent startup: golang-migrate's postgres driver takes a session-level Postgres advisory lock (so `video-api` and `video-worker` starting at the same time serialize instead of racing — the loser just sees `ErrNoChange`), and `EnsureBuckets` checks `BucketExists` before creating.

## Consequences

- `docker compose ps` shows only running services — no `Exited` rows to explain.
- A migration or bucket-creation failure now fails the owning service's own startup (`log.Fatalf`) instead of a separate job silently blocking dependents — arguably more correct: the service that needs the schema is the one that fails loudly if it can't get it.
- Does not change or supersede `docs/adr/0007` — the service/database split is untouched; this only changes how each service bootstraps its own schema and buckets.
- One more reason `video-api` and `video-worker` are each fully self-sufficient: neither depends on the other, or on a shared init step, to be ready.
