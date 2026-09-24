package solar

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// A final failure (auth, inactive credential) archives the sync task under
// its fixed id; the scheduled dispatch must replace it, or the plant never
// syncs again on schedule (R288).

type onePlant struct{}

func (onePlant) LinkedPlants(context.Context) ([]store.CompanyPlant, error) {
	return []store.CompanyPlant{{CompanyID: uuid.New(), PlantID: uuid.New()}}, nil
}

// conflictOnce answers the first enqueue with a TaskID conflict.
type conflictOnce struct{ calls int }

func (c *conflictOnce) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	c.calls++
	if c.calls == 1 {
		return nil, asynq.ErrTaskIDConflict
	}
	return &asynq.TaskInfo{ID: t.Type()}, nil
}

type archivedInspector struct{ deleted int }

func (a *archivedInspector) GetTaskInfo(string, string) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{State: asynq.TaskStateArchived}, nil
}

func (a *archivedInspector) DeleteTask(string, string) error { a.deleted++; return nil }

func TestDispatchReplacesAnArchivedSync(t *testing.T) {
	enq, insp := &conflictOnce{}, &archivedInspector{}
	n, err := New(Deps{AdminSolar: onePlant{}, Enqueuer: enq, Inspector: insp}).DispatchSync(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, insp.deleted)
	require.Equal(t, 2, enq.calls, "the archived task is replaced by a fresh one")
}

func TestDispatchWithoutInspectorFails(t *testing.T) {
	_, err := New(Deps{AdminSolar: onePlant{}, Enqueuer: &conflictOnce{}}).DispatchSync(context.Background())
	require.Error(t, err, "without an inspector an archived sync would be skipped silently")
}
