package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolCheck reports whether the database is reachable.
func PoolCheck(pool *pgxpool.Pool) health.Check {
	return health.Check{
		Name: "database",
		Fn:   func(ctx context.Context) error { return Ping(ctx, pool) },
	}
}

// MigrationsCheck reports whether the schema is current. It answers through
// the already-open pool rather than opening a fresh connection: it is wired
// into a readiness endpoint polled roughly every ten seconds, and the answer
// almost never changes, so a new TCP connection and auth handshake per poll
// would be wasted work. Use PendingMigrations instead for one-off CLI use.
func MigrationsCheck(pool *pgxpool.Pool) health.Check {
	return health.Check{
		Name: "migrations",
		Fn: func(ctx context.Context) error {
			return checkMigrationsCurrent(ctx, pool)
		},
	}
}

// checkMigrationsCurrent compares the highest applied migration version,
// read through pool, against the highest version embedded in the binary
// (which needs no database connection to compute).
func checkMigrationsCurrent(ctx context.Context, pool *pgxpool.Pool) error {
	want, err := highestEmbeddedVersion()
	if err != nil {
		return err
	}

	// Scan into *int64, not int64: max() over an empty goose_db_version
	// (table exists, no row yet applied) returns SQL NULL.
	var applied *int64
	err = pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&applied)
	if err != nil {
		if isUndefinedTable(err) {
			applied = nil // goose_db_version has never been created: nothing applied yet
		} else {
			return fmt.Errorf("read schema version: %w", err)
		}
	}

	var appliedVersion int64
	if applied != nil {
		appliedVersion = *applied
	}
	if appliedVersion < want {
		return fmt.Errorf("%d migration(s) pending", want-appliedVersion)
	}
	return nil
}

// isUndefinedTable reports whether err is Postgres error code 42P01
// (undefined_table), meaning goose_db_version does not exist yet.
func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}
