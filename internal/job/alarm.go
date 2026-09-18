package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Alarm task types (01 §8's hourly alarm check, split the way F2 and F4 split
// theirs: a platform tick that fans out, one task per company, one per event).
const (
	TypeAlarmDispatch = "alarm.dispatch"
	TypeAlarmEvaluate = "alarm.evaluate"
	TypeAlarmNotify   = "alarm.notify"
)

// AlarmEvaluatePayload asks for one company's rules to be evaluated.
type AlarmEvaluatePayload struct {
	CompanyID uuid.UUID `json:"company_id"`
	// Hour is the tick this evaluation belongs to, truncated to the hour. It
	// is the TaskID's suffix (R225), so a manual trigger inside the cron
	// tick's own hour is de-duplicated by asynq rather than notifying twice.
	Hour time.Time `json:"hour"`
}

// AlarmNotifyPayload asks for one event to be delivered.
type AlarmNotifyPayload struct {
	CompanyID  uuid.UUID  `json:"company_id"`
	AlarmID    uuid.UUID  `json:"alarm_id"`
	EventID    uuid.UUID  `json:"event_id"`
	AnalyzerID *uuid.UUID `json:"analyzer_id,omitempty"`
}

// AlarmEvaluateTaskID is one evaluation per company per hour (R225).
func AlarmEvaluateTaskID(p AlarmEvaluatePayload) string {
	return fmt.Sprintf("%s:%s:%s", TypeAlarmEvaluate, p.CompanyID, p.Hour.UTC().Truncate(time.Hour).Format(time.RFC3339))
}

// AlarmNotifyTaskID is at most one delivery per event.
func AlarmNotifyTaskID(p AlarmNotifyPayload) string {
	return fmt.Sprintf("%s:%s", TypeAlarmNotify, p.EventID)
}

// NewAlarmDispatchTask builds the platform-wide hourly tick.
func NewAlarmDispatchTask(o TaskOptions) (*asynq.Task, error) {
	return asynq.NewTask(TypeAlarmDispatch, nil,
		append([]asynq.Option{asynq.Timeout(30 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// NewAlarmEvaluateTask builds one company's evaluation. The payload's Hour is
// truncated here, so two callers naming different minutes of the same hour
// produce the same TaskID.
func NewAlarmEvaluateTask(p AlarmEvaluatePayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.Hour.IsZero() {
		return nil, fmt.Errorf("alarm.evaluate: company and hour are required")
	}
	p.Hour = p.Hour.UTC().Truncate(time.Hour)
	payload, err := integEncode(TypeAlarmEvaluate, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute), asynq.TaskID(AlarmEvaluateTaskID(p))},
		integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeAlarmEvaluate, payload, opts...), nil
}

// NewAlarmNotifyTask builds one event's delivery.
func NewAlarmNotifyTask(p AlarmNotifyPayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.AlarmID == uuid.Nil || p.EventID == uuid.Nil {
		return nil, fmt.Errorf("alarm.notify: company, alarm and event are required")
	}
	payload, err := integEncode(TypeAlarmNotify, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(2 * time.Minute), asynq.TaskID(AlarmNotifyTaskID(p))},
		integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeAlarmNotify, payload, opts...), nil
}

// AlarmDispatcher handles alarm.dispatch.
type AlarmDispatcher interface {
	Dispatch(ctx context.Context) error
}

// AlarmEvaluator handles alarm.evaluate.
type AlarmEvaluator interface {
	Evaluate(ctx context.Context, p AlarmEvaluatePayload) error
}

// AlarmNotifier handles alarm.notify.
type AlarmNotifier interface {
	Notify(ctx context.Context, p AlarmNotifyPayload) error
}

func (h *Handlers) handleAlarmDispatch(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.AlarmDispatch.Dispatch(ctx))
}

func (h *Handlers) handleAlarmEvaluate(ctx context.Context, task *asynq.Task) error {
	var p AlarmEvaluatePayload
	if err := integDecode(TypeAlarmEvaluate, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.AlarmEvaluate.Evaluate(ctx, p))
}

func (h *Handlers) handleAlarmNotify(ctx context.Context, task *asynq.Task) error {
	var p AlarmNotifyPayload
	if err := integDecode(TypeAlarmNotify, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.AlarmNotify.Notify(ctx, p))
}
