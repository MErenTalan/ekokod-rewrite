package cli

import (
	"log/slog"
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/spf13/cobra"
)

func newWorkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run the background worker",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			log := logging.New(cfg.LogLevel, string(cfg.LogFormat), os.Stderr)

			server, mux, err := job.NewServer(cfg.Redis, cfg.Worker, log)
			if err != nil {
				return err
			}
			job.Register(mux, &job.Handlers{Log: log})

			// asynq.Server.Run(handler) is Start(handler) + waitForSignals()
			// + Shutdown(): it installs asynq's own SIGINT/SIGTERM/SIGTSTP
			// handler, which races the CLI's signal.NotifyContext in
			// cmd/ekokod/main.go. On a single signal both would become
			// ready and Run would return nil on its own, making shutdown
			// non-deterministic. Start (non-blocking) + waiting on the
			// command's own context + Stop, then Shutdown, keeps exactly
			// one signal handler in the process and makes the two-phase
			// shutdown explicit: Stop first so the processor stops pulling
			// new tasks off the queues, then Shutdown to drain whatever is
			// already in flight (bounded by cfg.Worker.Timeout, which
			// job.NewServer already sets as asynq.Config.ShutdownTimeout)
			// and close the broker connection.
			if err := server.Start(mux); err != nil {
				return err
			}
			log.Info("worker started", slog.Int("concurrency", cfg.Worker.Concurrency))

			<-cmd.Context().Done()
			log.Info("worker shutting down")
			server.Stop()
			server.Shutdown()
			return nil
		},
	}
}
