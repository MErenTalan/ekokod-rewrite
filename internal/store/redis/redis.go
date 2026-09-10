// Package redis owns the cache connection. The job queue uses a separate
// logical database so a cache flush can never drop queued work.
package redis

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	goredis "github.com/redis/go-redis/v9"
)

// New opens the cache client and verifies it can reach Redis.
func New(ctx context.Context, cfg config.Redis, log *slog.Logger) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, secret.URLParseErr("parse redis url")
	}
	opts.DB = cfg.CacheDB

	client := goredis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, scrubErr(opts.Password, "ping redis", err)
	}
	log.Info("redis ready", slog.Int("cache_db", cfg.CacheDB))
	return client, nil
}

// Check reports whether Redis is reachable.
func Check(client *goredis.Client) health.Check {
	return health.Check{
		Name: "redis",
		Fn: func(ctx context.Context) error {
			if client == nil {
				return fmt.Errorf("redis client is not initialised")
			}
			if err := client.Ping(ctx).Err(); err != nil {
				var password string
				if opts := client.Options(); opts != nil {
					password = opts.Password
				}
				return scrubErr(password, "ping redis", err)
			}
			return nil
		},
	}
}
