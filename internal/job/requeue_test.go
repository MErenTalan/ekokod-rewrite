package job

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type conflictOnce struct {
	calls, conflicts int
}

func (c *conflictOnce) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	c.calls++
	if c.calls <= c.conflicts {
		return nil, asynq.ErrTaskIDConflict
	}
	return &asynq.TaskInfo{ID: "x", Type: t.Type()}, nil
}

type heldBy struct {
	state   asynq.TaskState
	deleted []string
}

func (h *heldBy) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	if queue != QueueDefault {
		return nil, asynq.ErrQueueNotFound
	}
	return &asynq.TaskInfo{ID: id, Queue: queue, State: h.state}, nil
}

func (h *heldBy) DeleteTask(queue, id string) error {
	h.deleted = append(h.deleted, queue+"/"+id)
	return nil
}

func TestEnqueueReplacingFinishedRequeuesAnArchivedTask(t *testing.T) {
	// A failed generation is archived with its deterministic id; without this
	// a regenerate after fixing the data would be silently dropped.
	for _, state := range []asynq.TaskState{asynq.TaskStateArchived, asynq.TaskStateCompleted} {
		e, insp := &conflictOnce{conflicts: 1}, &heldBy{state: state}
		task := asynq.NewTask("report.generate", nil)
		require.NoError(t, EnqueueReplacingFinished(context.Background(), e, insp, task, "report.generate:b:monthly:2026-08"))
		require.Equal(t, []string{QueueDefault + "/report.generate:b:monthly:2026-08"}, insp.deleted, state)
		require.Equal(t, 2, e.calls)
	}
}

func TestEnqueueReplacingFinishedLeavesALiveTaskAlone(t *testing.T) {
	// A queued or running task for the same id is the same request: dedupe.
	for _, state := range []asynq.TaskState{asynq.TaskStatePending, asynq.TaskStateActive, asynq.TaskStateRetry} {
		e, insp := &conflictOnce{conflicts: 1}, &heldBy{state: state}
		require.NoError(t, EnqueueReplacingFinished(context.Background(), e, insp, asynq.NewTask("t", nil), "id"))
		require.Empty(t, insp.deleted, state)
		require.Equal(t, 1, e.calls)
	}
}

func TestEnqueueReplacingFinishedWithoutInspectorDedupes(t *testing.T) {
	e := &conflictOnce{conflicts: 1}
	require.NoError(t, EnqueueReplacingFinished(context.Background(), e, nil, asynq.NewTask("t", nil), "id"))
	require.Equal(t, 1, e.calls)
}
