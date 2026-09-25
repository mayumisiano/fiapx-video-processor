# 0002. RabbitMQ as the work queue between api and worker

Status: accepted

## Context

The brief requires messaging and requires that peak load not drop requests. Candidates considered: RabbitMQ, Apache Kafka, NATS JetStream, and a workflow engine (Temporal).

## Decision

Use RabbitMQ. One durable queue, one message per accepted (or retried) `ProcessingRequest`, persistent delivery mode, dead-letter exchange for messages that fail at the infrastructure level (not business failures — those are a normal state transition, not a broker exception).

## Consequences

- The access pattern is a classic work queue — one message, one consumer, done and discarded — which is exactly what RabbitMQ is built for. Kafka is built for event replay and multiple independent consumer groups, which this system doesn't need.
- `QueueDeclare` with `durable: true` plus `DeliveryMode: Persistent` means a broker restart doesn't lose an accepted-but-unprocessed request.
- Temporal would model retries and state more elegantly, but the learning curve doesn't fit the timeline; the risk of shipping nothing outweighs the sophistication.
- NATS JetStream is lighter, but RabbitMQ's DLQ and ack/retry semantics are more mature and better documented — no concrete gain from switching.
