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
}
