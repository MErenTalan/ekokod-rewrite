package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

// leaderRetry is how often a non-leading instance attempts to acquire
// leadership, and how often the current leader checks that it still holds
// the connection its advisory lock lives on.
const leaderRetry = 10 * time.Second

// Entry is one cron-scheduled task.
type Entry struct {
	Cron string
	Task *asynq.Task
}

// Scheduler enqueues the periodic tasks while it holds leadership.
type Scheduler struct {
	cfg     *config.Config
	log     *slog.Logger
	elector *Elector
}

// New builds the scheduler process. Whether it is run at all is a deployment
// decision (config.Scheduler.Enabled) made by the caller, not by Scheduler
// itself.
func New(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		log:     log,
		elector: NewElector(pool, LockKeyScheduler, leaderRetry, log),
	}
}

// entries returns the cron schedule. F0 registers only the noop task so the
// enqueue/leader-election wiring is exercised end to end; later phases add
// their own tasks (ingestion, EPIAS, alarms, billing, forecast, carbon,
// reports) here, each driven by its own field on config.Schedule.
func (s *Scheduler) entries() ([]Entry, error) {
	noop, err := job.NewNoopTask("scheduled heartbeat")
	if err != nil {
		return nil, err
	}
	return []Entry{{Cron: "@every 1h", Task: noop}}, nil
}

// Run campaigns for leadership and schedules tasks while leading. It blocks
// until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	opt, err := job.RedisOpt(s.cfg.Redis)
	if err != nil {
		return err
	}
	entries, err := s.entries()
	if err != nil {
		return err
	}

	return s.elector.Run(ctx, func(leadCtx context.Context) error {
		asynqScheduler := asynq.NewScheduler(opt, &asynq.SchedulerOpts{
			Location: s.cfg.Timezone,
			Logger:   schedulerLogger{log: s.log},
		})
		for _, entry := range entries {
			if _, err := asynqScheduler.Register(entry.Cron, entry.Task); err != nil {
				return fmt.Errorf("register %s at %q: %w", entry.Task.Type(), entry.Cron, err)
			}
		}
		if err := asynqScheduler.Start(); err != nil {
			return fmt.Errorf("start scheduler: %w", err)
		}
		s.log.Info("scheduler running", slog.Int("entries", len(entries)),
			slog.String("timezone", s.cfg.Timezone.String()))

		<-leadCtx.Done()
		asynqScheduler.Shutdown()
		return nil
	})
}

// schedulerLogger adapts slog to asynq's logger interface.
type schedulerLogger struct{ log *slog.Logger }

func (l schedulerLogger) Debug(args ...any) { l.log.Debug(fmt.Sprintln(args...)) }
func (l schedulerLogger) Info(args ...any)  { l.log.Info(fmt.Sprintln(args...)) }
func (l schedulerLogger) Warn(args ...any)  { l.log.Warn(fmt.Sprintln(args...)) }
func (l schedulerLogger) Error(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
func (l schedulerLogger) Fatal(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
