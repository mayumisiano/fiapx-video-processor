package ratelimit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"video-processor/internal/identity/ratelimit"
)

type fakeStore struct {
	counts map[string]int64
	err    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{counts: map[string]int64{}}
}

func (s *fakeStore) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.counts[key]++
	return s.counts[key], nil
}

func TestLimiter_Allow_UnderMax_ReturnsTrue(t *testing.T) {
	limiter := ratelimit.NewLimiter(newFakeStore(), 3, time.Minute)

	for i := 0; i < 3; i++ {
		if !limiter.Allow(context.Background(), "1.2.3.4") {
			t.Fatalf("attempt %d: Allow() = false, want true", i+1)
		}
	}
}

func TestLimiter_Allow_OverMax_ReturnsFalse(t *testing.T) {
	limiter := ratelimit.NewLimiter(newFakeStore(), 3, time.Minute)

	for i := 0; i < 3; i++ {
		limiter.Allow(context.Background(), "1.2.3.4")
	}

	if limiter.Allow(context.Background(), "1.2.3.4") {
		t.Fatalf("4th attempt: Allow() = true, want false")
	}
}

func TestLimiter_Allow_DifferentKeys_AreIndependent(t *testing.T) {
	limiter := ratelimit.NewLimiter(newFakeStore(), 1, time.Minute)

	if !limiter.Allow(context.Background(), "1.2.3.4") {
		t.Fatalf("first key, first attempt: Allow() = false, want true")
	}
	if !limiter.Allow(context.Background(), "5.6.7.8") {
		t.Fatalf("second key, first attempt: Allow() = false, want true")
	}
}

func TestLimiter_Allow_StoreError_FailsOpen(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("redis unreachable")
	limiter := ratelimit.NewLimiter(store, 1, time.Minute)

	if !limiter.Allow(context.Background(), "1.2.3.4") {
		t.Fatalf("Allow() = false on store error, want true (fail open)")
	}
}
