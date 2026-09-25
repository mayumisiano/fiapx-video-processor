# 0005. Horizontal scaling via `docker compose --scale`, not Kubernetes

Status: accepted

## Context

The brief requires an architecture that can scale, and lists Docker + Kubernetes or Docker Compose as acceptable stacks. Kubernetes + KEDA would give automatic autoscaling based on queue depth.

## Decision

Use Docker Compose. Horizontal scaling is demonstrated with `docker compose up --scale worker=N`: `worker` is stateless, so N replicas can consume the same RabbitMQ queue with no code change.

## Consequences

- Proves the scalability concept (stateless worker, shared queue, no coordination needed) with much lower operational risk than adding a Kubernetes cluster the week of the demo.
- No automatic scaling based on load — replica count is manual, decided by whoever runs the compose command.
- The design doesn't block a future move to Kubernetes: `worker` being stateless and horizontally scalable is the actual prerequisite, and that's already true regardless of orchestrator.
