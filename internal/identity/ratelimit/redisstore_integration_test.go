//go:build integration

package ratelimit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"video-processor/internal/identity/ratelimit"
	platformredis "video-processor/internal/platform/redis"
)

func TestRedisStore_Increment_TracksCountAndExpiresWindow(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set, skipping integration test")
	}

	client, err := platformredis.NewClient(url)
	if err != nil {
		t.Fatalf("connect to redis: %v", err)
	}
	defer client.Close()

	store := ratelimit.NewRedisStore(client)
	key := "test:ratelimit:" + t.Name()
	t.Cleanup(func() { client.Del(context.Background(), key) })

	window := 2 * time.Second
	for i := int64(1); i <= 3; i++ {
		count, err := store.Increment(context.Background(), key, window)
		if err != nil {
			t.Fatalf("Increment() error = %v", err)
		}
		if count != i {
			t.Fatalf("Increment() = %d, want %d", count, i)
		}
	}

	time.Sleep(window + 500*time.Millisecond)

	count, err := store.Increment(context.Background(), key, window)
	if err != nil {
		t.Fatalf("Increment() after expiry error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Increment() after window expired = %d, want 1 (counter should have reset)", count)
	}
}
