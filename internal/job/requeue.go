package job

import (
	"context"
	"errors"

	"github.com/hibiken/asynq"
)

// Enqueuer is the job client seam.
type Enqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// TaskInspector finds and removes a task by id.
type TaskInspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
	DeleteTask(queue, id string) error
}

// EnqueueReplacingFinished enqueues task under its deterministic id. A
// queued or running task with that id is the same request and is left
// alone; a finished one (archived after a failure, or completed and
// retained) would otherwise hold the id and silently swallow a deliberate
// re-run, so it is deleted and the task enqueued again. insp nil keeps the
// plain dedupe.
func EnqueueReplacingFinished(ctx context.Context, e Enqueuer, insp TaskInspector, task *asynq.Task, id string) error {
	_, err := EnqueueOrReplace(ctx, e, insp, task, id)
	return err
}

// EnqueueOrReplace is EnqueueReplacingFinished that also reports whether
// the task was (re-)enqueued: false means a live task already holds id.
func EnqueueOrReplace(ctx context.Context, e Enqueuer, insp TaskInspector, task *asynq.Task, id string) (bool, error) {
	_, err := e.Enqueue(ctx, task)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, asynq.ErrTaskIDConflict) && !errors.Is(err, asynq.ErrDuplicateTask) {
		return false, err
	}
	if insp == nil {
		return false, nil
	}
	for _, queue := range []string{QueueCritical, QueueDefault, QueueLow} {
		info, ierr := insp.GetTaskInfo(queue, id)
		if ierr != nil {
			continue
		}
		if info.State != asynq.TaskStateArchived && info.State != asynq.TaskStateCompleted {
			return false, nil
		}
		if err := insp.DeleteTask(queue, id); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
			return false, err
		}
		_, err := e.Enqueue(ctx, task)
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			return false, nil // a concurrent request re-enqueued it first
		}
		return err == nil, err
	}
	return false, nil
}
