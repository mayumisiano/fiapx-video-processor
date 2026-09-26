# 0008. Minimal observability: Prometheus + Grafana

Status: accepted

## Context

The challenge brief names `Prometheus + Grafana, ELK Stack ou algo de preferência do grupo` as recommended stack for monitoring. Until this decision, the only observability in the system was `GET /health` on `identity-api`/`video-api` (a Postgres ping) — no metrics, no dashboard, `docs/technical-architecture.md` §9 documented this honestly as "planned, not yet implemented".

## Decision

Add metrics, not a full observability stack:

- `internal/platform/metrics` (Go, `github.com/prometheus/client_golang`): a Gin middleware recording `http_requests_total`/`http_request_duration_seconds` (labeled by service, method, matched route, status), mounted on `identity-api` and `video-api` at `/metrics`. `video-worker` has no HTTP router today, so it gets a bare standalone `/metrics` server instead of pulling in Gin just for this.
- `videos_processed_total{status}` and `video_processing_duration_seconds`, recorded around the existing `Processor.Process` call in the worker's consumer loop. `Process` now returns the resulting `domain.Status` alongside its error, so the metric reflects the actual business outcome (`completed`/`failed`) — not just whether the message was nacked (`infra_error`), which is a different, already-existing signal (dead-letter queue).
- RabbitMQ's own `rabbitmq_prometheus` plugin, already bundled in the `-management` image in use, enabled via the compose `command` override — zero application code, gets queue depth and message rates for free.
- `prometheus` + `grafana` added to `docker-compose.yml`, both provisioned from files in the new `observability/` directory: Prometheus's scrape config, Grafana's datasource, and one dashboard (`fiapx-overview.json`, 5 panels covering the above). `docker compose up` produces a working dashboard with no manual clicking, and anonymous viewer access is enabled so opening `localhost:3000` needs no login for the demo recording.

## Consequences

- Closes the most visible gap against the brief's recommended stack, with a small, contained diff: one new internal package, ~10 lines wired into each `cmd/*/main.go`, and compose/config files. No new business logic changed.
- `Processor.Process`'s signature changed (`(error)` → `(domain.Status, error)`); its five existing unit tests were updated, not rewritten — the change is additive information, not new behavior.

## Explicitly out of scope

- **Distributed tracing** (OpenTelemetry, Jaeger): no cross-service request beyond the JWT trust boundary (`docs/adr/0007`) yet complex enough to need trace correlation; would be the first thing to add if a gateway or more service-to-service calls were introduced.
- **Alerting rules** (Alertmanager, Grafana alerts): no on-call process exists for a hackathon; a dashboard is enough to *show* health, not to page anyone.
- **Log aggregation (ELK or similar)**: the three binaries still log plain text to stdout (`docker compose logs`); shipping them into Elasticsearch/Loki is a real improvement but a separate piece of work from metrics, and lower priority than closing the "no monitoring at all" gap first.

These are logged here as conscious cuts, not omissions discovered later.
