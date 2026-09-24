package recompute_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/recompute"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type billable []store.BillableBuilding

func (b billable) BillableBuildings(context.Context) ([]store.BillableBuilding, error) { return b, nil }

type analyzers map[uuid.UUID][]model.Analyzer

// List pages like the repository does.
func (a analyzers) List(_ context.Context, _ store.Scope, f store.AnalyzerFilter) ([]model.Analyzer, error) {
	all := a[*f.BuildingID]
	from := min(int(f.Page.Offset), len(all))
	to := min(from+int(f.Page.Limit), len(all))
	return all[from:to], nil
}

type generator struct {
	mu       sync.Mutex
	calls    []job.BillingGeneratePayload
	failOn   uuid.UUID
	inFlight atomic.Int32
	peak     atomic.Int32
}

func (g *generator) Generate(_ context.Context, p job.BillingGeneratePayload) error {
	n := g.inFlight.Add(1)
	defer g.inFlight.Add(-1)
	for {
		if old := g.peak.Load(); n <= old || g.peak.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	g.mu.Lock()
	g.calls = append(g.calls, p)
	g.mu.Unlock()
	if p.SubjectID == g.failOn {
		return errors.New("tariff_not_found")
	}
	return nil
}

func TestPeriodKeys(t *testing.T) {
	keys, err := recompute.PeriodKeys("2025-11", "2026-02")
	require.NoError(t, err)
	require.Equal(t, []string{"2025-11", "2025-12", "2026-01", "2026-02"}, keys)
	_, err = recompute.PeriodKeys("2025-13", "2026-02")
	require.Error(t, err)
}

func TestBillsCoverEveryScopeAndCollectFailures(t *testing.T) {
	c1, c2 := uuid.New(), uuid.New()
	b1, b2, b3 := uuid.New(), uuid.New(), uuid.New()
	a1 := uuid.New()
	g := &generator{failOn: b2}
	var out bytes.Buffer
	sum, err := recompute.Bills(context.Background(),
		billable{{CompanyID: c1, BuildingID: b1, CutoffDay: 1}, {CompanyID: c1, BuildingID: b2, CutoffDay: 1}, {CompanyID: c2, BuildingID: b3, CutoffDay: 1}},
		analyzers{b1: {{ID: a1}}}, g, recompute.BillsOptions{From: "2026-07", To: "2026-08", Parallel: 2}, &out)
	require.NoError(t, err)
	// c1: b1 (2) + a1 (2) + b2 (2) + company (2); c2: b3 (2) + company (2).
	require.Len(t, g.calls, 12)
	require.Equal(t, 2, sum.Failed, "b2's two periods fail; the run goes on")
	require.Equal(t, 10, sum.Processed)
	require.Equal(t, b2.String(), sum.Failures[0].Subject)
	for _, c := range g.calls {
		require.False(t, c.Force, "no --force: a live bill is kept, so a rerun converges (Q-K1)")
	}
	scopes := map[model.BillScope]int{}
	for _, c := range g.calls {
		scopes[c.Scope]++
	}
	require.Equal(t, map[model.BillScope]int{model.BillScopeBuilding: 6, model.BillScopeAnalyzer: 2, model.BillScopeCompany: 4}, scopes)
	require.LessOrEqual(t, g.peak.Load(), int32(2), "companies run in parallel, bounded")
	require.Contains(t, out.String(), "bills ")
}

func TestBillsDefaultToTheLatestClosedPeriod(t *testing.T) {
	b := uuid.New()
	g := &generator{}
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	_, err := recompute.Bills(context.Background(), billable{{CompanyID: uuid.New(), BuildingID: b, CutoffDay: 1}}, analyzers{}, g,
		recompute.BillsOptions{From: "2026-07", Force: true, Now: now}, &bytes.Buffer{})
	require.NoError(t, err)
	var keys []string
	for _, c := range g.calls {
		if c.Scope == model.BillScopeBuilding {
			keys = append(keys, c.PeriodKey)
		}
		require.True(t, c.Force)
	}
	require.Equal(t, []string{"2026-07", "2026-08"}, keys, "August is closed and settled on 24 September")
}

type carbonDays struct{ days []string }

func (c *carbonDays) Recompute(_ context.Context, day time.Time) error {
	c.days = append(c.days, day.Format(time.DateOnly))
	if day.Day() == 21 {
		return errors.New("no grid factor")
	}
	return nil
}

func TestCarbonRunsEveryDayUpToYesterday(t *testing.T) {
	c := &carbonDays{}
	now := time.Date(2026, 9, 23, 22, 30, 0, 0, time.UTC) // already the 24th in Istanbul
	sum := recompute.Carbon(context.Background(), c, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), now, &bytes.Buffer{})
	require.Equal(t, []string{"2026-09-20", "2026-09-21", "2026-09-22", "2026-09-23"}, c.days)
	require.Equal(t, 1, sum.Failed)
	require.Equal(t, 3, sum.Processed)
}

func TestBillsParallelIsBounded(t *testing.T) {
	var bs billable
	for range 5 {
		bs = append(bs, store.BillableBuilding{CompanyID: uuid.New(), BuildingID: uuid.New(), CutoffDay: 1})
	}
	g := &generator{}
	_, err := recompute.Bills(context.Background(), bs, analyzers{}, g, recompute.BillsOptions{From: "2026-01", To: "2026-03", Parallel: 2}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, int32(2), g.peak.Load(), "never more than --parallel bills at once")
}

type refresher struct{ calls []string }

func (r *refresher) Refresh(_ context.Context, v store.AggregateView, w store.TimeRange) error {
	r.calls = append(r.calls, string(v)+" "+w.From.Format(time.DateOnly)+"→"+w.To.Format(time.DateOnly))
	return nil
}

func (r *refresher) RefreshPlantProduction(_ context.Context, w store.TimeRange) error {
	r.calls = append(r.calls, "plants "+w.From.Format(time.DateOnly)+"→"+w.To.Format(time.DateOnly))
	return nil
}

// R430: every window spans more than one bucket of its view (TimescaleDB refuses less).
func TestConsumptionWindowsFitTheirBuckets(t *testing.T) {
	r := &refresher{}
	ist, _ := time.LoadLocation("Europe/Istanbul")
	sum := recompute.Consumption(context.Background(), r, time.Date(2025, 12, 10, 0, 0, 0, 0, ist), time.Date(2026, 2, 5, 0, 0, 0, 0, ist), &bytes.Buffer{})
	require.Zero(t, sum.Failed)
	require.Equal(t, []string{
		"consumption_hourly 2025-11-30→2026-01-02", "consumption_daily 2025-11-30→2026-01-02", "plants 2025-11-30→2026-01-02",
		"consumption_hourly 2025-12-31→2026-02-02", "consumption_daily 2025-12-31→2026-02-02", "plants 2025-12-31→2026-02-02",
		"consumption_hourly 2026-01-31→2026-03-02", "consumption_daily 2026-01-31→2026-03-02", "plants 2026-01-31→2026-03-02",
		"consumption_monthly 2024-12-01→2026-02-01", "consumption_monthly 2025-12-01→2027-02-01",
		"consumption_yearly 2024-01-01→2028-01-01",
	}, r.calls)
}

func TestBillsPageThroughEveryAnalyzer(t *testing.T) {
	b := uuid.New()
	many := make([]model.Analyzer, 2501)
	for i := range many {
		many[i] = model.Analyzer{ID: uuid.New()}
	}
	g := &generator{}
	_, err := recompute.Bills(context.Background(), billable{{CompanyID: uuid.New(), BuildingID: b, CutoffDay: 1}}, analyzers{b: many}, g,
		recompute.BillsOptions{From: "2026-07", To: "2026-07"}, &bytes.Buffer{})
	require.NoError(t, err)
	n := 0
	for _, c := range g.calls {
		if c.Scope == model.BillScopeAnalyzer {
			n++
		}
	}
	require.Equal(t, 2501, n, "no analyzer is dropped by a page limit")
}

// F15b: a single large company is the common case (the scale run was one company,
// 0.5 s per bill sequentially): its buildings run in parallel too, bounded.
func TestBillsParallelisesBuildingsWithinACompany(t *testing.T) {
	company := uuid.New()
	var bs billable
	for range 6 {
		bs = append(bs, store.BillableBuilding{CompanyID: company, BuildingID: uuid.New(), CutoffDay: 1})
	}
	g := &generator{}
	sum, err := recompute.Bills(context.Background(), bs, analyzers{}, g, recompute.BillsOptions{From: "2026-01", To: "2026-02", Parallel: 3}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 6*2+2, sum.Processed, "six buildings × two months, then the company's two")
	require.Equal(t, int32(3), g.peak.Load())
	last := g.calls[len(g.calls)-2:]
	for _, c := range last {
		require.Equal(t, model.BillScopeCompany, c.Scope, "the company bill waits for every building")
	}
}
