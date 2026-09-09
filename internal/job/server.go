package job

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/hibiken/asynq"
)

// NewServer builds the worker server and an empty mux. Callers register
// handlers on the mux before running the server.
func NewServer(redisCfg config.Redis, worker config.Worker, log *slog.Logger) (*asynq.Server, *asynq.ServeMux, error) {
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
		ShutdownTimeout: worker.Timeout,
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// Never swallowed: every failure is logged with its task type and
			// surfaced as an operational message from F7 onwards.
			log.ErrorContext(ctx, "task failed",
				slog.String("task_type", task.Type()),
				slog.String("error", err.Error()))
		}),
		Logger: asynqLogger{log: log},
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
