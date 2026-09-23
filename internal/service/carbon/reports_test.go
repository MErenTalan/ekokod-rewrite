package carbon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func (w *world) reporter() *carbon.Service {
	return carbon.New(carbon.Deps{Carbon: w.carbon, Buildings: w.buildings, Clock: clock.NewFake(now),
		Companies: &fakeCompanies{names: map[uuid.UUID]string{w.company: "Acme Enerji", w.other: "Başka A.Ş."}}})
}

func (w *world) seedReportActivities(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	w.selectAll(t, w.b1)
	a, err := w.svc().CreateActivity(ctx, w.admin, user, gasInput(w.b1)) // Aug, 2066.72, pending
	require.NoError(t, err)
	_ = a
	approved, err := w.svc().CreateActivity(ctx, w.admin, user, carbon.ActivityInput{BuildingID: w.b1, SubCategory: "sub_space_heating",
		FactorKey: "natural_gas", Unit: "m3", Quantity: d("100"), PeriodStart: date(2026, 7, 1), PeriodEnd: date(2026, 7, 1)})
	require.NoError(t, err)
	_, err = w.svc().SetStatus(ctx, w.admin, approved.ID, model.CarbonStatusApproved)
	require.NoError(t, err)
	for _, sub := range []string{domain.SubGrid, domain.SubGeneration} {
		q, kg := "1000", "469"
		if sub == domain.SubGeneration {
			q, kg = "200", "0"
		}
		_, err = w.carbon.UpsertAutomatedActivity(ctx, w.admin, model.CarbonActivity{CompanyID: w.company, BuildingID: w.b1,
			SubCategory: sub, ActivityType: sub, PeriodStart: date(2026, 8, 10), PeriodEnd: date(2026, 8, 10), Quantity: d(q),
			Unit: "kWh", EmissionKgco2e: d(kg), Status: model.CarbonStatusApproved})
		require.NoError(t, err)
	}
}

func payloadOf(t *testing.T, r model.CarbonReport) domain.ReportPayload {
	t.Helper()
	var p domain.ReportPayload
	require.NoError(t, json.Unmarshal(r.Payload, &p))
	return p
}

func TestGHGReportSnapshot(t *testing.T) {
	w := newWorld()
	w.seedReportActivities(t)
	ctx := context.Background()
	r, err := w.reporter().CreateReport(ctx, w.admin, carbon.ReportInput{BuildingID: w.b1, Type: "ghg",
		From: date(2026, 7, 1), To: date(2026, 8, 31)})
	require.NoError(t, err)
	require.Equal(t, "GHG Protocol 2026-07-01 – 2026-08-31", r.Name)
	require.Equal(t, "ghg", r.ReportType)
	require.Equal(t, "2026-07-01/2026-08-31", r.Period)
	p := payloadOf(t, r)
	require.Equal(t, "Acme Enerji", p.Company)
	require.Equal(t, "Merkez", p.Building)
	require.Equal(t, []string{"scope_1", "scope_2", "scope_3"}, []string{p.Groups[0].Key, p.Groups[1].Key, p.Groups[2].Key})
	require.True(t, p.Groups[0].TotalKgCO2e.Equal(d("2273.392")), "2066.72 + 206.672 + 0 generation")
	require.True(t, p.Groups[1].TotalKgCO2e.Equal(d("469")))
	require.True(t, p.TotalKgCO2e.Equal(d("2742.392")))
	require.Equal(t, 1, p.Pending)
	require.NotNil(t, p.Figures94)
	require.True(t, p.Figures94.ConsumptionKwh.Equal(d("1000")))
	require.True(t, p.Figures94.GenerationKwh.Equal(d("200")))
	require.True(t, p.Figures94.GridFactor.Equal(d("0.469")))
	require.EqualValues(t, 2025, *p.Figures94.GridFactorYear)
	require.Equal(t, "kg CO2e/kWh", p.Figures94.GridFactorUnit)
	require.True(t, p.Figures94.NetT.Equal(d("0.3752")), "(1000 − 200) × 0.469 / 1000")

	// The snapshot does not move when the catalogue does.
	_, err = w.svc().OverrideFactor(ctx, w.admin, w.gridID, carbon.FactorOverride{BaseFactor: d("0.9")})
	require.NoError(t, err)
	again, err := w.carbon.Report(ctx, w.admin, r.ID)
	require.NoError(t, err)
	require.True(t, payloadOf(t, again).Figures94.GridFactor.Equal(d("0.469")))
}

func TestISOReportHasSixGroupsAndNamedReports(t *testing.T) {
	w := newWorld()
	w.seedReportActivities(t)
	r, err := w.reporter().CreateReport(context.Background(), w.admin, carbon.ReportInput{BuildingID: w.b1, Type: "iso",
		From: date(2026, 1, 1), To: date(2026, 6, 30), Name: ptr("  İlk yarı  ")})
	require.NoError(t, err)
	require.Equal(t, "İlk yarı", r.Name)
	p := payloadOf(t, r)
	require.Len(t, p.Groups, 6)
	require.True(t, p.TotalKgCO2e.IsZero())
	require.Nil(t, p.Figures94, "no automated record in the window")
}

func TestReportRefusals(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	for _, c := range []struct {
		in          carbon.ReportInput
		field, code string
	}{
		{carbon.ReportInput{BuildingID: w.b1, Type: "gri", From: date(2026, 1, 1), To: date(2026, 1, 2)}, "report_type", "invalid"},
		{carbon.ReportInput{BuildingID: w.b1, Type: "ghg", From: date(2026, 2, 1), To: date(2026, 1, 2)}, "to", "invalid_range"},
		{carbon.ReportInput{BuildingID: w.b1, Type: "ghg", From: date(2025, 1, 1), To: date(2026, 1, 2)}, "to", "range_too_long"},
		{carbon.ReportInput{BuildingID: w.b1, Type: "ghg", From: date(2026, 9, 1), To: date(2026, 9, 11)}, "to", "future"},
		{carbon.ReportInput{BuildingID: w.b1, Type: "ghg", From: date(2026, 1, 1), To: date(2026, 1, 2), Name: ptr(strings.Repeat("a", 121))}, "name", "too_long"},
	} {
		_, err := w.reporter().CreateReport(ctx, w.admin, c.in)
		requireValidation(t, err, c.field, c.code)
	}
	_, err := w.reporter().CreateReport(ctx, w.ba, carbon.ReportInput{BuildingID: w.b2, Type: "ghg", From: date(2026, 1, 1), To: date(2026, 1, 2)})
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestReportPDFAndList(t *testing.T) {
	w := newWorld()
	w.seedReportActivities(t)
	ctx := context.Background()
	r, err := w.reporter().CreateReport(ctx, w.admin, carbon.ReportInput{BuildingID: w.b1, Type: "ghg", From: date(2026, 7, 1), To: date(2026, 8, 31)})
	require.NoError(t, err)
	pdf, name, err := w.reporter().ReportPDF(ctx, w.ba, r.ID, "en")
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(pdf, []byte("%PDF-")))
	require.Equal(t, "carbon-ghg-2026-07-01-2026-08-31.pdf", name)
	_, _, err = w.reporter().ReportPDF(ctx, w.otherSc, r.ID, "tr")
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := w.reporter().Reports(ctx, w.ba, &w.b1, store.Page{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	_, err = w.reporter().Reports(ctx, w.ba, &w.b2, store.Page{})
	require.ErrorIs(t, err, store.ErrNotFound)
}
