package job

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestSyncPlantTaskIDIsStable(t *testing.T) {
	p := SolarSyncPayload{CompanyID: uuid.New(), PlantID: uuid.New(), BackfillDays: 400}
	require.Equal(t, "isolar.sync_plant:"+p.PlantID.String()+":400", SolarSyncTaskID(p))
	again := p
	again.CompanyID = uuid.New()
	require.Equal(t, SolarSyncTaskID(p), SolarSyncTaskID(again), "one task per plant and backfill (R288)")
	again.BackfillDays = 0
	require.NotEqual(t, SolarSyncTaskID(p), SolarSyncTaskID(again))

	task, err := NewSolarSyncTask(p, TaskOptions{})
	require.NoError(t, err)
	var got SolarSyncPayload
	require.NoError(t, integDecode(TypeSolarSyncPlant, task.Payload(), &got))
	require.Equal(t, p, got)
	require.Contains(t, string(task.Payload()), `"plant_id"`, "snake_case: jobs.Get decodes it")

	_, err = NewSolarSyncTask(SolarSyncPayload{PlantID: uuid.New()}, TaskOptions{})
	require.Error(t, err, "company required")
	_, err = NewSolarSyncTask(SolarSyncPayload{CompanyID: uuid.New(), PlantID: uuid.New(), BackfillDays: -1}, TaskOptions{})
	require.Error(t, err)
}

type recordingSolar struct{ dispatched, synced, fetched int }

func (r *recordingSolar) DispatchSync(context.Context) (int, error) { r.dispatched++; return 0, nil }
func (r *recordingSolar) SyncPlantTask(context.Context, SolarSyncPayload) error {
	r.synced++
	return errors.New("transient")
}
func (r *recordingSolar) FetchAlarms(context.Context) error { r.fetched++; return nil }

func TestRegisterRoutesSolarTypes(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{})
	_, pattern := mux.Handler(asynq.NewTask(TypeSolarSyncPlant, nil))
	require.Empty(t, pattern, "no handler without a syncer")

	rec := &recordingSolar{}
	mux = asynq.NewServeMux()
	Register(mux, &Handlers{Solar: rec})
	tick, err := NewSolarDispatchTask(TaskOptions{})
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(context.Background(), tick))
	alarms, err := NewSolarAlarmsTask(TaskOptions{})
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(context.Background(), alarms))
	task, err := NewSolarSyncTask(SolarSyncPayload{CompanyID: uuid.New(), PlantID: uuid.New()}, TaskOptions{})
	require.NoError(t, err)
	err = mux.ProcessTask(context.Background(), task)
	require.Error(t, err)
	require.False(t, errors.Is(err, asynq.SkipRetry))
	require.Equal(t, recordingSolar{dispatched: 1, synced: 1, fetched: 1}, *rec)

	require.ErrorIs(t, mux.ProcessTask(context.Background(), asynq.NewTask(TypeSolarSyncPlant, []byte(`{"x":1}`))), asynq.SkipRetry)
}
