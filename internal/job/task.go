// Package job defines the platform's background tasks and their handlers.
// Every task is idempotent, bounded, isolated and recorded.
package job

import (
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/hibiken/asynq"
	goredis "github.com/redis/go-redis/v9"
)

// Task type names. Later phases add integration, billing, alarm and report tasks.
const (
	TypeNoop = "system.noop"
)

// Queue names, ordered by priority when the worker picks work.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// RedisOpt builds the asynq connection options, pointing at the queue database.
func RedisOpt(cfg config.Redis) (asynq.RedisClientOpt, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return asynq.RedisClientOpt{}, secret.URLParseErr("parse redis url")
	}
	return asynq.RedisClientOpt{
		Addr:     opts.Addr,
		Username: opts.Username,
		Password: opts.Password,
		DB:       cfg.QueueDB,
	}, nil
}

// Handlers holds the dependencies every task handler needs.
type Handlers struct {
	Log *slog.Logger
}

// Register attaches every handler to the mux.
func Register(mux *asynq.ServeMux, h *Handlers) {
	mux.HandleFunc(TypeNoop, h.Noop)
}
