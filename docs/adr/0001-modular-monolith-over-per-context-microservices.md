# 0001. Modular monolith (api + worker), not one microservice per bounded context

Status: superseded by 0007

## Context

Three bounded contexts exist (Identity, Video Processing, Notification). The challenge brief asks for "microservices development." The literal reading would be one deployable service per context, each with its own database and API.

## Decision

Ship two binaries, `api` and `worker`, not three-plus services. Bounded contexts remain code boundaries (separate `internal/` packages, no cross-context imports of concrete adapters), but they run inside the same processes and share one Postgres database.

`worker` is the one split out on its own, because it's the only component with load patterns different from `api` and a genuine need to scale independently (multiple replicas consuming the same queue).

## Consequences

- Gets "process more than one video at a time" and "scalable architecture" for free (`docker compose up --scale worker=N`), without service discovery, network calls between contexts, or per-context database provisioning.
- Identity and Video Processing share a Postgres instance — not full data ownership per service, so this is not microservices in the strict sense.
- The package boundaries already in place mean extracting Identity into its own service later is a refactor, not a rewrite: swap the in-process call for an HTTP call, keep the JWT contract the same.
- If a reviewer expects literal per-context microservices, this decision needs to be defended out loud, not just left implicit in the code.
