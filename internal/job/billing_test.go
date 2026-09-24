package job

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestBillingGenerateTaskIDIsDeterministicAndHasNoRetention(t *testing.T) {
	p := BillingGeneratePayload{CompanyID: uuid.New(), Scope: model.BillScopeBuilding, SubjectID: uuid.New(), PeriodKey: "2026-01"}
	require.Equal(t, BillingGenerateTaskID(p), BillingGenerateTaskID(p))
	forced := p
	forced.Force = true
	require.Equal(t, BillingGenerateTaskID(p), BillingGenerateTaskID(forced), "M-10: force keeps the TaskID")
	other := p
	other.PeriodKey = "2025-12"
	require.NotEqual(t, BillingGenerateTaskID(p), BillingGenerateTaskID(other))
	require.Equal(t, "billing.generate:building:"+p.SubjectID.String()+":2026-01", BillingGenerateTaskID(p))

	task, err := NewBillingGenerateTask(p, TaskOptions{MaxRetry: 2})
	require.NoError(t, err)
	decoded := BillingGeneratePayload{}
	require.NoError(t, integDecode(TypeBillingGenerate, task.Payload(), &decoded))
	require.Equal(t, p, decoded)
	_, err = NewBillingGenerateTask(BillingGeneratePayload{}, TaskOptions{})
	require.Error(t, err)
	_, err = NewBillingRenderTask(BillingRenderPayload{}, TaskOptions{})
	require.Error(t, err)
}

type recordingBilling struct{ generated, rendered, dispatched int }

func (r *recordingBilling) Dispatch(context.Context) error { r.dispatched++; return nil }
func (r *recordingBilling) Generate(context.Context, BillingGeneratePayload) error {
	r.generated++
	return errors.New("transient")
}
func (r *recordingBilling) Render(context.Context, BillingRenderPayload) error {
	r.rendered++
	return nil
}

func TestBillingHandlersRegisterOnlyWhenSet(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{})
	_, pattern := mux.Handler(asynq.NewTask(TypeBillingGenerate, nil))
	require.Empty(t, pattern)

	rec := &recordingBilling{}
	mux = asynq.NewServeMux()
	Register(mux, &Handlers{BillingDispatch: rec, BillingGenerate: rec, BillingRender: rec})
	p := BillingGeneratePayload{CompanyID: uuid.New(), Scope: model.BillScopeAnalyzer, SubjectID: uuid.New(), PeriodKey: "2026-01"}
	task, err := NewBillingGenerateTask(p, TaskOptions{})
	require.NoError(t, err)
	err = mux.ProcessTask(context.Background(), task)
	require.Error(t, err)
	require.False(t, errors.Is(err, asynq.SkipRetry), "a transient error is retried")
	require.Equal(t, 1, rec.generated)

	bad := asynq.NewTask(TypeBillingGenerate, []byte(`{"unknown":1}`))
	require.ErrorIs(t, mux.ProcessTask(context.Background(), bad), asynq.SkipRetry, "a malformed payload is not retried")
}
