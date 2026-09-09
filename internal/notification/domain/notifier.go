package domain

import "context"

// Notifier is intentionally decoupled from the Video Processing domain's
// FailureReason enum (see docs/bounded-contexts.md: Notification owns how
// outcomes are communicated) — callers translate to plain, friendly text.
type Notifier interface {
	NotifyCompleted(ctx context.Context, to, fileName string) error
	NotifyFailed(ctx context.Context, to, fileName, reason string) error
}
