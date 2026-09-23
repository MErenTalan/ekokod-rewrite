package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// iSolar task types (03 §4.1, R288).
const (
	TypeSolarDispatchSync = "isolar.dispatch_sync"
	TypeSolarSyncPlant    = "isolar.sync_plant"
	TypeSolarFetchAlarms  = "isolar.fetch_alarms"
)

// SolarSyncPayload asks for one plant's sync; BackfillDays > 0 also fetches
// that many past days from the daily series (R281). Snake_case: jobs.Get reads it.
type SolarSyncPayload struct {
	CompanyID    uuid.UUID `json:"company_id"`
	PlantID      uuid.UUID `json:"plant_id"`
	BackfillDays int       `json:"backfill_days"`
}

// SolarSyncTaskID is one per plant and backfill size (R288).
func SolarSyncTaskID(p SolarSyncPayload) string {
	return fmt.Sprintf("%s:%s:%d", TypeSolarSyncPlant, p.PlantID, p.BackfillDays)
}

// NewSolarDispatchTask builds the sync tick.
func NewSolarDispatchTask(o TaskOptions) (*asynq.Task, error) {
	return asynq.NewTask(TypeSolarDispatchSync, nil, append([]asynq.Option{asynq.Timeout(5 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// NewSolarAlarmsTask builds the fault fetch tick.
func NewSolarAlarmsTask(o TaskOptions) (*asynq.Task, error) {
	return asynq.NewTask(TypeSolarFetchAlarms, nil, append([]asynq.Option{asynq.Timeout(15 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// NewSolarSyncTask builds isolar.sync_plant. No Retention (R53): a finished
// task is answered from its job_runs row (R267).
func NewSolarSyncTask(p SolarSyncPayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.PlantID == uuid.Nil || p.BackfillDays < 0 {
		return nil, fmt.Errorf("isolar.sync_plant: company and plant are required, backfill_days ≥ 0")
	}
	payload, err := integEncode(TypeSolarSyncPlant, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(15 * time.Minute), asynq.TaskID(SolarSyncTaskID(p))}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeSolarSyncPlant, payload, opts...), nil
}

// SolarJobs serves the three iSolar task types.
type SolarJobs interface {
	DispatchSync(ctx context.Context) (int, error)
	SyncPlantTask(ctx context.Context, p SolarSyncPayload) error
	FetchAlarms(ctx context.Context) error
}

func (h *Handlers) handleSolarDispatch(ctx context.Context, _ *asynq.Task) error {
	_, err := h.Solar.DispatchSync(ctx)
	return ClassifyForRetry(err)
}

func (h *Handlers) handleSolarSync(ctx context.Context, task *asynq.Task) error {
	var p SolarSyncPayload
	if err := integDecode(TypeSolarSyncPlant, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Solar.SyncPlantTask(ctx, p))
}

func (h *Handlers) handleSolarAlarms(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.Solar.FetchAlarms(ctx))
}
