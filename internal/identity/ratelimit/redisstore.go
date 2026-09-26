package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore is the Store adapter backing Limiter in production: INCR the
// key, and on its first increment within the window (count == 1) set its
// TTL — the standard fixed-window counter. A lost race on that boundary
// under-sets the expiry for one request at most, not a correctness issue
// for rate limiting.
type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		if err := s.client.Expire(ctx, key, window).Err(); err != nil {
			return 0, err
		}
	}
	return count, nil
}
