package job

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func reportPayload() ReportGeneratePayload {
	return ReportGeneratePayload{CompanyID: uuid.New(), BuildingID: uuid.New(), Type: "monthly", Period: "2026-03",
		PlantSelection: "all", PlantIDs: []uuid.UUID{uuid.New()}}
}

func TestReportTaskIDsAreDeterministic(t *testing.T) {
	p := reportPayload()
	require.Equal(t, "report.generate:"+p.BuildingID.String()+":monthly:2026-03", ReportGenerateTaskID(p))
	other := p
	other.PlantSelection = "grid"
	require.Equal(t, ReportGenerateTaskID(p), ReportGenerateTaskID(other), "one report per building, type and period")
	other.Period = "2026-04"
	require.NotEqual(t, ReportGenerateTaskID(p), ReportGenerateTaskID(other))
}

func TestReportDeliverTaskIDsDifferPerRequest(t *testing.T) {
	p := ReportDeliverPayload{CompanyID: uuid.New(), ReportID: uuid.New(), To: []string{"a@b.test"}, RequestID: uuid.New()}
	again := p
	again.RequestID = uuid.New()
	require.NotEqual(t, ReportDeliverTaskID(p), ReportDeliverTaskID(again), "each send is a new intent (R266)")
	require.Equal(t, "report.deliver:"+p.ReportID.String()+":"+p.RequestID.String(), ReportDeliverTaskID(p))
}

func TestReportPayloadRoundTrip(t *testing.T) {
	p := reportPayload()
	task, err := NewReportGenerateTask(p, TaskOptions{MaxRetry: 2})
	require.NoError(t, err)
	var got ReportGeneratePayload
	require.NoError(t, integDecode(TypeReportGenerate, task.Payload(), &got))
	require.Equal(t, p, got)
	require.Contains(t, string(task.Payload()), `"building_id"`, "snake_case like billing.generate (jobs.Get decodes it)")

	d := ReportDeliverPayload{CompanyID: uuid.New(), ReportID: uuid.New(), To: []string{"a@b.test"}, RequestID: uuid.New()}
	dt, err := NewReportDeliverTask(d, TaskOptions{})
	require.NoError(t, err)
	var gotD ReportDeliverPayload
	require.NoError(t, integDecode(TypeReportDeliver, dt.Payload(), &gotD))
	require.Equal(t, d, gotD)

	_, err = NewReportGenerateTask(ReportGeneratePayload{}, TaskOptions{})
	require.Error(t, err)
	_, err = NewReportDeliverTask(ReportDeliverPayload{CompanyID: uuid.New(), ReportID: uuid.New()}, TaskOptions{})
	require.Error(t, err, "no recipient")
	_, err = NewReportDispatchTask("weekly", TaskOptions{})
	require.Error(t, err)
}

type recordingReports struct {
	kinds           []string
	generated, sent int
}

func (r *recordingReports) Dispatch(_ context.Context, kind string) error {
	r.kinds = append(r.kinds, kind)
	return nil
}
func (r *recordingReports) Generate(context.Context, ReportGeneratePayload) error {
	r.generated++
	return errors.New("transient")
}
func (r *recordingReports) Deliver(context.Context, ReportDeliverPayload) error { r.sent++; return nil }

func TestRegisterRoutesReportTypes(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{})
	_, pattern := mux.Handler(asynq.NewTask(TypeReportGenerate, nil))
	require.Empty(t, pattern, "no handler without a generator")

	rec := &recordingReports{}
	mux = asynq.NewServeMux()
	Register(mux, &Handlers{ReportDispatch: rec, ReportGenerate: rec, ReportDeliver: rec})
	for _, kind := range []string{"monthly", "yearly"} {
		task, err := NewReportDispatchTask(kind, TaskOptions{})
		require.NoError(t, err)
		require.NoError(t, mux.ProcessTask(context.Background(), task))
	}
	require.Equal(t, []string{"monthly", "yearly"}, rec.kinds)

	task, err := NewReportGenerateTask(reportPayload(), TaskOptions{})
	require.NoError(t, err)
	err = mux.ProcessTask(context.Background(), task)
	require.Error(t, err)
	require.False(t, errors.Is(err, asynq.SkipRetry))
	require.Equal(t, 1, rec.generated)

	dt, err := NewReportDeliverTask(ReportDeliverPayload{CompanyID: uuid.New(), ReportID: uuid.New(), To: []string{"a@b.test"}, RequestID: uuid.New()}, TaskOptions{})
	require.NoError(t, err)
	require.NoError(t, mux.ProcessTask(context.Background(), dt))
	require.Equal(t, 1, rec.sent)

	bad := asynq.NewTask(TypeReportGenerate, []byte(`{"unknown":1}`))
	require.ErrorIs(t, mux.ProcessTask(context.Background(), bad), asynq.SkipRetry)
}
