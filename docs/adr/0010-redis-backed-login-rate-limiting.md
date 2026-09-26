# 0010. Redis-backed login rate limiting

Status: accepted

## Context

`redis` was provisioned in `docker-compose.yml` from the start (the stack recommendation names Postgres + Redis) but never wired into any Go code — `identity-api`'s `/auth/login` had no protection against credential-stuffing or brute-force password guessing. A recommended-but-unused container is also, on its own terms, the same kind of visual/operational noise as the init containers removed in `docs/adr/0009`: infrastructure with no job to point to.

Of the candidate uses for Redis in this system (read cache for status listing, upload idempotency, login rate limiting), rate limiting was chosen: it is the only one that closes an actual gap in the "protected by username and password" functional requirement, rather than optimizing something that already works correctly.

## Decision

- `internal/identity/ratelimit`: `Limiter.Allow(ctx, key)` implements a fixed-window counter (5 attempts per client IP per minute, `cmd/identity-api/main.go`) against a small `Store` port (`Increment(ctx, key, window) (int64, error)`), keeping the counting logic unit-testable without a live Redis.
- `internal/identity/ratelimit.RedisStore` is the real adapter: Redis `INCR` the key, `EXPIRE` it on the first increment within the window. `internal/platform/redis.NewClient` is a thin, reusable connection helper (mirrors `internal/platform/postgres.NewPool`).
- `internal/identity/http.RateLimitLogin` is Gin middleware applied only to `POST /auth/login`, keyed by `c.ClientIP()`. It counts every attempt, not just failed ones — simpler, and a legitimate user rarely calls login more than a handful of times per minute anyway.
- **Fails open**: if Redis is unreachable, `Limiter.Allow` returns `true` (allows the attempt) rather than blocking login entirely. A secondary anti-abuse protection going down must not take the primary authentication flow down with it.

## Consequences

- Redis now has a real, wired purpose; `docker-compose.yml`'s `redis` service gets a healthcheck and `identity-api` depends on it being healthy before starting.
- `/auth/register` is intentionally not rate-limited by this change — it's not the brute-force target `/auth/login` is; revisit if abuse there becomes a concern.
- The 5/minute threshold and fail-open policy are judgment calls appropriate for this system's scale, not derived from a specific attack model — tunable later without changing the `Limiter`/`Store` shape.
- Does not touch the read-cache or upload-idempotency use cases considered and discarded for this decision; either could be added later as a separate, additive use of the same Redis instance.
