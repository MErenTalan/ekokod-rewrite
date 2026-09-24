package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// MessagesRequest is GET /messages (01 §7.13).
type MessagesRequest struct {
	PageRequest
	Kind   *string    `query:"kind" json:"-" validate:"omitempty,oneof=alarm job system" enum:"alarm,job,system"`
	Status *string    `query:"status" json:"-" validate:"omitempty,oneof=success error warning info" enum:"success,error,warning,info"`
	Q      string     `query:"q" json:"-" validate:"max=200"`
	From   *time.Time `query:"from" json:"-"`
	To     *time.Time `query:"to" json:"-"`
}

// Message is one row of the Messages screen.
type Message struct {
	ID       int64   `json:"id" required:"true"`
	Kind     string  `json:"kind" required:"true" enum:"alarm,job,system"`
	Category string  `json:"category" required:"true"`
	Status   string  `json:"status" required:"true" enum:"success,error,warning,info"`
	Message  string  `json:"message" required:"true"`
	Detail   *string `json:"detail"`
	// RelatedType and RelatedID are §7.13's "related entity" column; both are
	// null for a message about nothing in particular.
	RelatedType *string    `json:"related_type"`
	RelatedID   *uuid.UUID `json:"related_id"`
	CreatedAt   time.Time  `json:"created_at" required:"true"`
}

// JobRunsRequest is GET /job-runs.
type JobRunsRequest struct {
	PageRequest
	JobType *string `query:"job_type" json:"-" validate:"omitempty,max=64"`
	Status  *string `query:"status" json:"-" validate:"omitempty,oneof=running success partial failed" enum:"running,success,partial,failed"`
}

// JobRun is one execution of a background job.
type JobRun struct {
	ID uuid.UUID `json:"id" required:"true"`
	// JobType is the asynq task name, which is also what the trigger takes.
	JobType string `json:"job_type" required:"true"`
	// Scope records what the run covered.
	Scope      json.RawMessage `json:"scope" required:"true"`
	StartedAt  time.Time       `json:"started_at" required:"true"`
	FinishedAt *time.Time      `json:"finished_at"`
	Status     string          `json:"status" required:"true" enum:"running,success,partial,failed"`
	Processed  int32           `json:"processed" required:"true"`
	Skipped    int32           `json:"skipped" required:"true"`
	Failed     int32           `json:"failed" required:"true"`
	Error      *string         `json:"error"`
}

// JobTriggerRequest is POST /job-runs/{type}/trigger. The enum IS R220's
// allow-list, so the generated client cannot offer a type the server refuses.
type JobTriggerRequest struct {
	Type string `path:"type" json:"-" validate:"required,oneof=alarm.evaluate billing.dispatch integration.sync_dispatch epias.sync_prices report.dispatch_monthly report.dispatch_yearly isolar.dispatch_sync isolar.fetch_alarms carbon.daily_accrual forecast.run" required:"true" enum:"alarm.evaluate,billing.dispatch,integration.sync_dispatch,epias.sync_prices,report.dispatch_monthly,report.dispatch_yearly,isolar.dispatch_sync,isolar.fetch_alarms,carbon.daily_accrual,forecast.run"`
}
