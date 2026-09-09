package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

func openSQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open migration connection: %w", err)
	}
	return db, nil
}

func provider(dsn string) (*sql.DB, error) {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
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
	defer db.Close()

	before, _ := goose.GetDBVersionContext(ctx, db)
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
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
	defer db.Close()

	if err := goose.DownToContext(ctx, db, migrationsDir, 0); err != nil {
		return fmt.Errorf("roll back migrations: %w", err)
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
	defer db.Close()

	goose.SetLogger(log.New(out, "", 0))
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	return nil
}

// PendingMigrations counts migrations that have not been applied. Zero means
// the schema is current, which is what readiness reports.
func PendingMigrations(ctx context.Context, dsn string) (int, error) {
	db, err := provider(dsn)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return 0, fmt.Errorf("collect migrations: %w", err)
	}
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}

	pending := 0
	for _, m := range migrations {
		if m.Version > current {
			pending++
		}
	}
	return pending, nil
}
