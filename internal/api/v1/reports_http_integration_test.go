//go:build integration

package v1_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// completedReportFor stores a completed monthly report with a real payload
// and no stored files, so every download renders from the payload.
func (h *harness) completedReportFor(company, building uuid.UUID, period string) string {
	h.t.Helper()
	v := decimal.RequireFromString("1234.5")
	m := domain.Monthly{Year: 2026, Month: 8, DaysInMonth: 31, Selection: domain.SelectionAll,
		Consumption: domain.Figure{Value: &v, WithData: 1, Of: 1}}
	raw, err := json.Marshal(domain.Payload{Version: 1, Type: domain.TypeMonthly, Period: period, Monthly: &m})
	require.NoError(h.t, err)
	subject, body := "Rapor", "<p>Rapor</p>"
	rp, err := postgres.NewReportRepository(h.pool).Upsert(h.t.Context(), store.SystemScope(company), model.Report{
		CompanyID: company, BuildingID: building, Type: model.ReportTypeMonthly, Period: period, PlantSelection: model.PlantSelectionAll,
		Payload: raw, Status: model.ReportStatusCompleted, EmailSubject: &subject, EmailBody: &body, CreatedAt: h.clock.Now(),
	})
	require.NoError(h.t, err)
	return rp.ID.String()
}

func TestReportRoutesHideForeignReports(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.completedReportFor(h.fx.CompanyA, h.fx.BuildingA1, "2026-08")
	foreign := h.completedReportFor(h.fx.CompanyB, h.fx.BuildingB1, "2026-08")
	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusOK, ca.do(http.MethodGet, "/reports/"+own, nil).status)
	for _, path := range []string{"/reports/" + foreign, "/reports/" + foreign + "/pdf", "/reports/" + foreign + "/excel", "/reports/" + uuid.NewString()} {
		res := ca.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusNotFound, res.status, "%s: %s", path, string(res.body))
	}
	res := ca.do(http.MethodPost, "/reports/"+foreign+"/email", map[string]any{"to": []string{"a@b.test"}})
	require.Equal(t, http.StatusNotFound, res.status, string(res.body))
	var page dto.ReportPage
	ca.do(http.MethodGet, "/reports", nil).json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, own, page.Items[0].ID.String())
}

func TestReportPreviewUnknownParamIs400(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	base := "/reports/preview?type=monthly&period=2026-08&plant_selection=all&building_ids=" + h.fx.BuildingA1.String()
	require.Equal(t, http.StatusBadRequest, ca.do(http.MethodGet, base+"&colour=green", nil).status)
	res := ca.do(http.MethodGet, base, nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var p dto.ReportPayload
	res.json(t, &p)
	require.Equal(t, "monthly", p.Type)
	require.NotNil(t, p.Monthly, "/reports/preview must win over /reports/{id}")
	require.Contains(t, string(res.body), `"days_in_month":31`)
	bad := ca.do(http.MethodGet, "/reports/preview?type=monthly&period=2026-13&plant_selection=all&building_ids="+h.fx.BuildingA1.String(), nil)
	require.Equal(t, http.StatusUnprocessableEntity, bad.status, string(bad.body))
}

func TestReportGenerateAnswersPerBuilding(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/reports/generate", map[string]any{"type": "monthly", "period": "2026-08", "plant_selection": "all",
		"building_ids": []string{h.fx.BuildingA1.String(), h.fx.BuildingA2.String()}})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var out dto.ReportGenerateAccepted
	res.json(t, &out)
	require.Len(t, out.Items, 2)
	for _, item := range out.Items {
		require.NotEqual(t, uuid.Nil, item.ReportID)
		require.Contains(t, item.JobID, job.TypeReportGenerate+":"+item.BuildingID.String())
	}
	var generated int
	for _, task := range h.enq.snapshot() {
		if task.Type() == job.TypeReportGenerate {
			generated++
		}
	}
	require.Equal(t, 2, generated, "one task per building (R263)")
}

func TestReportPDFEnglishRendersOnDemand(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.completedReportFor(h.fx.CompanyA, h.fx.BuildingA1, "2026-08")
	ca := h.as(seed.E2ECompanyAdminEmail)
	tr := ca.do(http.MethodGet, "/reports/"+id+"/pdf", nil)
	require.Equal(t, http.StatusOK, tr.status, string(tr.body))
	require.Equal(t, "application/pdf", tr.header.Get("Content-Type"))
	require.Contains(t, tr.header.Get("Content-Disposition"), "rapor-2026-08.pdf")
	require.True(t, bytes.HasPrefix(tr.body, []byte("%PDF-")))
	en := ca.do(http.MethodGet, "/reports/"+id+"/pdf", nil, "Accept-Language", "en")
	require.Equal(t, http.StatusOK, en.status)
	require.NotEqual(t, tr.body, en.body, "English is rendered from the payload, not the stored Turkish file")
	xlsx := ca.do(http.MethodGet, "/reports/"+id+"/excel", nil)
	require.Equal(t, http.StatusOK, xlsx.status)
	require.True(t, bytes.HasPrefix(xlsx.body, []byte("PK")), "a zip container")
}

func TestReportPendingPDFIs409(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.reportFor(h.fx.CompanyA, h.fx.BuildingA1) // pending, payload {}
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/reports/"+id+"/pdf", nil)
	require.Equal(t, http.StatusConflict, res.status, string(res.body))
	require.Equal(t, "report_not_ready", res.code(t))
}

func TestReportListTotal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, period := range []string{"2026-06", "2026-07", "2026-08"} {
		h.completedReportFor(h.fx.CompanyA, h.fx.BuildingA1, period)
	}
	ca := h.as(seed.E2ECompanyAdminEmail)
	var page dto.ReportPage
	ca.do(http.MethodGet, "/reports?limit=2&year=2026&type=monthly", nil).json(t, &page)
	require.Len(t, page.Items, 2)
	require.EqualValues(t, 3, page.Total, "the archive's count is the whole match, not the page")
	require.NotNil(t, page.NextCursor)
	require.Equal(t, h.fx.BuildingA1, page.Items[0].BuildingID)
	require.NotEmpty(t, page.Items[0].BuildingName)
	var none dto.ReportPage
	ca.do(http.MethodGet, "/reports?year=2025", nil).json(t, &none)
	require.Zero(t, none.Total)
}

func TestReportEmailEnqueuesDelivery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.completedReportFor(h.fx.CompanyA, h.fx.BuildingA1, "2026-08")
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/reports/"+id+"/email", map[string]any{"to": []string{"not an address"}})
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	res = ca.do(http.MethodPost, "/reports/"+id+"/email", map[string]any{"to": []string{"yonetici@firma.test"}})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var out dto.ReportEmailAccepted
	res.json(t, &out)
	require.Contains(t, out.JobID, job.TypeReportDeliver+":"+id)
}
