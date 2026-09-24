package report_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// fakeReports is an in-memory ReportRepository keyed like the table.
type fakeReports struct {
	store.ReportRepository
	mu   sync.Mutex
	rows map[uuid.UUID]model.Report
}

func newReports(rows ...model.Report) *fakeReports {
	f := &fakeReports{rows: map[uuid.UUID]model.Report{}}
	for _, r := range rows {
		f.rows[r.ID] = r
	}
	return f
}

func (f *fakeReports) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		return r, nil
	}
	return model.Report{}, store.ErrNotFound
}

func (f *fakeReports) List(_ context.Context, _ store.Scope, flt store.ReportFilter) ([]model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.Report
	for _, r := range f.rows {
		if (flt.BuildingID == nil || r.BuildingID == *flt.BuildingID) && (flt.Type == nil || r.Type == *flt.Type) &&
			(flt.Period == nil || r.Period == *flt.Period) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeReports) Upsert(_ context.Context, sc store.Scope, rp model.Report) (model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, r := range f.rows {
		if r.BuildingID == rp.BuildingID && r.Type == rp.Type && r.Period == rp.Period {
			rp.ID = id
		}
	}
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	rp.CompanyID = sc.CompanyID
	f.rows[rp.ID] = rp
	return rp, nil
}

func (f *fakeReports) UpdateStatus(_ context.Context, _ store.Scope, id uuid.UUID, status model.ReportStatus, msg *string, _ time.Time) (model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return model.Report{}, store.ErrNotFound
	}
	r.Status, r.ErrorMessage = status, msg
	f.rows[id] = r
	return r, nil
}

type fakeEnqueuer struct{ tasks []*asynq.Task }

func (f *fakeEnqueuer) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.tasks = append(f.tasks, t)
	return &asynq.TaskInfo{ID: "id"}, nil
}

type fakeOps struct {
	store.OpsRepository
	runs     []model.JobRun
	finished []string
	details  []string
	messages []model.OperationalMessage
}

func (f *fakeOps) StartRun(_ context.Context, _ store.Scope, run model.JobRun) (model.JobRun, error) {
	run.ID = uuid.New()
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeOps) FinishRun(_ context.Context, _ store.Scope, _ uuid.UUID, status string, _, _, _ int32, _ *string, detail []byte, _ time.Time) (model.JobRun, error) {
	f.finished = append(f.finished, status)
	f.details = append(f.details, string(detail))
	return model.JobRun{}, nil
}

func (f *fakeOps) AppendMessage(_ context.Context, _ store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	f.messages = append(f.messages, m)
	return m, nil
}

type fakeSMTP struct {
	store.SMTPRepository
	missing  bool
	password string
}

func (f fakeSMTP) Get(context.Context, store.Scope) (model.SMTPSettings, error) {
	if f.missing {
		return model.SMTPSettings{}, store.ErrNotFound
	}
	return model.SMTPSettings{Host: "smtp.test", Port: 587, FromAddress: "rapor@ekokod.test"}, nil
}

func (f fakeSMTP) OpenPassword(context.Context, store.Scope) ([]byte, error) {
	return []byte(f.password), nil
}

type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, _ model.SMTPSettings, _ []byte, m mail.Message) error {
	f.sent = append(f.sent, m)
	return f.err
}

type fakeCompanies struct{ store.CompanyRepository }

func (fakeCompanies) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Company, error) {
	return model.Company{ID: id, Name: "Ekokod A.Ş."}, nil
}

var now = time.Date(2026, 3, 2, 3, 0, 0, 0, time.UTC) // 06:00 Istanbul, the monthly tick's day

func requests(t *testing.T, reports *fakeReports, q *fakeEnqueuer) report.Requests {
	t.Helper()
	var e report.TaskEnqueuer = q
	if q == nil {
		e = &fakeEnqueuer{}
	}
	return report.Requests{Service: newService(t), Reports: reports, Enqueuer: e, Clock: clock.NewFake(now),
		Files: report.Files{Root: t.TempDir(), Companies: fakeCompanies{}, Buildings: fakeBuildings{visible: map[uuid.UUID]string{buildingA: "Merkez"}}}}
}

func completedReport(t *testing.T) model.Report {
	t.Helper()
	raw, err := json.Marshal(samplePayload())
	require.NoError(t, err)
	subject, body := "Merkez - Aylık Enerji Raporu - Mart 2026", "<p>özet</p>"
	return model.Report{ID: uuid.New(), CompanyID: companyA, BuildingID: buildingA, Type: model.ReportTypeMonthly, Period: "2026-03",
		PlantSelection: model.PlantSelectionAll, Payload: raw, Status: model.ReportStatusCompleted, EmailSubject: &subject, EmailBody: &body}
}

func TestEnqueueMarksExistingReportPending(t *testing.T) {
	t.Parallel()
	existing := completedReport(t)
	reports, q := newReports(existing), &fakeEnqueuer{}
	out, err := requests(t, reports, q).Enqueue(t.Context(), store.SystemScope(companyA), report.Request{
		Type: domain.TypeMonthly, Period: "2026-03", Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{buildingA}})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, existing.ID, out[0].ReportID, "the same row, not a second one")
	require.Equal(t, job.ReportGenerateTaskID(job.ReportGeneratePayload{BuildingID: buildingA, Type: "monthly", Period: "2026-03"}), out[0].JobID)
	got := reports.rows[existing.ID]
	require.Equal(t, model.ReportStatusPending, got.Status)
	require.JSONEq(t, string(existing.Payload), string(got.Payload), "the old payload stays readable while pending (R264)")
	require.Len(t, q.tasks, 1)
}

func TestEnqueueCreatesPendingRow(t *testing.T) {
	t.Parallel()
	reports, q := newReports(), &fakeEnqueuer{}
	out, err := requests(t, reports, q).Enqueue(t.Context(), store.SystemScope(companyA), report.Request{
		Type: domain.TypeYearly, Period: "2025", Selection: domain.SelectionGrid, BuildingIDs: []uuid.UUID{buildingA}})
	require.NoError(t, err)
	row := reports.rows[out[0].ReportID]
	require.Equal(t, model.ReportStatusPending, row.Status)
	require.Equal(t, model.PlantSelectionGrid, row.PlantSelection)
	require.JSONEq(t, `{}`, string(row.Payload))
}

func TestEnqueueRefusesInvisibleBuilding(t *testing.T) {
	t.Parallel()
	reports, q := newReports(), &fakeEnqueuer{}
	_, err := requests(t, reports, q).Enqueue(t.Context(), store.SystemScope(companyA), report.Request{
		Type: domain.TypeMonthly, Period: "2026-03", Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{buildingA, buildingB}})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Empty(t, q.tasks, "nothing is enqueued for any building")
	require.Empty(t, reports.rows, "nothing is written for any building")
}

func TestEnqueueDeliveryValidatesRecipients(t *testing.T) {
	t.Parallel()
	existing := completedReport(t)
	many := make([]string, 11)
	for i := range many {
		many[i] = "a@b.test"
	}
	for name, to := range map[string][]string{
		"none": nil, "eleven": many, "line break": {"a@b.test\r\nBcc: x@evil.test"}, "malformed": {"not an address"},
	} {
		q := &fakeEnqueuer{}
		_, err := requests(t, newReports(existing), q).EnqueueDelivery(t.Context(), store.SystemScope(companyA), existing.ID, to)
		require.Error(t, err, name)
		require.Empty(t, q.tasks, name)
	}
	q := &fakeEnqueuer{}
	id, err := requests(t, newReports(existing), q).EnqueueDelivery(t.Context(), store.SystemScope(companyA), existing.ID, []string{"a@b.test"})
	require.NoError(t, err)
	require.Contains(t, id, "report.deliver:"+existing.ID.String()+":")
	_, err = requests(t, newReports(existing), q).EnqueueDelivery(t.Context(), store.SystemScope(companyA), uuid.New(), []string{"a@b.test"})
	require.ErrorIs(t, err, store.ErrNotFound)
}

func deliverer(t *testing.T, reports *fakeReports, ops *fakeOps, smtp fakeSMTP, sender *fakeSender) report.Deliverer {
	t.Helper()
	return report.Deliverer{Reports: reports, Ops: ops, SMTP: smtp, Mail: sender, Clock: clock.NewFake(now),
		Files: report.Files{Root: t.TempDir(), Companies: fakeCompanies{}, Buildings: fakeBuildings{visible: map[uuid.UUID]string{buildingA: "Merkez"}}}}
}

func deliver(rp model.Report) job.ReportDeliverPayload {
	return job.ReportDeliverPayload{CompanyID: companyA, ReportID: rp.ID, To: []string{"yonetici@firma.test"}, RequestID: uuid.New()}
}

func TestDeliverNotReady(t *testing.T) {
	t.Parallel()
	rp := completedReport(t)
	rp.Status = model.ReportStatusPending
	ops, sender := &fakeOps{}, &fakeSender{}
	require.NoError(t, deliverer(t, newReports(rp), ops, fakeSMTP{}, sender).Deliver(t.Context(), deliver(rp)))
	require.Empty(t, sender.sent)
	require.Equal(t, []string{"failed"}, ops.finished)
	require.Contains(t, ops.details[0], `"report_not_ready"`)
}

func TestDeliverWithoutSMTPRecordsMessage(t *testing.T) {
	t.Parallel()
	rp := completedReport(t)
	ops, sender := &fakeOps{}, &fakeSender{}
	require.NoError(t, deliverer(t, newReports(rp), ops, fakeSMTP{missing: true}, sender).Deliver(t.Context(), deliver(rp)),
		"a delivery failure does not retry (R222)")
	require.Empty(t, sender.sent)
	require.Contains(t, ops.details[0], `"smtp_not_configured"`)
	require.Len(t, ops.messages, 1)
	require.Equal(t, "error", ops.messages[0].Status)
	require.Equal(t, "report-delivery", ops.messages[0].Category)
}

func TestDeliverFailureIsScrubbed(t *testing.T) {
	t.Parallel()
	rp := completedReport(t)
	ops := &fakeOps{}
	sender := &fakeSender{err: errors.New("535 authentication failed for s3cretPass!")}
	require.NoError(t, deliverer(t, newReports(rp), ops, fakeSMTP{password: "s3cretPass!"}, sender).Deliver(t.Context(), deliver(rp)))
	require.Contains(t, ops.details[0], `"delivery_failed"`)
	require.Len(t, ops.messages, 1)
	require.NotContains(t, ops.messages[0].Message, "s3cretPass!", "an SMTP reply can quote the password")
}

func TestDeliverAttachesPDFAndExcel(t *testing.T) {
	t.Parallel()
	rp := completedReport(t)
	ops, sender := &fakeOps{}, &fakeSender{}
	require.NoError(t, deliverer(t, newReports(rp), ops, fakeSMTP{}, sender).Deliver(t.Context(), deliver(rp)))
	require.Len(t, sender.sent, 1)
	m := sender.sent[0]
	require.Equal(t, []string{"yonetici@firma.test"}, m.To)
	require.Equal(t, *rp.EmailSubject, m.Subject)
	require.Equal(t, *rp.EmailBody, m.HTML)
	require.NotEmpty(t, m.Text, "a plain-text alternative")
	require.Len(t, m.Attachments, 2)
	require.Equal(t, "rapor-2026-03.pdf", m.Attachments[0].Filename)
	require.Equal(t, "application/pdf", m.Attachments[0].ContentType)
	require.Equal(t, "rapor-2026-03.xlsx", m.Attachments[1].Filename)
	require.NotEmpty(t, m.Attachments[0].Data)
	require.Equal(t, []string{"success"}, ops.finished)
	require.Equal(t, job.TypeReportDeliver, ops.runs[0].JobType)
	require.NotNil(t, ops.runs[0].TaskID, "R267: the run is found by its task id")
}

type fakeBillable struct{ store.AdminBillingRepository }

func (fakeBillable) BillableBuildings(context.Context) ([]store.BillableBuilding, error) {
	return []store.BillableBuilding{{CompanyID: companyA, BuildingID: buildingA, CutoffDay: 1}, {CompanyID: companyA, BuildingID: buildingB, CutoffDay: 15}}, nil
}

func dispatched(t *testing.T, kind string, at time.Time) []job.ReportGeneratePayload {
	t.Helper()
	q := &fakeEnqueuer{}
	require.NoError(t, report.Dispatcher{Billable: fakeBillable{}, Enqueuer: q, Clock: clock.NewFake(at)}.Dispatch(t.Context(), kind))
	var out []job.ReportGeneratePayload
	for _, task := range q.tasks {
		var p job.ReportGeneratePayload
		require.NoError(t, json.Unmarshal(task.Payload(), &p))
		out = append(out, p)
	}
	return out
}

func TestDispatchMonthlyTargetsPreviousMonth(t *testing.T) {
	t.Parallel()
	got := dispatched(t, "monthly", now)
	require.Len(t, got, 2, "every building, whatever its cutoff day")
	for _, p := range got {
		require.Equal(t, "monthly", p.Type)
		require.Equal(t, "2026-02", p.Period, "the closed month, never the running one (R268)")
		require.Equal(t, domain.SelectionAll, p.PlantSelection)
	}
	jan := dispatched(t, "monthly", time.Date(2026, 1, 1, 22, 0, 0, 0, time.UTC)) // 2 January, 01:00 Istanbul
	require.Equal(t, "2025-12", jan[0].Period, "Istanbul's calendar, not UTC's")
}

func TestDispatchYearlyTargetsPreviousYear(t *testing.T) {
	t.Parallel()
	got := dispatched(t, "yearly", time.Date(2026, 1, 3, 4, 0, 0, 0, time.UTC))
	require.Len(t, got, 2)
	require.Equal(t, "2025", got[0].Period)
	require.Equal(t, "yearly", got[0].Type)
}

func TestFilesReadRefusesPathOutsideCompanyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir() + "/secret.pdf"
	require.NoError(t, os.WriteFile(outside, []byte("SECRET"), 0o600))
	inside := root + "/reports/" + companyA.String() + "/../../../" + filepath.Base(filepath.Dir(outside)) + "/secret.pdf"
	rp := completedReport(t)
	files := report.Files{Root: root, Companies: fakeCompanies{}, Buildings: fakeBuildings{visible: map[uuid.UUID]string{buildingA: "Merkez"}}}
	for _, path := range []string{outside, inside} {
		p := path
		rp.PdfPath = &p
		raw, err := files.Read(t.Context(), rp, report.FormatPDF, "tr")
		require.NoError(t, err)
		require.NotEqual(t, "SECRET", string(raw), "a stored path outside the company's directory is never read: %s", path)
		require.True(t, strings.HasPrefix(string(raw), "%PDF-"), "it is rendered from the payload instead")
	}
	rp.PdfPath = nil
	rp.Payload = json.RawMessage(`{}`)
	_, err := files.Read(t.Context(), rp, report.FormatPDF, "tr")
	require.ErrorIs(t, err, report.ErrNotReady)
}

type conflictingQueue struct{ tasks []*asynq.Task }

func (c *conflictingQueue) Enqueue(_ context.Context, t *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	c.tasks = append(c.tasks, t)
	if len(c.tasks) == 1 {
		return nil, asynq.ErrTaskIDConflict
	}
	return &asynq.TaskInfo{ID: "id"}, nil
}

type archivedTasks struct{ deleted int }

func (a *archivedTasks) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{ID: id, Queue: queue, State: asynq.TaskStateArchived}, nil
}

func (a *archivedTasks) DeleteTask(string, string) error { a.deleted++; return nil }

// A failed generation is archived under the same id: regenerating after the
// data is fixed must run again, not be swallowed as a duplicate.
func TestEnqueueRegeneratesAfterAFailedRun(t *testing.T) {
	t.Parallel()
	q, tasks := &conflictingQueue{}, &archivedTasks{}
	req := requests(t, newReports(), nil)
	req.Enqueuer, req.Tasks = q, tasks
	_, err := req.Enqueue(t.Context(), store.SystemScope(companyA), report.Request{
		Type: domain.TypeMonthly, Period: "2026-03", Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{buildingA}})
	require.NoError(t, err)
	require.Equal(t, 1, tasks.deleted)
	require.Len(t, q.tasks, 2, "enqueued again after the archived task was removed")
}
