package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
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

// entries returns the cron schedule: the F0 heartbeat, plus F2's two
// platform-wide dispatch ticks (R16) — integration.sync_dispatch, which
// itself fans out per-credential work, and epias.sync_prices. Every future
// phase's own cron task (alarms, billing, forecast, carbon, reports) is
// added here the same way, each driven by its own field on config.Schedule.
func (s *Scheduler) entries() ([]Entry, error) {
	noop, err := job.NewNoopTask("scheduled heartbeat")
	if err != nil {
		return nil, err
	}
	retryOpts := job.TaskOptions{MaxRetry: s.cfg.Worker.MaxRetries}
	syncDispatch, err := job.NewSyncDispatchTask(retryOpts)
	if err != nil {
		return nil, err
	}
	syncPrices, err := job.NewSyncPricesTask(job.SyncPricesPayload{}, retryOpts)
	if err != nil {
		return nil, err
	}
	billingDispatch, err := job.NewBillingDispatchTask(retryOpts)
	if err != nil {
		return nil, err
	}
	demoExtend, err := job.NewDemoExtendTask(retryOpts)
	if err != nil {
		return nil, err
	}
	return []Entry{
		{Cron: "@every 1h", Task: noop},
		{Cron: s.cfg.Schedule.Ingestion, Task: syncDispatch},
		{Cron: s.cfg.Schedule.EPIAS, Task: syncPrices},
		{Cron: s.cfg.Schedule.Billing, Task: billingDispatch},
		{Cron: s.cfg.Schedule.Demo, Task: demoExtend},
	}, nil
}

// redisPingTimeout bounds newSchedulerRedisClient's own connectivity check —
// short, since it only needs to prove ONE round trip works, not wait out a
// slow broker.
const redisPingTimeout = 5 * time.Second

// newSchedulerRedisClient opens a caller-owned goredis client pointed at
// the same logical database asynq's own RedisClientOpt would use
// (job.RedisOpt's own parsing, duplicated here rather than shared: job
// package builds an asynq.RedisClientOpt, not a *goredis.Client, and this
// scheduler needs the latter for asynq.NewSchedulerFromRedisClient — see
// runTerm), and pings it once before returning — go-redis dials lazily on
// first command otherwise, which would let a term "succeed" in opening a
// client against an unreachable broker and only fail later, silently,
// inside asynq's own background heartbeat. The caller closes the returned
// client; this function closes it itself only on its own ping-failure path.
func newSchedulerRedisClient(ctx context.Context, cfg config.Redis) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, secret.URLParseErr("parse redis url")
	}
	opts.DB = cfg.QueueDB

	client := goredis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, redisPingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, secret.Wrap("ping redis", secret.Fragments(opts.Password), err)
	}
	return client, nil
}

// Run campaigns for leadership and schedules tasks while leading. It blocks
// until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	entries, err := s.entries()
	if err != nil {
		return err
	}

	return s.elector.Run(ctx, func(leadCtx context.Context) error {
		return s.runTerm(leadCtx, entries)
	})
}

// runTerm runs one leadership term: it opens its own goredis client, builds
// a fresh *asynq.Scheduler from it (asynq schedulers cannot be restarted —
// see TestSchedulerBuildsAFreshAsynqSchedulerEveryTerm), registers every
// entry, runs until leadCtx is cancelled, and shuts the asynq scheduler
// down. Carried-forward defect 3 (F1 plan): the goredis client this term
// opened is closed on EVERY return path via defer — including a bad cron
// entry failing Register, or Start failing — so a term that never gets to
// "running" no longer leaks a pooled Redis client. asynq.NewSchedulerFromRedisClient's
// own doc is explicit that it never closes the client it is given
// ("Warning: The underlying redis connection pool will not be closed by
// Asynq, you are responsible for closing it") — asynq.NewScheduler's
// opt-based form manages this internally, which is exactly why switching to
// the RedisClient form (needed so this method, not asynq, controls the
// client's lifetime) requires taking over that responsibility explicitly.
func (s *Scheduler) runTerm(leadCtx context.Context, entries []Entry) error {
	redisClient, err := newSchedulerRedisClient(leadCtx, s.cfg.Redis)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	asynqScheduler := asynq.NewSchedulerFromRedisClient(redisClient, &asynq.SchedulerOpts{
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
}

// schedulerLogger adapts slog to asynq's logger interface.
type schedulerLogger struct{ log *slog.Logger }

func (l schedulerLogger) Debug(args ...any) { l.log.Debug(fmt.Sprintln(args...)) }
func (l schedulerLogger) Info(args ...any)  { l.log.Info(fmt.Sprintln(args...)) }
func (l schedulerLogger) Warn(args ...any)  { l.log.Warn(fmt.Sprintln(args...)) }
func (l schedulerLogger) Error(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
func (l schedulerLogger) Fatal(args ...any) { l.log.Error(fmt.Sprintln(args...)) }
