# 0011. CI hardening: lint, scoped coverage gate, broader integration tests, compose smoke test

Status: accepted

## Context

An audit of the CI pipeline (`.github/workflows/ci.yml`) surfaced four gaps:

1. **No linter.** Only `go vet` ran — it catches correctness bugs (nil pointer misuse, bad `Printf` verbs), not the style/quality issues a linter catches (ignored errors, ineffectual assignments, dead code).
2. **No coverage gate.** `go test -cover` printed a percentage but nothing failed the build if it dropped.
3. **Integration tests covered only Postgres (+ the just-added Redis rate limiter).** `internal/videoprocessing/queue` (RabbitMQ) and `internal/videoprocessing/storage` (MinIO) had zero test files, unit or integration — the riskiest infra boundaries in the system were only ever checked by hand.
4. **The Docker build check only proved the images compile.** `build-images` never proved the services actually start and pass their health checks together.

## Decision

- **Lint**: new `lint` job running `golangci-lint` v2 (`.golangci.yml`: `errcheck`, `govet`, `staticcheck`, `unused`, `ineffassign` — a deliberately small, high-signal set, not the "enable everything" preset). Runs clean today (`0 issues`).
- **Coverage gate, scoped, not flat**: a repo-wide coverage percentage is meaningless here — thin infra adapters (`postgres`, `rabbitmq`, `minio`, `redis`, `jwt`) are exercised by integration tests, not unit tests, so they sit at 0% unit coverage *by design*. Gating on the flat total (~20%) would either be trivially easy to pass or would misrepresent adapters as "untested." Instead, `unit-tests` gates only the four domain/application packages that are supposed to be unit-testable without any infra (`internal/{identity,videoprocessing}/{domain,application}`), currently at 80.9%, against a 70% floor.
- **RabbitMQ and MinIO integration tests**: `queue/rabbitmq_integration_test.go` (publish → consume round trip, asserting the delivered body) and `storage/minio_integration_test.go` (`EnsureBuckets` idempotency, upload/stat/download round trip, and a presigned URL fetched over real HTTP and checked byte-for-byte). Same `//go:build integration` + env-var-skip convention as the existing Postgres tests. `integration-tests` in CI gains a `rabbitmq` service (fits the `services:` block — its image's default `CMD` needs no args) and a MinIO container started as a plain `docker run` step, since GitHub Actions' `services:` block has no field for a custom command and MinIO's image requires `server /data` as its command.
- **Compose smoke test**: new `compose-smoke-test` job — `docker compose up --build`, poll `identity-api`/`video-api` container health status, and fail if any container is `Exited` (meaningful now that ADR 0009 means there should never be one). This is the first automated check that the whole stack — not just each image individually — actually comes up together.

## Consequences

- Four CI jobs now run in parallel (`lint`, `unit-tests`, `integration-tests`, then `build-images` → `compose-smoke-test`), catching more classes of regression before merge: style/quality issues, a coverage regression in business logic specifically, breakage in the RabbitMQ/MinIO adapters, and a stack that builds but doesn't actually come up together.
- The coverage gate is intentionally narrow. Widening it to cover adapters would require testing them at the unit level with fakes (defeating the purpose of the integration tests that already exist for them) or lowering the bar to a number low enough to be meaningless. Scoping to business logic keeps the gate honest.
- CI runtime increases (new jobs, an extra service container, a full compose stack boot) — acceptable for the coverage gained, no different in kind from the tradeoff already accepted for `integration-tests` needing real Postgres/Redis containers.
