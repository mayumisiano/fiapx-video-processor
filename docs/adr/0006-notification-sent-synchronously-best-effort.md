# 0006. Notification sent synchronously, best-effort, from the worker

Status: accepted

## Context

The brief requires notifying the user on error. Video Processing → Notification is modeled as a domain event relationship in `docs/bounded-contexts.md` (`ProcessingCompleted` / `ProcessingFailed`), which suggests an async, queue-driven notification path. In practice, the worker calls the SMTP mailer directly after updating the request's status, inside the same message handler that processed the video.

## Decision

Keep it synchronous and best-effort: sending the email happens inline in `Processor.Process`, guarded so a mailer failure never fails the video-processing message itself (the video's status transition is already committed before the email is attempted). No separate notification queue or consumer process.

## Consequences

- One less moving part: no second queue, no second consumer to deploy, monitor, and keep alive.
- A slow or unreachable SMTP server delays the worker picking up the *next* video, since the same goroutine that consumes the queue makes the SMTP call — capped by an explicit send timeout so it degrades instead of hanging.
- `internal/notification/consumer/` exists as an empty package. It should be deleted, not left as a stub implying a queue that doesn't exist — tracked as follow-up cleanup, not a pending feature.
- If email volume or latency ever becomes a bottleneck, the fix is to publish an event and give Notification its own consumer — the domain event language for this already exists in `docs/bounded-contexts.md`, so the extraction is additive, not a redesign.
