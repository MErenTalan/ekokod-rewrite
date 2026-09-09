// Package postgres owns the database connection pool and the embedded schema
// migrations. It contains no business logic.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens the connection pool and verifies it can reach the database.
func NewPool(ctx context.Context, cfg config.DB, log *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, scrubErr(cfg.URL, "parse database url", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute

	if cfg.StatementTimeout > 0 {
		if poolCfg.ConnConfig.RuntimeParams == nil {
			poolCfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
			fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, scrubErr(cfg.URL, "open database pool", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, scrubErr(cfg.URL, "ping database", err)
	}

	log.Info("database pool ready",
		slog.Int("max_conns", cfg.MaxConns),
		slog.Int("min_conns", cfg.MinConns),
		slog.Duration("statement_timeout", cfg.StatementTimeout))
	return pool, nil
}

// Ping verifies the pool can reach the database.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is not initialised")
	}
	if err := pool.Ping(ctx); err != nil {
		var password string
		if cfg := pool.Config(); cfg != nil && cfg.ConnConfig != nil {
			password = cfg.ConnConfig.Password
		}
		return scrubPoolErr(password, "ping database", err)
	}
	return nil
}
