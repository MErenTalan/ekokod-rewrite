package dto

import "time"

// JobIDPath is the asynq task id in the path; it is not a UUID.
type JobIDPath struct {
	ID string `path:"id" json:"-" validate:"required,max=128"`
}

// Job is R192: the coarse state of a background job a screen started. The
// worker's own error text is never returned — it may quote a provider.
type Job struct {
	ID          string     `json:"id" required:"true"`
	Type        string     `json:"type" required:"true"`
	Status      string     `json:"status" required:"true" enum:"queued,running,succeeded,failed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// ErrorCode is R113's closed set for a failed bill computation, so the
	// bills screen can print §7.10's sentence. Never free text (R237).
	ErrorCode string `json:"error_code,omitempty" enum:"tariff_not_found,no_consumption_data,unresolved_anomaly,period_not_closed,ptf_data_missing,billing_parameters_missing,billing_parameters_invalid,report_building_not_found,report_not_ready,smtp_not_configured,delivery_failed,isolar_auth,isolar_unavailable,isolar_not_linked,credential_missing,credential_inactive"`
	// Children is what a backfill's or analyzer refresh's fetch tasks did.
	Children *JobChildren `json:"children,omitempty"`
}

// JobChildren summarises the fetch tasks a parent job enqueued.
type JobChildren struct {
	Enqueued      *int32          `json:"enqueued,omitempty"`
	AlreadyQueued int32           `json:"already_queued" required:"true"`
	Running       int32           `json:"running" required:"true"`
	Succeeded     int32           `json:"succeeded" required:"true"`
	Partial       int32           `json:"partial" required:"true"`
	Failed        int32           `json:"failed" required:"true"`
	Errors        []JobChildError `json:"errors" required:"true"`
}

// JobChildError is one distinct failure and how many tasks hit it.
type JobChildError struct {
	Message string `json:"message" required:"true"`
	Count   int32  `json:"count" required:"true"`
}

// JobQueue is one background queue's current load (admin only).
type JobQueue struct {
	Queue          string `json:"queue" required:"true"`
	Pending        int    `json:"pending" required:"true"`
	Active         int    `json:"active" required:"true"`
	Scheduled      int    `json:"scheduled" required:"true"`
	Retry          int    `json:"retry" required:"true"`
	Archived       int    `json:"archived" required:"true"`
	Completed      int    `json:"completed" required:"true"`
	ProcessedToday int    `json:"processed_today" required:"true"`
	FailedToday    int    `json:"failed_today" required:"true"`
	Paused         bool   `json:"paused" required:"true"`
	LatencySeconds int64  `json:"latency_seconds" required:"true"`
}

// JobQueues is GET /job-queues.
type JobQueues struct {
	Items []JobQueue `json:"items" required:"true"`
}
