# 0012. k6 load test proving concurrent uploads aren't lost

Status: accepted

## Context

Two functional requirements claim a property that had never actually been demonstrated under load: "process more than one video at a time" and "don't lose a request under a spike." Both were true by construction (durable RabbitMQ queue, `ProcessingRequest` persisted before publish, `video-worker` horizontally scalable — see ADR 0002, `docs/technical-architecture.md` §4), but "true by construction" and "proven under an actual concurrent burst" are different claims, and only the second is convincing in a presentation.

Building this surfaced two real bugs, not just missing test coverage:

1. `docker-compose.yml`'s `video-worker` published a fixed host port (`9102:9102`) for its `/metrics` endpoint. Scaling it (`docker compose up --scale video-worker=3`, the exact scenario needed to demonstrate concurrency) failed outright — Docker can't bind the same host port to multiple replicas.
2. `observability/prometheus.yml` scraped `video-worker` via `static_configs`, which resolves the hostname once to a single address. Scaled to N replicas, Prometheus would silently scrape only one of them under one shared `instance` label — the other replicas' metrics would never reach Grafana, making the dashboard actively misleading during the exact demo it's meant to support.

## Decision

- **Fixed both bugs first**: dropped the host port publish on `video-worker` (Prometheus reaches it over the internal Docker network regardless; losing host-side `curl` access to it is an acceptable trade). Switched the `video-worker` Prometheus job from `static_configs` to `dns_sd_configs` (`type: A`), which resolves every replica's IP as a distinct target/instance — verified with 3 replicas: 3 separate `up` targets, each with its own `videos_processed_total` series.
- **`scripts/loadtest/upload_stress.js`** (k6): `VUS` concurrent virtual users (default 20) each perform one real multipart upload of a fixture video (`scripts/loadtest/fixtures/sample.mp4`, a real ~66KB MP4, not a fake byte blob — `video-api`'s `ffprobe`-based duration check would reject anything else) against a live `video-api`. A `checks{check:upload_accepted}` threshold (`rate==1.0`) fails the run if even one upload is rejected or errors.
- A `teardown()` phase polls the shared test user's status listing (`GET /videos`) until every uploaded request reaches a terminal state, then asserts zero `FAILED` — proving the burst was not just *accepted* but actually *processed* to completion, not merely queued and forgotten.
- Not part of CI: it needs the full stack running and real `ffmpeg` processing (slow, resource-heavy on a shared CI runner) — same category of deliberate scope cut as `docs/adr/0009`'s and `0011`'s CI decisions. It's a manual/demo tool, run before recording the presentation video, ideally with `docker compose up --build --scale video-worker=3` and Grafana open (`localhost:3000`) to show the burst land and drain live.

## Consequences

- Verified end to end: 20 concurrent uploads, 100% accepted, 100% reached `COMPLETED`, 0 failures, load spread evenly across 3 `video-worker` replicas (`videos_processed_total{status="completed"}`: 13/13/13 across three distinct `instance` labels), RabbitMQ queue depth back to 0 after the drain.
- The port and Prometheus DNS fixes are correctness fixes independent of the load test itself — they were simply invisible until someone tried to actually scale `video-worker` past 1 replica while watching its metrics, which nothing in the project had done before this.
- `scripts/loadtest/fixtures/sample.mp4` is committed (unlike `testdata/`, which is gitignored) so the script is reproducible from a fresh clone without depending on local test fixtures.
