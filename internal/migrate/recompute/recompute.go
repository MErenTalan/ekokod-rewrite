// Package recompute regenerates derived data after a migration load (08 §7):
// every command converges on re-runs, reports progress and collects failures
// instead of aborting.
package recompute

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Failure is one unit that did not recompute.
type Failure struct {
	Company string `json:"company,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Subject string `json:"subject,omitempty"`
	Period  string `json:"period,omitempty"`
	Error   string `json:"error"`
}

// Summary is R435's result.
type Summary struct {
	Processed int       `json:"processed"`
	Failed    int       `json:"failed"`
	Failures  []Failure `json:"failures"`
}

type tally struct {
	mu  sync.Mutex
	sum Summary
	out io.Writer
}

func (t *tally) ok() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sum.Processed++
}

func (t *tally) fail(f Failure) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sum.Failed++
	t.sum.Failures = append(t.sum.Failures, f)
}

func (t *tally) progress(format string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(t.out, format+"\n", args...)
}

func (t *tally) result() Summary {
	t.mu.Lock()
	defer t.mu.Unlock()
	sort.Slice(t.sum.Failures, func(i, j int) bool {
		a, b := t.sum.Failures[i], t.sum.Failures[j]
		return a.Company+a.Scope+a.Subject+a.Period < b.Company+b.Scope+b.Subject+b.Period
	})
	if t.sum.Failures == nil {
		t.sum.Failures = []Failure{}
	}
	return t.sum
}

// Refresher refreshes the continuous aggregates (admin.AggregateRepository).
type Refresher interface {
	Refresh(ctx context.Context, view store.AggregateView, rng store.TimeRange) error
	RefreshPlantProduction(ctx context.Context, rng store.TimeRange) error
}

// Consumption is R430: fine views in one-month windows, monthly views in
// one-year windows, yearly in one window — each padded by a bucket on both
// sides, since TimescaleDB refuses a window smaller than one bucket and a
// refresh converges anyway.
func Consumption(ctx context.Context, r Refresher, from, to time.Time, out io.Writer) Summary {
	t := &tally{out: out}
	windows := func(step func(time.Time) time.Time, first time.Time) []store.TimeRange {
		var ws []store.TimeRange
		for s := first; s.Before(to); s = step(s) {
			ws = append(ws, store.TimeRange{From: s, To: step(s)})
		}
		return ws
	}
	month := func(d time.Time) time.Time { return d.AddDate(0, 1, 0) }
	year := func(d time.Time) time.Time { return d.AddDate(1, 0, 0) }
	pad := func(w store.TimeRange, before, after func(time.Time) time.Time) store.TimeRange {
		return store.TimeRange{From: before(w.From), To: after(w.To)}
	}
	dayBack, dayOn := func(d time.Time) time.Time { return d.AddDate(0, 0, -1) }, func(d time.Time) time.Time { return d.AddDate(0, 0, 1) }
	monthBack := func(d time.Time) time.Time { return d.AddDate(0, -1, 0) }
	yearBack := func(d time.Time) time.Time { return d.AddDate(-1, 0, 0) }
	refresh := func(label string, w store.TimeRange, fn func(store.TimeRange) error) {
		if err := fn(w); err != nil {
			t.fail(Failure{Scope: label, Period: w.From.Format("2006-01-02"), Error: err.Error()})
			return
		}
		t.ok()
	}
	view := func(v store.AggregateView) func(store.TimeRange) error {
		return func(w store.TimeRange) error { return r.Refresh(ctx, v, w) }
	}
	for _, w := range windows(month, monthStart(from)) {
		refresh(string(store.ViewConsumptionHourly), pad(w, dayBack, dayOn), view(store.ViewConsumptionHourly))
		refresh(string(store.ViewConsumptionDaily), pad(w, dayBack, dayOn), view(store.ViewConsumptionDaily))
		refresh("plant_production", pad(w, dayBack, dayOn), func(w store.TimeRange) error { return r.RefreshPlantProduction(ctx, w) })
		t.progress("consumption %s refreshed (hourly, daily, plants)", w.From.Format("2006-01"))
	}
	for _, w := range windows(year, yearStart(from)) {
		refresh(string(store.ViewConsumptionMonthly), pad(w, monthBack, month), view(store.ViewConsumptionMonthly))
		t.progress("consumption %s refreshed (monthly)", w.From.Format("2006"))
	}
	all := store.TimeRange{From: yearBack(yearStart(from)), To: year(year(yearStart(to)))}
	refresh(string(store.ViewConsumptionYearly), all, view(store.ViewConsumptionYearly))
	return t.result()
}

func yearStart(d time.Time) time.Time {
	l := d.In(istanbul)
	return time.Date(l.Year(), 1, 1, 0, 0, 0, 0, istanbul)
}

func monthStart(d time.Time) time.Time {
	l := d.In(istanbul)
	return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, istanbul)
}

// Billable lists the buildings that can be billed (admin.BillingRepository).
type Billable interface {
	BillableBuildings(ctx context.Context) ([]store.BillableBuilding, error)
}

// Analyzers lists a building's analyzers inside a company scope.
type Analyzers interface {
	List(ctx context.Context, s store.Scope, f store.AnalyzerFilter) ([]model.Analyzer, error)
}

// BillingGenerator is the worker's billing.generate handler.
type BillingGenerator interface {
	Generate(ctx context.Context, p job.BillingGeneratePayload) error
}

// BillsOptions are R431's flags.
type BillsOptions struct {
	From, To string // YYYY-MM; To empty = each building's latest closed period
	Force    bool
	Parallel int
	Now      time.Time
}

// PeriodKeys lists YYYY-MM keys from `from` to `to` inclusive.
func PeriodKeys(from, to string) ([]string, error) {
	f, err := time.Parse("2006-01", from)
	if err != nil {
		return nil, fmt.Errorf("period %q: %w", from, err)
	}
	l, err := time.Parse("2006-01", to)
	if err != nil {
		return nil, fmt.Errorf("period %q: %w", to, err)
	}
	var keys []string
	for m := f; !m.After(l); m = m.AddDate(0, 1, 0) {
		keys = append(keys, m.Format("2006-01"))
	}
	return keys, nil
}

// Bills is R431.
func Bills(ctx context.Context, b Billable, a Analyzers, g BillingGenerator, o BillsOptions, out io.Writer) (Summary, error) {
	buildings, err := b.BillableBuildings(ctx)
	if err != nil {
		return Summary{}, err
	}
	byCompany := map[uuid.UUID][]store.BillableBuilding{}
	var companies []uuid.UUID
	for _, bb := range buildings {
		if _, seen := byCompany[bb.CompanyID]; !seen {
			companies = append(companies, bb.CompanyID)
		}
		byCompany[bb.CompanyID] = append(byCompany[bb.CompanyID], bb)
	}
	t := &tally{out: out}
	// One bound across every building of every company: a single large company
	// is the common case, so buildings, not companies, are the unit of work.
	sem := make(chan struct{}, max(o.Parallel, 1))
	var wg sync.WaitGroup
	for _, company := range companies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			billCompany(ctx, a, g, o, company, byCompany[company], t, sem)
		}()
	}
	wg.Wait()
	return t.result(), nil
}

func billCompany(ctx context.Context, a Analyzers, g BillingGenerator, o BillsOptions, company uuid.UUID, buildings []store.BillableBuilding, t *tally, sem chan struct{}) {
	var mu sync.Mutex
	companyKeys := map[string]bool{}
	gen := func(scope model.BillScope, subject uuid.UUID, key string) {
		p := job.BillingGeneratePayload{CompanyID: company, Scope: scope, SubjectID: subject, PeriodKey: key, Force: o.Force}
		if err := g.Generate(ctx, p); err != nil {
			t.fail(Failure{Company: company.String(), Scope: string(scope), Subject: subject.String(), Period: key, Error: err.Error()})
			return
		}
		t.ok()
	}
	var wg sync.WaitGroup
	for _, bb := range buildings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			to := o.To
			if to == "" {
				to = domain.LatestClosedPeriodKey(bb.CutoffDay, o.Now, consumption.SettleDelayMonthly, istanbul)
			}
			keys, err := PeriodKeys(o.From, to)
			if err != nil {
				t.fail(Failure{Company: company.String(), Subject: bb.BuildingID.String(), Error: err.Error()})
				return
			}
			bid := bb.BuildingID
			analyzers, err := allAnalyzers(ctx, a, company, bid)
			if err != nil {
				t.fail(Failure{Company: company.String(), Subject: bid.String(), Error: err.Error()})
				return
			}
			for _, key := range keys {
				gen(model.BillScopeBuilding, bid, key)
				for _, an := range analyzers {
					gen(model.BillScopeAnalyzer, an.ID, key)
				}
				mu.Lock()
				companyKeys[key] = true
				mu.Unlock()
			}
			t.progress("bills %s building %s: %d periods", company, bid, len(keys))
		}()
	}
	wg.Wait()
	keys := make([]string, 0, len(companyKeys))
	for k := range companyKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sem <- struct{}{}
	defer func() { <-sem }()
	for _, key := range keys {
		gen(model.BillScopeCompany, company, key)
	}
}

const analyzerPage = 1000

func allAnalyzers(ctx context.Context, a Analyzers, company, building uuid.UUID) ([]model.Analyzer, error) {
	var out []model.Analyzer
	for offset := int32(0); ; offset += analyzerPage {
		page, err := a.List(ctx, store.SystemScope(company), store.AnalyzerFilter{BuildingID: &building, Page: store.Page{Limit: analyzerPage, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < analyzerPage {
			return out, nil
		}
	}
}

// ReportGenerator is the worker's report.generate handler.
type ReportGenerator interface {
	Generate(ctx context.Context, p job.ReportGeneratePayload) error
}

// Reports is R432: monthly per building and period, yearly per closed year.
func Reports(ctx context.Context, b Billable, g ReportGenerator, from string, now time.Time, out io.Writer) (Summary, error) {
	buildings, err := b.BillableBuildings(ctx)
	if err != nil {
		return Summary{}, err
	}
	local := now.In(istanbul)
	lastMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0).Format("2006-01")
	months, err := PeriodKeys(from, lastMonth)
	if err != nil {
		return Summary{}, err
	}
	t := &tally{out: out}
	first, _ := strconv.Atoi(from[:4])
	for _, bb := range buildings {
		run := func(kind, period string) {
			p := job.ReportGeneratePayload{CompanyID: bb.CompanyID, BuildingID: bb.BuildingID, Type: kind, Period: period, PlantSelection: "all"}
			if err := g.Generate(ctx, p); err != nil {
				t.fail(Failure{Company: bb.CompanyID.String(), Scope: kind, Subject: bb.BuildingID.String(), Period: period, Error: err.Error()})
				return
			}
			t.ok()
		}
		for _, m := range months {
			run("monthly", m)
		}
		for y := first; y < local.Year(); y++ {
			run("yearly", strconv.Itoa(y))
		}
		t.progress("reports %s: %d months", bb.BuildingID, len(months))
	}
	return t.result(), nil
}

// CarbonRecomputer is carbon.Service.Recompute (Q-K3).
type CarbonRecomputer interface {
	Recompute(ctx context.Context, day time.Time) error
}

// Carbon is R433: every day from `from` to yesterday (Istanbul).
func Carbon(ctx context.Context, c CarbonRecomputer, from, now time.Time, out io.Writer) Summary {
	t := &tally{out: out}
	local := now.In(istanbul)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	f := from.In(istanbul)
	for d := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, time.UTC); d.Before(today); d = d.AddDate(0, 0, 1) {
		if err := c.Recompute(ctx, d); err != nil {
			t.fail(Failure{Period: d.Format(time.DateOnly), Error: err.Error()})
			continue
		}
		t.ok()
		if d.Day() == 1 {
			t.progress("carbon %s", d.Format("2006-01"))
		}
	}
	return t.result()
}

// ForecastRunner is the worker's forecast.run handler.
type ForecastRunner interface {
	RunTask(ctx context.Context) error
}

// Forecasts is R434.
func Forecasts(ctx context.Context, f ForecastRunner, out io.Writer) Summary {
	t := &tally{out: out}
	if err := f.RunTask(ctx); err != nil {
		t.fail(Failure{Scope: "forecast", Error: err.Error()})
	} else {
		t.ok()
	}
	t.progress("forecasts done")
	return t.result()
}
