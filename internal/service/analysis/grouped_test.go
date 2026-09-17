package analysis_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/grouping"
	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/analysis"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var istanbul = mustIstanbul()

func mustIstanbul() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}

// fakeSeries returns one daily row per requested day, 10 kWh each.
type fakeSeries struct{ asked []store.TimeRange }

func (f *fakeSeries) Consumption(_ context.Context, _ store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error) {
	f.asked = append(f.asked, req.Range)
	var out []consumption.Row
	for day := req.Range.From; day.Before(req.Range.To); day = day.AddDate(0, 0, 1) {
		v := decimal.NewFromInt(10)
		ind, cap := decimal.NewFromInt(2), decimal.NewFromInt(1)
		out = append(out, consumption.Row{
			AnalyzerID: req.AnalyzerIDs[0],
			Window:     energy.Window{From: day, To: day.AddDate(0, 0, 1)},
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport: &v, energy.ReactiveInductiveImport: &ind, energy.ReactiveCapacitiveImport: &cap,
			},
		})
	}
	return out, nil
}

type fakeProfiles struct {
	weekend   map[time.Weekday]bool
	vacations []domainlp.DateRange
	asked     []store.TimeRange
}

func (f *fakeProfiles) Profiles(context.Context, store.Scope, loadprofile.Request) (loadprofile.Result, error) {
	return loadprofile.Result{}, nil
}

func (f *fakeProfiles) CalendarConfig(_ context.Context, _ store.Scope, r store.TimeRange) (domainlp.Config, error) {
	f.asked = append(f.asked, r)
	return domainlp.Config{WeekendDays: f.weekend, Vacations: f.vacations, Location: istanbul}, nil
}

type fakeAnalyzers struct{ store.AnalyzerRepository }

func (fakeAnalyzers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Analyzer, error) {
	return model.Analyzer{ID: id}, nil
}

type fakeBuildings struct{ store.BuildingRepository }

type fakeParams struct {
	store.BillingParameterRepository
}

type fakeTariffs struct{ store.TariffRepository }

type fakeAnomalies struct{}

func (fakeAnomalies) ListAnomalies(context.Context, store.Scope, consumption.AnomalyListRequest) ([]model.ConsumptionAnomaly, error) {
	return nil, nil
}

func (fakeAnomalies) ResolveAnomaly(context.Context, store.Scope, uuid.UUID, uuid.UUID, consumption.Resolution) (model.ConsumptionAnomaly, error) {
	return model.ConsumptionAnomaly{}, nil
}

func newGroupedService(t *testing.T, profiles *fakeProfiles, series *fakeSeries) *analysis.Service {
	t.Helper()
	s, err := analysis.New(analysis.Deps{
		Series: series, Anomalies: fakeAnomalies{}, Profiles: profiles, Analyzers: fakeAnalyzers{},
		Buildings: fakeBuildings{}, Params: fakeParams{}, Tariffs: fakeTariffs{}, Clock: clock.NewFake(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)), Location: istanbul,
	})
	require.NoError(t, err)
	return s
}

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, istanbul)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGroupedPreviousPeriod(t *testing.T) {
	analyzer := uuid.New()
	profiles := &fakeProfiles{weekend: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}}
	series := &fakeSeries{}
	svc := newGroupedService(t, profiles, series)
	out, err := svc.Grouped(t.Context(), store.SystemScope(uuid.New()), analysis.GroupedInput{
		Subject: analysis.Subject{AnalyzerID: &analyzer}, From: day("2026-03-10"), To: day("2026-03-16"),
		By: grouping.DayType, ComparePrevious: true,
	})
	require.NoError(t, err)
	require.Equal(t, grouping.DayType, out.By)
	require.Equal(t, []string{"weekday", "weekend"}, []string{out.Current.Buckets[0].Key, out.Current.Buckets[1].Key})
	require.Equal(t, "50", out.Current.Buckets[0].Active.String(), "five weekdays × 10 kWh")
	require.Equal(t, "20", out.Current.Buckets[1].Active.String())
	require.Equal(t, "70", out.Current.Statistics.Total.String())

	require.NotNil(t, out.Previous)
	require.Equal(t, day("2026-03-03"), out.Previous.From, "the seven days before the range")
	require.Equal(t, day("2026-03-09"), out.Previous.To)
	require.Equal(t, "70", out.Previous.Statistics.Total.String())

	require.Len(t, profiles.asked, 1, "the calendar is read once for both periods")
	require.Equal(t, day("2026-03-03"), profiles.asked[0].From)
}

func TestGroupedRejectsBadInput(t *testing.T) {
	analyzer := uuid.New()
	svc := newGroupedService(t, &fakeProfiles{}, &fakeSeries{})
	subject := analysis.Subject{AnalyzerID: &analyzer}
	scope := store.SystemScope(uuid.New())

	_, err := svc.Grouped(t.Context(), scope, analysis.GroupedInput{
		Subject: subject, From: day("2026-03-10"), To: day("2026-03-16"), By: "monthly"})
	require.ErrorIs(t, err, analysis.ErrInvalidParameters)

	_, err = svc.Grouped(t.Context(), scope, analysis.GroupedInput{
		Subject: subject, From: day("2025-01-01"), To: day("2026-03-16"), By: grouping.Week})
	require.ErrorIs(t, err, analysis.ErrInvalidParameters, "a range wider than a year is refused")

	_, err = svc.Grouped(t.Context(), scope, analysis.GroupedInput{
		Subject: subject, From: day("2026-03-16"), To: day("2026-03-10"), By: grouping.Week})
	require.ErrorIs(t, err, analysis.ErrInvalidParameters)
}

func TestGroupedUsesTheCompanyCalendar(t *testing.T) {
	analyzer := uuid.New()
	// Thursday 12 March is a vacation day and Friday is a weekend day: both count as weekend (R137).
	profiles := &fakeProfiles{
		weekend:   map[time.Weekday]bool{time.Friday: true, time.Saturday: true},
		vacations: []domainlp.DateRange{{Start: day("2026-03-12"), End: day("2026-03-12")}},
	}
	svc := newGroupedService(t, profiles, &fakeSeries{})
	out, err := svc.Grouped(t.Context(), store.SystemScope(uuid.New()), analysis.GroupedInput{
		Subject: analysis.Subject{AnalyzerID: &analyzer}, From: day("2026-03-09"), To: day("2026-03-15"),
		By: grouping.DayType,
	})
	require.NoError(t, err)
	require.Equal(t, "40", out.Current.Buckets[0].Active.String(), "Mon, Tue, Wed and Sun are working days here")
	require.Equal(t, "30", out.Current.Buckets[1].Active.String(), "Thu (vacation), Fri and Sat")
	require.Nil(t, out.Previous)
}
