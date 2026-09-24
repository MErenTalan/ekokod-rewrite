package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Report task types (01 §8, 03 §4.1–§4.2, R268).
const (
	TypeReportDispatchMonthly = "report.dispatch_monthly"
	TypeReportDispatchYearly  = "report.dispatch_yearly"
	TypeReportGenerate        = "report.generate"
	TypeReportDeliver         = "report.deliver"
)

// ReportGeneratePayload asks for one building's report. Its tags are
// snake_case, like billing.generate's: jobs.Get decodes it by these names.
type ReportGeneratePayload struct {
	CompanyID      uuid.UUID   `json:"company_id"`
	BuildingID     uuid.UUID   `json:"building_id"`
	Type           string      `json:"type"`
	Period         string      `json:"period"`
	PlantSelection string      `json:"plant_selection"`
	PlantIDs       []uuid.UUID `json:"plant_ids"`
}

// ReportDeliverPayload asks for one report to be e-mailed.
type ReportDeliverPayload struct {
	CompanyID uuid.UUID `json:"company_id"`
	ReportID  uuid.UUID `json:"report_id"`
	To        []string  `json:"to"`
	// RequestID makes every send its own task: resending is a new intent.
	RequestID uuid.UUID `json:"request_id"`
}

// ReportGenerateTaskID is one per building, type and period — the report's
// own unique key.
func ReportGenerateTaskID(p ReportGeneratePayload) string {
	return fmt.Sprintf("%s:%s:%s:%s", TypeReportGenerate, p.BuildingID, p.Type, p.Period)
}

// ReportDeliverTaskID is one per send request.
func ReportDeliverTaskID(p ReportDeliverPayload) string {
	return fmt.Sprintf("%s:%s:%s", TypeReportDeliver, p.ReportID, p.RequestID)
}

// NewReportDispatchTask builds the monthly or yearly schedule tick.
func NewReportDispatchTask(kind string, o TaskOptions) (*asynq.Task, error) {
	var taskType string
	switch kind {
	case "monthly":
		taskType = TypeReportDispatchMonthly
	case "yearly":
		taskType = TypeReportDispatchYearly
	default:
		return nil, fmt.Errorf("report.dispatch: unknown kind %q", kind)
	}
	return asynq.NewTask(taskType, nil, append([]asynq.Option{asynq.Timeout(30 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// NewReportGenerateTask builds report.generate. No Retention (R53): a
// finished task is answered from its job_runs row (R267).
func NewReportGenerateTask(p ReportGeneratePayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.BuildingID == uuid.Nil || p.Type == "" || p.Period == "" {
		return nil, fmt.Errorf("report.generate: company, building, type and period are required")
	}
	payload, err := integEncode(TypeReportGenerate, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute), asynq.TaskID(ReportGenerateTaskID(p))}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeReportGenerate, payload, opts...), nil
}

// NewReportDeliverTask builds report.deliver.
func NewReportDeliverTask(p ReportDeliverPayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.ReportID == uuid.Nil || p.RequestID == uuid.Nil || len(p.To) == 0 {
		return nil, fmt.Errorf("report.deliver: company, report, request and a recipient are required")
	}
	payload, err := integEncode(TypeReportDeliver, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(5 * time.Minute), asynq.TaskID(ReportDeliverTaskID(p))}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeReportDeliver, payload, opts...), nil
}

// ReportDispatcher handles the two schedule ticks.
type ReportDispatcher interface {
	Dispatch(ctx context.Context, kind string) error
}

// ReportGenerator handles report.generate.
type ReportGenerator interface {
	Generate(ctx context.Context, p ReportGeneratePayload) error
}

// ReportDeliverer handles report.deliver.
type ReportDeliverer interface {
	Deliver(ctx context.Context, p ReportDeliverPayload) error
}

func (h *Handlers) handleReportDispatchMonthly(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.ReportDispatch.Dispatch(ctx, "monthly"))
}

func (h *Handlers) handleReportDispatchYearly(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.ReportDispatch.Dispatch(ctx, "yearly"))
}

func (h *Handlers) handleReportGenerate(ctx context.Context, task *asynq.Task) error {
	var p ReportGeneratePayload
	if err := integDecode(TypeReportGenerate, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.ReportGenerate.Generate(ctx, p))
}

func (h *Handlers) handleReportDeliver(ctx context.Context, task *asynq.Task) error {
	var p ReportDeliverPayload
	if err := integDecode(TypeReportDeliver, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.ReportDeliver.Deliver(ctx, p))
}
