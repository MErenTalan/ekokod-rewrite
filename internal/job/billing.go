package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Billing task types.
const (
	TypeBillingDispatch  = "billing.dispatch"
	TypeBillingGenerate  = "billing.generate"
	TypeBillingRenderPDF = "billing.render_pdf"

	// BillingDispatchLookbackPeriods is how many closed periods each dispatch
	// retries, so a failing or flagged period is not stuck forever (I-8).
	BillingDispatchLookbackPeriods = 3
)

// BillingGeneratePayload asks for one bill.
type BillingGeneratePayload struct {
	CompanyID uuid.UUID       `json:"company_id"`
	Scope     model.BillScope `json:"scope"`
	SubjectID uuid.UUID       `json:"subject_id"`
	PeriodKey string          `json:"period_key"`
	Force     bool            `json:"force"` // M-10: a manual /bills/compute force, same TaskID
}

// BillingRenderPayload asks for one bill's PDF.
type BillingRenderPayload struct {
	CompanyID uuid.UUID `json:"company_id"`
	BillID    uuid.UUID `json:"bill_id"`
}

// BillingGenerateTaskID is deterministic per subject and period.
func BillingGenerateTaskID(p BillingGeneratePayload) string {
	return fmt.Sprintf("%s:%s:%s:%s", TypeBillingGenerate, p.Scope, p.SubjectID, p.PeriodKey)
}

// BillingRenderTaskID is deterministic per bill.
func BillingRenderTaskID(p BillingRenderPayload) string {
	return fmt.Sprintf("%s:%s", TypeBillingRenderPDF, p.BillID)
}

// NewBillingDispatchTask builds the platform-wide billing.dispatch tick.
func NewBillingDispatchTask(o TaskOptions) (*asynq.Task, error) {
	return asynq.NewTask(TypeBillingDispatch, nil, append([]asynq.Option{asynq.Timeout(30 * time.Minute)}, integMaxRetryOptions(o)...)...), nil
}

// NewBillingGenerateTask builds billing.generate with a TaskID and no
// Retention: a retained id would swallow a later legitimate re-run (R53).
func NewBillingGenerateTask(p BillingGeneratePayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.SubjectID == uuid.Nil || !p.Scope.Valid() || p.PeriodKey == "" {
		return nil, fmt.Errorf("billing.generate: company, scope, subject and period are required")
	}
	payload, err := integEncode(TypeBillingGenerate, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute), asynq.TaskID(BillingGenerateTaskID(p))}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeBillingGenerate, payload, opts...), nil
}

// NewBillingRenderTask builds billing.render_pdf.
func NewBillingRenderTask(p BillingRenderPayload, o TaskOptions) (*asynq.Task, error) {
	if p.CompanyID == uuid.Nil || p.BillID == uuid.Nil {
		return nil, fmt.Errorf("billing.render_pdf: company and bill are required")
	}
	payload, err := integEncode(TypeBillingRenderPDF, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(5 * time.Minute), asynq.TaskID(BillingRenderTaskID(p))}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeBillingRenderPDF, payload, opts...), nil
}

// BillingDispatcher handles billing.dispatch.
type BillingDispatcher interface {
	Dispatch(ctx context.Context) error
}

// BillingGenerator handles billing.generate.
type BillingGenerator interface {
	Generate(ctx context.Context, p BillingGeneratePayload) error
}

// BillingRenderer handles billing.render_pdf.
type BillingRenderer interface {
	Render(ctx context.Context, p BillingRenderPayload) error
}

func (h *Handlers) handleBillingDispatch(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.BillingDispatch.Dispatch(ctx))
}

func (h *Handlers) handleBillingGenerate(ctx context.Context, task *asynq.Task) error {
	var p BillingGeneratePayload
	if err := integDecode(TypeBillingGenerate, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.BillingGenerate.Generate(ctx, p))
}

func (h *Handlers) handleBillingRender(ctx context.Context, task *asynq.Task) error {
	var p BillingRenderPayload
	if err := integDecode(TypeBillingRenderPDF, task.Payload(), &p); err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.BillingRender.Render(ctx, p))
}
