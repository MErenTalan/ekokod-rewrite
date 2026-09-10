package job

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
)

// HealthCheckInterval is how often the worker pings its Redis broker to
// verify it is still reachable. Set explicitly (rather than left zero) so
// the behaviour is documented in one place instead of relying on asynq's
// own default (also 15s, server.go:418 in asynq@v0.26.0).
const HealthCheckInterval = 15 * time.Second

// MaxShutdownGrace bounds how long the worker may take to drain in-flight
// tasks on shutdown. This is deliberately independent of
// config.Worker.Timeout (EKOKOD_JOB_TIMEOUT): that field bounds how long a
// single task may run, not how long the process may take to exit. Passing
// it straight through as asynq's ShutdownTimeout would let a worker
// configured with the 30 minute default block a SIGTERM drain for 30
// minutes — far past Kubernetes' default 30s terminationGracePeriodSeconds
// or docker-compose's 10s stop_grace_period, both of which SIGKILL the
// process anyway once their own grace period elapses, so a longer
// ShutdownTimeout buys nothing but guarantees the ungraceful outcome it was
// meant to avoid. A task that does not finish within this window is
// requeued by asynq, not lost.
const MaxShutdownGrace = 30 * time.Second

// ShutdownTimeout is the ShutdownTimeout NewServer gives asynq: the smaller
// of the configured max task runtime and MaxShutdownGrace. Exported and
// kept as a pure function (no I/O, no broker) so the policy in
// MaxShutdownGrace's doc comment is provable by a unit test even while
// internal/job's own integration test cannot run (Docker is down for this
// phase).
func ShutdownTimeout(taskTimeout time.Duration) time.Duration {
	return min(taskTimeout, MaxShutdownGrace)
}

// NewServer builds the worker server and an empty mux. Callers register
// handlers on the mux before running the server. onHealthCheck, if
// non-nil, is called after every periodic broker ping with the ping's
// result (nil on success). Without a non-nil HealthCheckFunc, asynq does
// not even start its healthcheck goroutine (healthcheck.go:58-61 in
// asynq@v0.26.0), so a caller that wants to learn the broker is
// unreachable — asynq itself never surfaces that as an error once Start
// has succeeded — must supply onHealthCheck.
func NewServer(redisCfg config.Redis, worker config.Worker, log *slog.Logger, onHealthCheck func(error)) (*asynq.Server, *asynq.ServeMux, error) {
	opt, err := RedisOpt(redisCfg)
	if err != nil {
		return nil, nil, err
	}

	server := asynq.NewServer(opt, asynq.Config{
		Concurrency: worker.Concurrency,
		Queues: map[string]int{
			QueueCritical: 6,
			QueueDefault:  3,
			QueueLow:      1,
		},
		ShutdownTimeout: ShutdownTimeout(worker.Timeout),
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// Never swallowed: every failure is logged with its task type and
			// surfaced as an operational message from F7 onwards.
			log.ErrorContext(ctx, "task failed",
				slog.String("task_type", task.Type()),
				slog.String("error", err.Error()))
		}),
		Logger: asynqLogger{log: log},
		HealthCheckFunc: func(err error) {
			if err != nil {
				log.Error("redis broker health check failed", slog.String("error", err.Error()))
			}
			if onHealthCheck != nil {
				onHealthCheck(err)
			}
		},
		HealthCheckInterval: HealthCheckInterval,
	})
	return server, asynq.NewServeMux(), nil
}

// asynqLogger adapts slog to asynq's logger interface.
type asynqLogger struct{ log *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.log.Debug(sprint(args)) }
func (l asynqLogger) Info(args ...any)  { l.log.Info(sprint(args)) }
func (l asynqLogger) Warn(args ...any)  { l.log.Warn(sprint(args)) }
func (l asynqLogger) Error(args ...any) { l.log.Error(sprint(args)) }
func (l asynqLogger) Fatal(args ...any) { l.log.Error(sprint(args)) }

func sprint(args []any) string { return strings.TrimSpace(fmt.Sprintln(args...)) }
