package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
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

// MigrateDownN rolls back exactly n applied migrations, most recent first —
// `goose down` run n times, in one call. It exists for
// TestF2IntervalColumnSurvivesCompressedChunk (task-5-brief.md step 1),
// which must roll back 00013 then 00012 ONLY, proving the pair's own
// add-column/drop-column round-trips, without also tearing down every
// earlier migration's tables the way MigrateDownAll would (which would lose
// the very row the test is trying to re-read). n <= 0 is a no-op.
func MigrateDownN(ctx context.Context, dsn string, log *slog.Logger, n int) error {
	db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	before, _ := goose.GetDBVersionContext(ctx, db)
	for range n {
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			return scrubErr(dsn, "roll back one migration", err)
		}
	}
	after, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return scrubErr(dsn, "read schema version", err)
	}
	log.Info("migrations rolled back", slog.Int64("from", before), slog.Int64("to", after), slog.Int("steps", n))
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

// MigrationsFingerprint returns a short, deterministic hex digest over every
// embedded migration file's name and contents.
//
// It exists for testfixtures' shared isolated-database template (F4 Task
// 0): that template's name folds this in, so a server shared across test
// PROCESSES — where no in-memory sync.Once can protect against a template
// another process (or an older branch's build, left running) already
// built — never clones a template migrated with a different, stale
// migration set. Change any migration file and the fingerprint changes, so
// the template name changes, so a fresh template gets built rather than a
// stale one silently reused.
//
// embed.FS.ReadDir returns entries already sorted by filename (documented
// behaviour), so hashing in that order is deterministic across processes
// without this function sorting again itself.
func MigrationsFingerprint() (string, error) {
	return fingerprintFS(migrationsFS, migrationsDir)
}

// fingerprintFS is MigrationsFingerprint's pure implementation, taking the
// filesystem and directory as parameters so a unit test can hand it a
// small in-memory fs.FS (fstest.MapFS) instead of needing to change the
// real embedded migrations to prove the fingerprint reacts to content.
func fingerprintFS(fsys fs.FS, dir string) (string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", fmt.Errorf("read embedded migrations dir: %w", err)
	}

	h := sha256.New()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(fsys, dir+"/"+entry.Name())
		if err != nil {
			return "", fmt.Errorf("read embedded migration %s: %w", entry.Name(), err)
		}
		// A NUL separator between fields/entries: no migration filename or
		// contents can contain one, so this can never let two different
		// (name, content) sequences hash the same.
		h.Write([]byte(entry.Name()))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
