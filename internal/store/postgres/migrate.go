package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"
	"log/slog"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

// gooseSetupOnce guards goose's package-level SetBaseFS/SetDialect calls:
// they are plain package variables with no internal synchronisation in
// goose v3.28.0, so calling them on every invocation (as provider did
// originally) races when MigrationsCheck is polled concurrently with a CLI
// migration. Performing the setup exactly once removes the race.
var (
	gooseSetupOnce sync.Once
	gooseSetupErr  error

	// gooseLogMu guards goose.SetLogger, which — unlike SetBaseFS/SetDialect —
	// genuinely changes behaviour on every call (MigrateStatus points it at a
	// caller-supplied io.Writer). The mutex is held for the duration of
	// MigrateStatus so a concurrent call can never observe, or race on,
	// another call's logger.
	gooseLogMu sync.Mutex
)

func setupGoose() error {
	gooseSetupOnce.Do(func() {
		goose.SetBaseFS(migrationsFS)
		gooseSetupErr = goose.SetDialect("postgres")
	})
	return gooseSetupErr
}

func openSQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, scrubErr(dsn, "open migration connection", err)
	}
	return db, nil
}

func provider(dsn string) (*sql.DB, error) {
	if err := setupGoose(); err != nil {
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	return openSQL(dsn)
}

// MigrateUp applies every pending migration.
func MigrateUp(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	before, _ := goose.GetDBVersionContext(ctx, db)
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return scrubErr(dsn, "apply migrations", err)
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return scrubErr(dsn, "read schema version", err)
	}
	log.Info("migrations applied", slog.Int64("from", before), slog.Int64("to", after))
	return nil
}

// MigrateDownAll rolls every migration back.
func MigrateDownAll(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := goose.DownToContext(ctx, db, migrationsDir, 0); err != nil {
		return scrubErr(dsn, "roll back migrations", err)
	}
	log.Info("migrations rolled back")
	return nil
}

// MigrateStatus writes the migration status table to out.
func MigrateStatus(ctx context.Context, dsn string, out io.Writer) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	gooseLogMu.Lock()
	defer gooseLogMu.Unlock()

	goose.SetLogger(log.New(out, "", 0))
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return scrubErr(dsn, "read migration status", err)
	}
	return nil
}

// PendingMigrations counts migrations that have not been applied. Zero means
// the schema is current, which is what readiness reports. It opens its own
// connection, so it is meant for CLI use; the readiness check instead uses
// MigrationsCheck, which answers through an already-open pool.
func PendingMigrations(ctx context.Context, dsn string) (int, error) {
	db, err := provider(dsn)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()

	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("collect migrations: %w", err)
	}
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, scrubErr(dsn, "read schema version", err)
	}

	pending := 0
	for _, m := range migrations {
		if m.Version > current {
			pending++
		}
	}
	return pending, nil
}

// highestEmbeddedVersion returns the version of the most recent migration
// embedded in the binary. It requires no database connection, so
// MigrationsCheck can call it on every readiness poll for free.
func highestEmbeddedVersion() (int64, error) {
	if err := setupGoose(); err != nil {
		return 0, fmt.Errorf("set goose dialect: %w", err)
	}
	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("collect migrations: %w", err)
	}
	var highest int64
	for _, m := range migrations {
		if m.Version > highest {
			highest = m.Version
		}
	}
	return highest, nil
}
