package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
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
			log := newCommandLogger(cfg, os.Stderr)

			// asynq never dials its broker eagerly and, once Start
			// succeeds, surfaces no error at all if the broker later goes
			// unreachable (task 9 review, Important-1): the processor just
			// loops on Dequeue failures forever. HealthCheckFunc is the
			// only signal asynq offers, and asynq does not even start its
			// healthcheck goroutine unless one is supplied
			// (asynq@v0.26.0 healthcheck.go:58-61) — so onHealthCheck
			// below is not optional instrumentation, it is the only way
			// this process can ever learn its broker died and exit
			// non-zero for a supervisor to act on.
			brokerDead := make(chan error, 1)
			onHealthCheck := newHealthTracker(consecutiveHealthCheckFailureLimit, func() {
				select {
				case brokerDead <- fmt.Errorf(
					"worker: redis broker unreachable for %d consecutive health checks",
					consecutiveHealthCheckFailureLimit):
				default:
				}
			})

			server, mux, err := job.NewServer(cfg.Redis, cfg.Worker, log, onHealthCheck)
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
			// already in flight (bounded by job.ShutdownTimeout(cfg.Worker.Timeout),
			// which job.NewServer sets as asynq.Config.ShutdownTimeout) and
			// close the broker connection.
			if err := server.Start(mux); err != nil {
				return err
			}
			log.Info("worker started", slog.Int("concurrency", cfg.Worker.Concurrency))

			select {
			case <-cmd.Context().Done():
				log.Info("worker shutting down")
				server.Stop()
				server.Shutdown()
				return nil
			case err := <-brokerDead:
				log.Error("worker exiting: broker unreachable", slog.String("error", err.Error()))
				server.Stop()
				server.Shutdown()
				return err
			}
		},
	}
}
