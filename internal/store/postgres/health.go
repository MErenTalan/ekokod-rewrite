package postgres

import (
	"context"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolCheck reports whether the database is reachable.
func PoolCheck(pool *pgxpool.Pool) health.Check {
	return health.Check{
		Name: "database",
		Fn:   func(ctx context.Context) error { return Ping(ctx, pool) },
	}
}

// MigrationsCheck reports whether the schema is current.
func MigrationsCheck(dsn string) health.Check {
	return health.Check{
		Name: "migrations",
		Fn: func(ctx context.Context) error {
			pending, err := PendingMigrations(ctx, dsn)
			if err != nil {
				return err
			}
			if pending > 0 {
				return fmt.Errorf("%d migration(s) pending", pending)
			}
			return nil
		},
	}
}
