package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
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

// DemoJob adapts ExtendDemo to job.DemoExtender.
type DemoJob struct {
	Deps  DemoDeps
	Clock interface{ Now() time.Time }
}

// ExtendDemo implements job.DemoExtender.
func (j DemoJob) ExtendDemo(ctx context.Context) error {
	_, err := ExtendDemo(ctx, j.Deps, j.Clock.Now())
	return err
}
