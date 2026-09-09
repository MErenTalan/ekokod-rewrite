package cli

import (
	"context"
	"errors"
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/scheduler"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/spf13/cobra"
)

func newSchedulerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scheduler",
		Short: "Run the periodic task scheduler (leader-elected)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)

			// scheduler.New's doc comment (internal/scheduler/scheduler.go:26-28)
			// is explicit that whether the scheduler runs at all is a
			// deployment decision left to the caller. Honour it here, before
			// opening a database connection: on a replica where the
			// scheduler role is disabled this must be a no-op, not a
			// connection to a database it will never use.
			if !cfg.Scheduler.Enabled {
				log.Warn("scheduler disabled by EKOKOD_SCHEDULER_ENABLED=false; exiting")
				return nil
			}

			pool, err := postgres.NewPool(cmd.Context(), cfg.DB, log)
			if err != nil {
				return err
			}
			defer pool.Close()

			err = scheduler.New(cfg, pool, log).Run(cmd.Context())
			if isCleanShutdown(err) {
				// Scheduler.Run returns the leader-election loop's
				// ctx.Err() on a normal shutdown (SIGINT/SIGTERM cancels
				// cmd.Context()), not nil. Checking the returned error
				// itself with errors.Is, rather than re-reading
				// cmd.Context().Err() after the fact, ties this
				// classification to the actual value Run produced: if Run
				// ever raced a genuine failure against the context being
				// cancelled, re-deriving the answer from ctx.Err() could
				// call a real error "clean" merely because the process was
				// also shutting down at that moment.
				log.Info("scheduler stopped")
				return nil
			}
			return err
		},
	}
}

// isCleanShutdown reports whether err is exactly the context cancellation
// scheduler.Scheduler.Run propagates from its underlying elector on a normal
// stop, as opposed to a genuine startup or operational failure.
func isCleanShutdown(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}
