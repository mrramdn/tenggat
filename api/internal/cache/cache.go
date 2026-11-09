// Package cache is the Redis side of Tenggat: cached reads, cached certificate
// lookups and a per-client rate limit. Every method fails soft: when Redis is
// unreachable the API still answers, only without the cache.
package cache

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const prefix = "tenggat:"

// Cache wraps a Redis client.
type Cache struct {
	rdb *redis.Client
}

// New connects lazily to a Redis server at addr, e.g. "cache:6379".
func New(addr string) *Cache {
	return &Cache{rdb: redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PoolSize:     10,
	})}
}

// Close releases the connections.
func (c *Cache) Close() error { return c.rdb.Close() }

// Ping checks that Redis answers.
func (c *Cache) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

// Get returns a cached value; any error counts as a miss.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	b, err := c.rdb.Get(ctx, prefix+key).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

// Set stores a value with a time to live.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) {
	_ = c.rdb.Set(ctx, prefix+key, value, ttl).Err()
}

// Version is part of every cached read key of one scope (a user). Bump changes
// it after a write, so old entries are never read again and simply expire.
func (c *Cache) Version(ctx context.Context, scope string) int64 {
	v, err := c.rdb.Get(ctx, prefix+"versi:"+scope).Int64()
	if err != nil {
		return 0
	}
	return v
}

// Bump invalidates every cached read of one scope. The counter lives a day; read
// entries live a minute, so a counter that expires can never revive a stale one.
func (c *Cache) Bump(ctx context.Context, scope string) {
	key := prefix + "versi:" + scope
	pipe := c.rdb.TxPipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 24*time.Hour)
	_, _ = pipe.Exec(ctx)
}

// Allow counts one request from client in a fixed window. It returns false and
// the time until the window resets once limit requests were already counted.
// If Redis cannot be reached the request is allowed.
func (c *Cache) Allow(ctx context.Context, bucket, client string, limit int, window time.Duration) (bool, time.Duration) {
	now := time.Now()
	slot := now.Unix() / int64(window.Seconds())
	key := prefix + "batas:" + bucket + ":" + client + ":" + strconv.FormatInt(slot, 10)

	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return true, 0
	}
	if incr.Val() > int64(limit) {
		reset := time.Unix((slot+1)*int64(window.Seconds()), 0)
		return false, reset.Sub(now)
	}
	return true, 0
}
