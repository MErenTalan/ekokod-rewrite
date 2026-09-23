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
	ErrorCode string `json:"error_code,omitempty" enum:"tariff_not_found,no_consumption_data,unresolved_anomaly,period_not_closed,ptf_data_missing,billing_parameters_missing,billing_parameters_invalid"`
}
