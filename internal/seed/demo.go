package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// The synthetic demo company (R138, R186). Every id is fixed; the analyzers
// are the R550 baseline's, so the first demo's analyzer/1 and /2 are purged.
var (
	demoNamespace   = uuid.MustParse("7c1e2a55-0d6b-4e8e-9d3b-3f7e5a2c9b40")
	DemoCompanyID   = uuid.NewSHA1(demoNamespace, []byte("company"))
	DemoBuildingID  = uuid.NewSHA1(demoNamespace, []byte("building"))
	DemoAnalyzerIDs = DemoBaseline().Keep().Analyzers
	DemoUserID      = uuid.NewSHA1(demoNamespace, []byte("user"))
)

// DemoEmail is the demo login.
const DemoEmail = "demo@ekokod.com.tr"

const demoHistory = 400 * 24 * time.Hour

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// DemoResult reports what a seed or extension did.
type DemoResult struct {
	Created  bool
	Readings int
}

// Demo creates the demo company or converges it to its baseline, then fills
// readings and production up to the current hour.
func Demo(ctx context.Context, d DemoDeps, password string, now time.Time) (DemoResult, error) {
	var res DemoResult
	if d.Billing == nil {
		return res, errors.New("seed demo: DemoDeps.Billing is required")
	}
	if d.Storage.Root == "" {
		return res, errors.New("seed demo: DemoDeps.Storage.Root is required")
	}
	switch _, err := postgres.NewCompanyRepository(d.Pool).Get(ctx, store.SystemScope(DemoCompanyID), DemoCompanyID); {
	case errors.Is(err, store.ErrNotFound):
		if password == "" {
			return res, errors.New("seed demo: a password is required to create the demo user")
		}
		res.Created = true
	case err != nil:
		return res, err
	}
	// Refuse a weak password before anything is written.
	if password != "" {
		in := auth.PolicyInput{Password: password, Name: "Demo Kullanıcı", Email: DemoEmail, CompanyName: "Demo Enerji A.Ş."}
		if codes := auth.CheckPolicy(d.Hasher, in); len(codes) > 0 {
			return res, fmt.Errorf("%w: %v", ErrWeakPassword, codes)
		}
	}
	b := DemoBaseline()
	if err := ensureDemoSites(ctx, d, b, now); err != nil {
		return res, err
	}
	if err := ensureDemoPeople(ctx, d, b, password, now); err != nil {
		return res, err
	}
	n, err := ensureDemoSeries(ctx, d, b, now)
	res.Readings = n
	if err != nil {
		return res, err
	}
	if err := ensureDemoMoney(ctx, d, b, now); err != nil {
		return res, err
	}
	return res, ensureDemoContent(ctx, d, b, now)
}

// ExtendDemo appends hourly readings and production up to now when the demo
// company exists; otherwise it does nothing (R186).
func ExtendDemo(ctx context.Context, d DemoDeps, now time.Time) (int, error) {
	if _, err := postgres.NewCompanyRepository(d.Pool).Get(ctx, store.SystemScope(DemoCompanyID), DemoCompanyID); errors.Is(err, store.ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return ensureDemoSeries(ctx, d, DemoBaseline(), now)
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

type demoRegisters struct{ active, inductive, capacitive decimal.Decimal }

// advance adds the consumption of the hour starting at ts: a deterministic
// weekday/weekend load curve with ±10 % noise, 18–26 % inductive and 2–6 %
// capacitive shares, so re-running always produces the same history.
func (r *demoRegisters) advance(p readingProfile, ts time.Time) {
	local := ts.In(istanbul)
	day, night := p.day, p.night
	base := night
	if h := local.Hour(); h >= 6 && h < 18 {
		base = day
	}
	if wd := local.Weekday(); wd == time.Saturday || wd == time.Sunday {
		base = base * 4 / 10
	}
	seed := uint32(42000 + local.YearDay()*100 + local.Year()*37000 + local.Hour() + int(p.day)*7)
	noise := mulberry32(seed)
	step := decimal.NewFromInt(base).Mul(decimal.New(int64(900+noise%201), -3))
	r.active = r.active.Add(step)
	r.inductive = r.inductive.Add(step.Mul(decimal.New(int64(180+mulberry32(seed+1)%81), -3)))
	r.capacitive = r.capacitive.Add(step.Mul(decimal.New(int64(20+mulberry32(seed+2)%41), -3)))
}

// mulberry32 is legacy demoDataset.ts's generator, returning the raw 32-bit output.
func mulberry32(seed uint32) uint32 {
	seed += 0x6d2b79f5
	t := seed
	t = (t ^ (t >> 15)) * (1 | seed)
	t = (t + (t^(t>>7))*(61|t)) ^ t
	return t ^ (t >> 14)
}

// DemoJob adapts ExtendDemo and ResetDemo to job.DemoExtender and
// job.DemoResetter.
type DemoJob struct {
	Deps  DemoDeps
	Lock  lock.Locker
	Clock interface{ Now() time.Time }
}

// ExtendDemo implements job.DemoExtender; it stands aside while a reset runs.
func (j DemoJob) ExtendDemo(ctx context.Context) error {
	lease, err := tryLock(ctx, j.Lock, DemoResetLock, demoResetTTL)
	if lease == nil || err != nil {
		return err
	}
	defer func() { _ = lease.Release(context.WithoutCancel(ctx)) }()
	_, err = ExtendDemo(ctx, j.Deps, j.Clock.Now())
	return err
}

// ResetDemo implements job.DemoResetter.
func (j DemoJob) ResetDemo(ctx context.Context) error {
	started := time.Now()
	res, err := ResetDemo(ctx, j.Deps, j.Lock, j.Clock.Now())
	if err == nil && !res.Skipped && j.Deps.Log != nil {
		j.Deps.Log.InfoContext(ctx, "demo reset", "readings", res.Readings, "took", time.Since(started).Round(time.Millisecond))
	}
	return err
}

// DemoResetLock is the lock key a reset holds.
const DemoResetLock = "demo:reset"

// ResetResult reports what a reset did.
type ResetResult struct {
	Found    bool // a demo company exists
	Skipped  bool // another reset holds the lock
	Purged   admin.PurgeResult
	Readings int
}

// demoResetTTL outlives the slowest reset seen (about a minute) by far, and
// still frees the lock well before the next hourly run.
const demoResetTTL = 10 * time.Minute

// ResetDemo returns the demo company to its baseline: everything a visitor
// changed or added is purged, the emptied aggregate buckets are recomputed,
// the company's files are removed and the generator runs again (R547, R548).
// A running reset or a missing demo company makes it a no-op.
func ResetDemo(ctx context.Context, d DemoDeps, locker lock.Locker, now time.Time) (ResetResult, error) {
	var res ResetResult
	var exists bool
	if err := d.Pool.QueryRow(ctx, `select exists (select 1 from companies where id = $1 and is_demo)`, DemoCompanyID).Scan(&exists); err != nil || !exists {
		return res, err
	}
	res.Found = true
	lease, err := tryLock(ctx, locker, DemoResetLock, demoResetTTL)
	if lease == nil || err != nil {
		res.Skipped = err == nil
		return res, err
	}
	defer func() { _ = lease.Release(context.WithoutCancel(ctx)) }()
	if res.Purged, err = admin.NewDemoRepository(d.Pool).Purge(ctx, DemoCompanyID, DemoBaseline().Keep()); err != nil {
		return res, fmt.Errorf("demo reset: %w", err)
	}
	if err := refreshPurged(ctx, d, res.Purged); err != nil {
		return res, err
	}
	if err := d.Storage.RemoveCompany(DemoCompanyID); err != nil {
		return res, fmt.Errorf("demo reset: %w", err)
	}
	gen, err := Demo(ctx, d, "", now)
	res.Readings = gen.Readings
	return res, err
}

// tryLock takes the lease without queueing behind a holder: a nil lease and a
// nil error mean someone else holds it.
func tryLock(ctx context.Context, locker lock.Locker, key string, ttl time.Duration) (lock.Lease, error) {
	try, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	lease, err := locker.Acquire(try, key, ttl)
	if errors.Is(err, lock.ErrNotAcquired) && ctx.Err() == nil {
		return nil, nil
	}
	return lease, err
}

// refreshPurged recomputes only the buckets the purge emptied: visitor meters'
// readings and visitor plants' production (R548).
func refreshPurged(ctx context.Context, d DemoDeps, p admin.PurgeResult) error {
	aggregates := admin.NewAggregateRepository(d.Pool)
	if r := p.ReadingRange; r != nil {
		for _, view := range []store.AggregateView{store.ViewConsumptionHourly, store.ViewConsumptionDaily} {
			err := refreshDays(r.From, istanbulDay(r.To).AddDate(0, 0, 1), func(w store.TimeRange) error {
				return aggregates.Refresh(ctx, view, w)
			})
			if err != nil {
				return fmt.Errorf("demo reset refresh %s: %w", view, err)
			}
		}
	}
	if r := p.PlantRange; r != nil {
		first := istanbulDay(r.From)
		for m := first.AddDate(0, 0, 1-first.Day()); !m.After(r.To); m = m.AddDate(0, 1, 0) {
			if err := aggregates.RefreshPlantProduction(ctx, store.TimeRange{From: m.AddDate(0, 0, -1), To: m.AddDate(0, 1, 1)}); err != nil {
				return fmt.Errorf("demo reset refresh plant views: %w", err)
			}
		}
	}
	return nil
}
