package report_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var (
	companyA  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	buildingA = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	buildingB = uuid.MustParse("66666666-6666-6666-6666-666666666666")
	plantA    = uuid.MustParse("77777777-7777-7777-7777-777777777777")
)

type fakeBuildings struct {
	store.BuildingRepository
	visible map[uuid.UUID]string
}

func (f fakeBuildings) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Building, error) {
	if name, ok := f.visible[id]; ok {
		return model.Building{ID: id, Name: name}, nil
	}
	return model.Building{}, store.ErrNotFound
}

type fakeAnalyzers struct{ store.AnalyzerRepository }

func (fakeAnalyzers) List(context.Context, store.Scope, store.AnalyzerFilter) ([]model.Analyzer, error) {
	return nil, nil
}

type fakeBills struct{ store.BillRepository }

func (fakeBills) List(context.Context, store.Scope, store.BillFilter) ([]model.Bill, error) {
	return nil, nil
}

type fakePlants struct {
	store.PlantRepository
	visible map[uuid.UUID]bool
}

func (f fakePlants) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.PowerPlant, error) {
	if f.visible[id] {
		return model.PowerPlant{ID: id, Name: "GES", PlantKind: "grid"}, nil
	}
	return model.PowerPlant{}, store.ErrNotFound
}

func (fakePlants) List(context.Context, store.Scope, store.PlantFilter) ([]model.PowerPlant, error) {
	return nil, nil
}

func (fakePlants) MonthlyTargets(context.Context, store.Scope, uuid.UUID) ([]model.PlantMonthlyTarget, error) {
	return nil, nil
}

type fakeSolar struct{ store.SolarTariffRepository }

func (fakeSolar) Effective(context.Context, store.Scope, uuid.UUID, time.Time) (model.SolarTariff, error) {
	return model.SolarTariff{}, store.ErrNotFound
}

type fakeTariffs struct{ store.TariffRepository }

func (fakeTariffs) Effective(context.Context, store.Scope, uuid.UUID, time.Time) (model.Tariff, error) {
	return model.Tariff{}, store.ErrNotFound
}

type fakeCarbon struct{ store.CarbonRepository }

func (fakeCarbon) ListFactors(context.Context, store.Scope, store.EmissionFactorFilter) ([]model.EmissionFactor, error) {
	return nil, nil
}

type fakeAnalytics struct{ store.AnalyticsRepository }

func (fakeAnalytics) ProductionMonthly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.PlantProductionBucket, error) {
	return nil, nil
}

func newService(t *testing.T) *report.Service {
	t.Helper()
	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: fakeAnalytics{}, Log: testfixtures.DiscardLogger()})
	require.NoError(t, err)
	s, err := report.New(report.Deps{
		Buildings: fakeBuildings{visible: map[uuid.UUID]string{buildingA: "Merkez"}}, Analyzers: fakeAnalyzers{},
		Bills: fakeBills{}, Plants: fakePlants{visible: map[uuid.UUID]bool{plantA: true}}, Solar: fakeSolar{},
		Tariffs: fakeTariffs{}, Carbon: fakeCarbon{}, Analytics: fakeAnalytics{}, Consumption: analytics,
		Clock: clock.NewFake(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	return s
}

func monthly(ids ...uuid.UUID) report.Request {
	return report.Request{Type: domain.TypeMonthly, Period: "2026-08", Selection: domain.SelectionAll, BuildingIDs: ids}
}

// fieldOf returns the one field a 422 names.
func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var e *perr.Error
	require.ErrorAs(t, err, &e)
	require.Len(t, e.Params, 1)
	for k := range e.Params {
		return k
	}
	return ""
}

func TestValidateRules(t *testing.T) {
	t.Parallel()
	s := newService(t)
	many := make([]uuid.UUID, report.MaxBuildings+1)
	for i := range many {
		many[i] = uuid.New()
	}
	plants := make([]uuid.UUID, report.MaxPlants+1)
	for i := range plants {
		plants[i] = uuid.New()
	}
	for _, tc := range []struct {
		name  string
		edit  func(*report.Request)
		field string
	}{
		{"type", func(r *report.Request) { r.Type = "weekly" }, "type"},
		{"monthly period", func(r *report.Request) { r.Period = "2026-13" }, "period"},
		{"yearly period", func(r *report.Request) { r.Type, r.Period = domain.TypeYearly, "26" }, "period"},
		{"year too old", func(r *report.Request) { r.Period = "1999-12" }, "period"},
		{"year too new", func(r *report.Request) { r.Period = "2028-01" }, "period"},
		{"selection", func(r *report.Request) { r.Selection = "wind" }, "plant_selection"},
		{"no building", func(r *report.Request) { r.BuildingIDs = nil }, "building_ids"},
		{"too many buildings", func(r *report.Request) { r.BuildingIDs = many }, "building_ids"},
		{"duplicate building", func(r *report.Request) { r.BuildingIDs = []uuid.UUID{buildingA, buildingA} }, "building_ids"},
		{"too many plants", func(r *report.Request) { r.PlantIDs = plants }, "plant_ids"},
		{"duplicate plant", func(r *report.Request) { r.PlantIDs = []uuid.UUID{plantA, plantA} }, "plant_ids"},
	} {
		r := monthly(buildingA)
		tc.edit(&r)
		_, err := s.Validate(r)
		require.Equal(t, tc.field, fieldOf(t, err), tc.name)
	}
	_, err := s.Validate(monthly(buildingA))
	require.NoError(t, err)
	_, err = s.Validate(report.Request{Type: domain.TypeYearly, Period: "2027", Selection: domain.SelectionGrid, BuildingIDs: []uuid.UUID{buildingA}})
	require.NoError(t, err, "next year is the upper bound")
}

func TestPreviewRefusesInvisibleBuilding(t *testing.T) {
	t.Parallel()
	_, err := newService(t).Preview(t.Context(), store.SystemScope(companyA), monthly(buildingA, buildingB))
	require.ErrorIs(t, err, store.ErrNotFound, "one invisible building fails the whole preview as a 404")
}

func TestPreviewRefusesForeignPlant(t *testing.T) {
	t.Parallel()
	r := monthly(buildingA)
	r.PlantIDs = []uuid.UUID{uuid.New()}
	_, err := newService(t).Preview(t.Context(), store.SystemScope(companyA), r)
	require.ErrorIs(t, err, store.ErrNotFound)

	r.PlantIDs = []uuid.UUID{plantA}
	p, err := newService(t).Preview(t.Context(), store.SystemScope(companyA), r)
	require.NoError(t, err)
	require.Len(t, p.Monthly.Plants, 1)
	require.Equal(t, "2026-08", p.Period)
	require.Equal(t, domain.PayloadVersion, p.Version)
}

func samplePayload() domain.Payload {
	v := decimal.RequireFromString("1234.5")
	return domain.Payload{Type: domain.TypeMonthly, Period: "2026-03", Monthly: &domain.Monthly{Year: 2026, Month: 3,
		Consumption: domain.Figure{Value: &v, WithData: 1, Of: 1},
		Bill:        []domain.MoneyFigure{{Currency: "TRY", Value: decimal.RequireFromString("5000"), WithData: 1, Of: 1}}}}
}

func TestEmailBodyEscapesBuildingName(t *testing.T) {
	t.Parallel()
	subject, html, err := report.EmailContent(samplePayload(), []string{"<script>alert(1)</script>\r\nBcc: x@evil.test"}, "tr")
	require.NoError(t, err)
	require.NotContains(t, html, "<script>")
	require.Contains(t, html, "&lt;script&gt;")
	require.NotContains(t, subject, "\n", "a line break would end the Subject header")
	require.NotContains(t, subject, "\r")
}

func TestEmailContentBothLocales(t *testing.T) {
	t.Parallel()
	subject, html, err := report.EmailContent(samplePayload(), []string{"Merkez", "Depo"}, "tr")
	require.NoError(t, err)
	require.Equal(t, "Merkez, Depo - Aylık Enerji Raporu - Mart 2026", subject)
	require.Contains(t, html, "1.234,50 kWh")
	require.Contains(t, html, "5.000,00 TRY")
	require.Contains(t, html, "veri yok", "a missing production says so")

	subject, _, err = report.EmailContent(samplePayload(), []string{"Merkez"}, "en")
	require.NoError(t, err)
	require.Equal(t, "Merkez - Monthly Energy Report - March 2026", subject)
	text := report.EmailText(samplePayload(), []string{"Merkez"}, "en")
	require.True(t, strings.Contains(text, "Total consumption: 1,234.50 kWh"), text)
}
