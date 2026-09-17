//go:build integration

package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
}

func (r *recordingEnqueuer) Enqueue(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (r *recordingEnqueuer) generates(t *testing.T) []job.BillingGeneratePayload {
	t.Helper()
	var out []job.BillingGeneratePayload
	for _, task := range r.tasks {
		if task.Type() == job.TypeBillingGenerate {
			var p job.BillingGeneratePayload
			require.NoError(t, json.Unmarshal(task.Payload(), &p))
			out = append(out, p)
		}
	}
	return out
}

func (r *recordingEnqueuer) renders(t *testing.T) []job.BillingRenderPayload {
	t.Helper()
	var out []job.BillingRenderPayload
	for _, task := range r.tasks {
		if task.Type() == job.TypeBillingRenderPDF {
			var p job.BillingRenderPayload
			require.NoError(t, json.Unmarshal(task.Payload(), &p))
			out = append(out, p)
		}
	}
	return out
}

func TestDispatchEnqueuesLastThreeClosedKeys(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7101)
	_, err := h.pool.Exec(h.ctx, `update buildings set bill_cutoff_day = 1 where id = $1`, h.tenant.Buildings[1].ID)
	require.NoError(t, err)
	h.clock.Set(time.Date(2026, 2, 17, 12, 0, 0, 0, h.loc)) // cut-off 15's Jan period settles on Feb 18; cut-off 1's on Feb 4
	enq := &recordingEnqueuer{}
	d := billingsvc.Dispatcher{Billable: admin.NewBillingRepository(h.pool), Bills: h.bills, Enqueuer: enq, Clock: h.clock}
	require.NoError(t, d.Dispatch(h.ctx))

	keys := map[uuid.UUID][]string{}
	for _, p := range enq.generates(t) {
		keys[p.SubjectID] = append(keys[p.SubjectID], p.PeriodKey)
	}
	require.Equal(t, []string{"2025-12", "2025-11", "2025-10"}, keys[h.tenant.Buildings[0].ID], "cut-off 15")
	require.Equal(t, []string{"2026-01", "2025-12", "2025-11"}, keys[h.tenant.Buildings[1].ID], "cut-off 1")
	// TestDispatchCompanyKeyIsMinimumAcrossBuildings.
	require.Equal(t, []string{"2025-12", "2025-11", "2025-10"}, keys[h.tenant.Company.ID])
}

func TestDispatchEnqueuesRenderForLiveBillMissingPdfPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7102)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	enq := &recordingEnqueuer{}
	d := billingsvc.Dispatcher{Billable: admin.NewBillingRepository(h.pool), Bills: h.bills, Enqueuer: enq, Clock: h.clock}
	require.NoError(t, d.Dispatch(h.ctx))
	require.Equal(t, []job.BillingRenderPayload{{CompanyID: h.tenant.Company.ID, BillID: res.Bill.ID}}, enq.renders(t))

	require.NoError(t, h.bills.SetPDFPath(h.ctx, h.tenant.AdminScope, res.Bill.ID, "/x.pdf"))
	enq2 := &recordingEnqueuer{}
	d.Enqueuer = enq2
	require.NoError(t, d.Dispatch(h.ctx))
	require.Empty(t, enq2.renders(t))
}

func (h *harness) jobGenerator(enq billingsvc.TaskEnqueuer) billingsvc.JobGenerator {
	return billingsvc.JobGenerator{Service: h.svc, Ops: h.ops, Enqueuer: enq, Clock: h.clock}
}

func TestGenerateHandlerComputeErrorSkipsRetryAndRecordsRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7103)
	enq := &recordingEnqueuer{}
	p := job.BillingGeneratePayload{CompanyID: h.tenant.Company.ID, Scope: model.BillScopeAnalyzer, SubjectID: h.tenant.Analyzers[0].ID, PeriodKey: "2026-01"}
	err := h.jobGenerator(enq).Generate(h.ctx, p) // no readings
	require.ErrorIs(t, err, asynq.SkipRetry)
	var ce *billingsvc.ComputeError
	require.True(t, errors.As(err, &ce))
	runs, err := h.ops.ListRuns(h.ctx, h.tenant.AdminScope, store.JobRunFilter{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "failed", runs[0].Status)
	require.Contains(t, string(runs[0].Detail), billingsvc.CodeNoConsumptionData)
	require.Empty(t, enq.tasks)
}

func TestGenerateHandlerRecomputesFlaggedNeverIssued(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7104)
	a := h.tenant.Analyzers[0].ID
	h.seed(a, h.window.From, "1000", "", "")
	h.seed(a, h.window.To, "1500", "", "")
	enq := &recordingEnqueuer{}
	g := h.jobGenerator(enq)
	p := job.BillingGeneratePayload{CompanyID: h.tenant.Company.ID, Scope: model.BillScopeAnalyzer, SubjectID: a, PeriodKey: "2026-01"}
	require.NoError(t, g.Generate(h.ctx, p))
	flagged, err := h.bills.Current(h.ctx, h.tenant.Scope, model.BillScopeAnalyzer, a, "2026-01")
	require.NoError(t, err)
	require.Equal(t, model.BillStatusFlagged, flagged.Status)

	h.seedPeriod(a, h.window, "500")
	require.NoError(t, g.Generate(h.ctx, p))
	issued, err := h.bills.Current(h.ctx, h.tenant.Scope, model.BillScopeAnalyzer, a, "2026-01")
	require.NoError(t, err)
	require.Equal(t, model.BillStatusIssued, issued.Status)
	require.NotEqual(t, flagged.ID, issued.ID)

	require.NoError(t, g.Generate(h.ctx, p))
	still, err := h.bills.Current(h.ctx, h.tenant.Scope, model.BillScopeAnalyzer, a, "2026-01")
	require.NoError(t, err)
	require.Equal(t, issued.ID, still.ID, "an issued bill is never recomputed by the job")
	// TestGenerateHandlerEnqueuesRenderOnCreate: one render per created bill.
	require.Len(t, enq.renders(t), 2)
}

func TestRenderHandlerWritesFileAndPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7105)
	for _, a := range h.tenant.Analyzers[:2] {
		h.seedPeriod(a.ID, h.window, "300")
	}
	res, err := h.generate(h.tenant.Scope, model.BillScopeBuilding, h.tenant.Buildings[0].ID)
	require.NoError(t, err)
	root := t.TempDir()
	r := billingsvc.PDFRenderer{Bills: h.bills, Companies: postgres.NewCompanyRepository(h.pool), Buildings: postgres.NewBuildingRepository(h.pool),
		Analyzers: postgres.NewAnalyzerRepository(h.pool), Root: root}
	require.NoError(t, r.Render(h.ctx, job.BillingRenderPayload{CompanyID: h.tenant.Company.ID, BillID: res.Bill.ID}))
	b, err := h.bills.Get(h.ctx, h.tenant.AdminScope, res.Bill.ID)
	require.NoError(t, err)
	require.NotNil(t, b.PdfPath)
	require.Equal(t, root+"/bills/"+h.tenant.Company.ID.String()+"/"+res.Bill.ID.String()+".pdf", *b.PdfPath)
	info, err := os.Stat(*b.PdfPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	content, err := os.ReadFile(*b.PdfPath)
	require.NoError(t, err)
	require.True(t, len(content) > 1000 && string(content[:5]) == "%PDF-")
	entries, err := os.ReadDir(root + "/bills/" + h.tenant.Company.ID.String())
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temp file left behind")
}
