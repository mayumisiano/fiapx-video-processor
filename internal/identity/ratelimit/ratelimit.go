// Package ratelimit throttles login attempts to blunt credential-stuffing
// and brute-force against identity-api (docs/adr/0010).
package ratelimit

import (
	"context"
	"time"
)

// Store is the port Limiter depends on: a fixed-window counter that
// increments key and returns the count after incrementing, arming the
// window's expiry only on that key's first increment. RedisStore
// (internal/platform/redis-backed) is the real adapter; tests can fake
// this without a live Redis.
type Store interface {
	Increment(ctx context.Context, key string, window time.Duration) (int64, error)
}

type Limiter struct {
	store  Store
	max    int64
	window time.Duration
}

func NewLimiter(store Store, max int, window time.Duration) *Limiter {
	return &Limiter{store: store, max: int64(max), window: window}
}

// Allow reports whether another attempt under key is permitted, and
// increments the counter as a side effect. It fails open (allows the
// attempt) if the store errors: Redis being unreachable must not lock
// every user out of login over a secondary protection (see docs/adr/0010).
func (l *Limiter) Allow(ctx context.Context, key string) bool {
	count, err := l.store.Increment(ctx, key, l.window)
	if err != nil {
		return true
	}
	return count <= l.max
}
