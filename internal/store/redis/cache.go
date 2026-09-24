package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Cache is a prefixed byte cache with TTLs (the weather panel's, R291).
type Cache struct {
	client *goredis.Client
	prefix string
}

// NewCache wraps client; prefix namespaces every key.
func NewCache(client *goredis.Client, prefix string) *Cache {
	return &Cache{client: client, prefix: prefix}
}

// Get returns the value and whether it was present; a miss is not an error.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := c.client.Get(ctx, c.prefix+key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// Set stores value for ttl.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, c.prefix+key, value, ttl).Err()
}
