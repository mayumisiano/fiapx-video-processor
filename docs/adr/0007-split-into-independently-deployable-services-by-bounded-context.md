# 0007. Split into independently deployable services, by bounded context

Status: accepted (supersedes 0001)

## Context

0001 accepted a modular monolith (`api` + `worker`, split by sync/async, not by bounded context). Reviewing that decision against a likely criticism — "why isn't each context its own service?" — surfaced a distinction worth making explicit: monorepo vs. multi-repo is a code-organization choice, orthogonal to monolith vs. microservices, which is a runtime-architecture choice (independent deploy, independent scaling, data ownership). "Put it in a separate repo" would not have fixed anything real.

What actually violated microservices boundaries was found in code, not in repo layout:

1. `identity` and `videoprocessing` shared one Postgres pool.
2. `migrations/0002_create_processing_requests.up.sql` had `user_id UUID NOT NULL REFERENCES users(id)` — a foreign key across what should be two independent datastores.
3. `cmd/worker/main.go` read the `users` table directly (`identitypostgres.NewRepository(pool)`) just to fetch an email for the outcome notification.

These are concrete, fixable violations of "each service owns its data" — the actual bar for calling something a microservice, independent of where the code lives.

## Decision

Split into three independently deployable binaries, one per bounded context that has a runtime footprint:

- **`identity-api`**: registration, login, JWT issuance. Owns a dedicated Postgres database (`fiapx_identity`).
- **`video-api`** + **`video-worker`**: upload, status, download, retry, and async processing. Owns a dedicated Postgres database (`fiapx_video`). Two binaries because they still have different scaling needs (sync HTTP vs. queue consumer, as in 0001) — the split is now two-dimensional: by bounded context, and by sync/async within Video Processing.
- **Notification** stays an in-process library inside `video-worker`, per 0006 — not revisited here.

To make this possible without breaking the two properties worth keeping from 0001:

- **JWT verification uses a shared secret, not a shared database.** `internal/platform/jwt.Verify` is pure crypto (no I/O); `video-api` validates a token issued by `identity-api` without ever calling it over the network. Sharing a signing secret across services is standard practice — it's a trust-boundary key, not a data dependency. Identity now only issues tokens (`domain.TokenIssuer.Issue`); it dropped `Parse`, since nothing in identity ever needed to validate its own tokens.
- **The worker no longer needs Identity's data at all.** `ProcessingRequest` now carries `UserEmail`, copied from the JWT claim at upload time (`docs/bounded-contexts.md`'s documented Identity → Video Processing Conformist relationship: receive the trusted claim once, never ask again). The FK was dropped; `user_id` is now an opaque identifier trusted from the token, not enforced by Postgres.

## Consequences

- `video-worker` has zero imports under `internal/identity/...` — verified by `go build`/`grep` after the change.
- Two databases to run and migrate locally instead of one (`migrations/identity/`, `migrations/video/`, two `migrate` jobs in `docker-compose.yml`, one Postgres container each).
- No API gateway yet: the frontend now talks to two base URLs (`identity-api` on `:8081`, `video-api` on `:8080`). Logged here as a follow-up, not done in this change — acceptable for the hackathon's scope, revisit if a unified public entrypoint becomes necessary.
- Extracting `identity-api` from `video-api`/`video-worker` further (e.g. to a different repo, or a different language) is now a deployment/ops decision, not a code refactor — the data and network boundary is already real.
- The "monorepo vs. microservices" framing this ADR started from stays available as a direct rebuttal if raised again: nothing here required a repo split, and nothing about a repo split would have achieved what this ADR actually did.
