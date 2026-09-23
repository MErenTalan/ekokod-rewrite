//go:build integration

package report_test

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

type genHarness struct {
	pool   *pgxpool.Pool
	tenant testfixtures.Tenant
	gen    report.Generator
	req    report.Requests
	files  report.Files
}

func newGenHarness(t *testing.T) genHarness {
	t.Helper()
	svc, tenant, _, pool := reportFixture(t)
	files := report.Files{Root: t.TempDir(), Companies: postgres.NewCompanyRepository(pool), Buildings: postgres.NewBuildingRepository(pool)}
	reports := postgres.NewReportRepository(pool)
	clk := clock.NewFake(time.Date(2026, 4, 2, 3, 0, 0, 0, time.UTC))
	return genHarness{pool: pool, tenant: tenant, files: files,
		gen: report.Generator{Service: svc, Reports: reports, Companies: postgres.NewCompanyRepository(pool),
			Ops: postgres.NewOpsRepository(pool), Files: files, Clock: clk},
		req: report.Requests{Service: svc, Reports: reports, Enqueuer: &fakeEnqueuer{}, Files: files, Clock: clk}}
}

func (h genHarness) payload(building uuid.UUID) job.ReportGeneratePayload {
	return job.ReportGeneratePayload{CompanyID: h.tenant.Company.ID, BuildingID: building, Type: domain.TypeMonthly,
		Period: "2026-03", PlantSelection: domain.SelectionAll}
}

func (h genHarness) only(t *testing.T, building uuid.UUID) model.Report {
	t.Helper()
	list, err := postgres.NewReportRepository(h.pool).List(t.Context(), h.tenant.AdminScope, store.ReportFilter{BuildingID: &building})
	require.NoError(t, err)
	require.Len(t, list, 1, "one row per building, type and period")
	return list[0]
}

func TestGenerateStoresFilesAndCompletes(t *testing.T) {
	t.Parallel()
	h := newGenHarness(t)
	b := h.tenant.Buildings[0].ID
	require.NoError(t, h.gen.Generate(t.Context(), h.payload(b)))
	rp := h.only(t, b)
	require.Equal(t, model.ReportStatusCompleted, rp.Status)
	require.NotNil(t, rp.PdfPath)
	require.NotNil(t, rp.ExcelPath)
	pdf, err := os.ReadFile(*rp.PdfPath)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(pdf, []byte("%PDF-")))
	require.Contains(t, *rp.EmailSubject, "Aylık Enerji Raporu - Mart 2026")
	require.Contains(t, *rp.EmailBody, "kWh")
	run, err := postgres.NewOpsRepository(h.pool).RunByTaskID(t.Context(), h.tenant.AdminScope, job.ReportGenerateTaskID(h.payload(b)))
	require.NoError(t, err)
	require.Equal(t, "success", run.Status, "R267: the finished task is answered from this row")
}

func TestGenerateKeepsPreviousFilesWhilePending(t *testing.T) {
	t.Parallel()
	h := newGenHarness(t)
	b := h.tenant.Buildings[0].ID
	require.NoError(t, h.gen.Generate(t.Context(), h.payload(b)))
	before := h.only(t, b)
	stored, err := os.ReadFile(*before.PdfPath)
	require.NoError(t, err)

	_, err = h.req.Enqueue(t.Context(), h.tenant.AdminScope, report.Request{Type: domain.TypeMonthly, Period: "2026-03",
		Selection: domain.SelectionAll, BuildingIDs: []uuid.UUID{b}})
	require.NoError(t, err)
	pending := h.only(t, b)
	require.Equal(t, model.ReportStatusPending, pending.Status)
	require.JSONEq(t, string(before.Payload), string(pending.Payload), "the old figures stay")
	got, err := h.files.Read(t.Context(), pending, report.FormatPDF, "tr")
	require.NoError(t, err)
	require.Equal(t, stored, got, "the old PDF stays downloadable while the new one is prepared")
}

func TestGenerateTwiceKeepsOneRow(t *testing.T) {
	t.Parallel()
	h := newGenHarness(t)
	b := h.tenant.Buildings[0].ID
	require.NoError(t, h.gen.Generate(t.Context(), h.payload(b)))
	first := h.only(t, b)
	require.NoError(t, h.gen.Generate(t.Context(), h.payload(b)))
	second := h.only(t, b)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, *first.PdfPath, *second.PdfPath, "the same file is replaced, not a second one written")
}

func TestGenerateFailureRecordsCode(t *testing.T) {
	t.Parallel()
	h := newGenHarness(t)
	p := h.payload(uuid.New()) // a building nobody owns
	err := h.gen.Generate(t.Context(), p)
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry), "a missing building will not appear on retry")
	run, err := postgres.NewOpsRepository(h.pool).RunByTaskID(t.Context(), h.tenant.AdminScope, job.ReportGenerateTaskID(p))
	require.NoError(t, err)
	require.Equal(t, "failed", run.Status)
	require.JSONEq(t, `{"code":"report_building_not_found"}`, string(run.Detail))
}
