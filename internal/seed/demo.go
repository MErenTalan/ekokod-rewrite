package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// The synthetic demo company (R138, R186). Every id is fixed.
var (
	demoNamespace   = uuid.MustParse("7c1e2a55-0d6b-4e8e-9d3b-3f7e5a2c9b40")
	DemoCompanyID   = uuid.NewSHA1(demoNamespace, []byte("company"))
	DemoBuildingID  = uuid.NewSHA1(demoNamespace, []byte("building"))
	DemoAnalyzerIDs = []uuid.UUID{uuid.NewSHA1(demoNamespace, []byte("analyzer/1")), uuid.NewSHA1(demoNamespace, []byte("analyzer/2"))}
	DemoUserID      = uuid.NewSHA1(demoNamespace, []byte("user"))
)

// DemoEmail is the demo login.
const DemoEmail = "demo@ekokod.com.tr"

const demoHistory = 180 * 24 * time.Hour

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

// Demo creates the demo company, building, analyzers, calendar, tariff and
// user when absent, then extends readings to the current hour.
func Demo(ctx context.Context, pool *pgxpool.Pool, hasher auth.Hasher, password string, now time.Time) (DemoResult, error) {
	sc := store.SystemScope(DemoCompanyID)
	companies := postgres.NewCompanyRepository(pool)
	var res DemoResult
	if _, err := companies.Get(ctx, sc, DemoCompanyID); errors.Is(err, store.ErrNotFound) {
		if password == "" {
			return res, errors.New("seed demo: a password is required to create the demo user")
		}
		if err := createDemo(ctx, pool, hasher, password, now); err != nil {
			return res, err
		}
		res.Created = true
	} else if err != nil {
		return res, err
	}
	n, err := extendDemo(ctx, pool, now)
	res.Readings = n
	return res, err
}

func createDemo(ctx context.Context, pool *pgxpool.Pool, hasher auth.Hasher, password string, now time.Time) error {
	sc := store.SystemScope(DemoCompanyID)
	if codes := auth.CheckPolicy(hasher, auth.PolicyInput{Password: password, Name: "Demo Kullanıcı", Email: DemoEmail, CompanyName: "Demo Enerji A.Ş."}); len(codes) > 0 {
		return fmt.Errorf("%w: %v", ErrWeakPassword, codes)
	}
	sector, area, personnel := "Üretim", decimal.RequireFromString("12500.00"), int32(180)
	if _, err := postgres.NewCompanyRepository(pool).Create(ctx, sc, model.Company{
		ID: DemoCompanyID, Name: "Demo Enerji A.Ş.", Sector: &sector, TotalAreaM2: &area, PersonnelCount: &personnel, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed demo company: %w", err)
	}
	hash, err := hasher.Hash(password)
	if err != nil {
		return err
	}
	if _, err := postgres.NewUserRepository(pool).Create(ctx, sc, model.User{
		ID: DemoUserID, CompanyID: DemoCompanyID, Name: "Demo Kullanıcı", Email: DemoEmail, PasswordHash: hash,
		Role: model.UserRoleDemo, IsActive: true, Locale: "tr", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed demo user: %w", err)
	}
	lat, lon := decimal.RequireFromString("39.925533"), decimal.RequireFromString("32.866287")
	address := "Ostim OSB, Ankara"
	if _, err := postgres.NewBuildingRepository(pool).Create(ctx, sc, model.Building{
		ID: DemoBuildingID, CompanyID: DemoCompanyID, Name: "Demo Fabrika", Address: &address, Latitude: &lat, Longitude: &lon,
		Sector: &sector, TotalAreaM2: &area, PersonnelCount: &personnel, BillCutoffDay: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("seed demo building: %w", err)
	}
	analyzers := postgres.NewAnalyzerRepository(pool)
	for i, power := range []string{"250.00", "45.00"} {
		kw := decimal.RequireFromString(power)
		building := DemoBuildingID
		if _, err := analyzers.Create(ctx, sc, model.Analyzer{
			ID: DemoAnalyzerIDs[i], CompanyID: DemoCompanyID, BuildingID: &building, Provider: model.IntegrationProviderOSOS,
			ProviderSubtype: "Demo", InstallationNumber: fmt.Sprintf("DEMO-%04d", i+1), InstalledPowerKw: &kw,
			MeterMultiplier: decimal.NewFromInt(1), Latitude: &lat, Longitude: &lon, IsActive: true, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("seed demo analyzer: %w", err)
		}
	}
	calendar := postgres.NewCalendarRepository(pool)
	if err := calendar.ReplaceWeekendDays(ctx, sc, []int16{0, 6}); err != nil {
		return fmt.Errorf("seed demo weekend days: %w", err)
	}
	holiday := time.Date(now.In(istanbul).Year(), time.October, 29, 0, 0, 0, 0, istanbul)
	description := "Cumhuriyet Bayramı"
	if _, err := calendar.CreateVacation(ctx, sc, model.CompanyVacation{CompanyID: DemoCompanyID, StartDate: holiday, EndDate: holiday, Description: &description}); err != nil {
		return fmt.Errorf("seed demo vacation: %w", err)
	}
	name, building := "Demo Ticari Tarife", DemoBuildingID
	contracted, powerPrice, single := decimal.RequireFromString("200"), decimal.RequireFromString("25.5"), decimal.RequireFromString("3.4547")
	tariffs := postgres.NewTariffRepository(pool)
	tariff, err := tariffs.Create(ctx, sc, model.Tariff{
		CompanyID: DemoCompanyID, BuildingID: &building, Name: &name,
		EffectiveFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, istanbul), Currency: model.CurrencyTRY, EnergyType: model.EnergyTypeGrid,
		VoltageLevel: model.VoltageLevelLV, UserGroup: model.UserGroupCommercial, PriceType: model.PriceTypeSingleTime,
		Term: model.TariffTermBinomial, SupplyCompany: model.SupplyCompanyIncumbent, SingleTimePrice: &single,
		DistributionCost: decimal.RequireFromString("2.4794"), ReactivePowerPrice: decimal.RequireFromString("3.4937"),
		ContractedPowerKw: &contracted, PowerUnitPrice: &powerPrice, GenerationUsage: model.GenerationUsageNone,
		VatRate: decimal.NewFromInt(20), PowerPriceSource: model.PriceSourceFixed, ReactivePriceSource: model.PriceSourceFixed,
		DistributionPriceSource: model.PriceSourceFixed, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("seed demo tariff: %w", err)
	}
	if _, err := tariffs.ReplaceTaxes(ctx, sc, tariff.ID, []model.TariffTax{{TariffID: tariff.ID, Name: "BTV", Rate: decimal.NewFromInt(5)}}); err != nil {
		return fmt.Errorf("seed demo taxes: %w", err)
	}
	return nil
}

// ExtendDemo appends hourly readings up to now when the demo company exists;
// otherwise it does nothing (R186).
func ExtendDemo(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int, error) {
	if _, err := postgres.NewCompanyRepository(pool).Get(ctx, store.SystemScope(DemoCompanyID), DemoCompanyID); errors.Is(err, store.ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return extendDemo(ctx, pool, now)
}

func extendDemo(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int, error) {
	sc := store.SystemScope(DemoCompanyID)
	analyzers := postgres.NewAnalyzerRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	end := now.UTC().Truncate(time.Hour)
	total := 0
	earliest := end
	for i, id := range DemoAnalyzerIDs {
		a, err := analyzers.Get(ctx, sc, id)
		if err != nil {
			return total, fmt.Errorf("seed demo analyzer %s: %w", id, err)
		}
		start := end.Add(-demoHistory)
		registers := demoRegisters{active: decimal.NewFromInt(100000), inductive: decimal.NewFromInt(20000), capacitive: decimal.NewFromInt(4000)}
		if a.LastReadingAt != nil {
			last, err := readings.Range(ctx, sc, id, store.TimeRange{From: *a.LastReadingAt, To: a.LastReadingAt.Add(time.Second)}, model.ReadingKindLoadProfile)
			if err != nil {
				return total, err
			}
			if len(last) == 1 && last[0].ActiveImport != nil && last[0].ReactiveInductiveImport != nil && last[0].ReactiveCapacitiveImport != nil {
				registers = demoRegisters{*last[0].ActiveImport, *last[0].ReactiveInductiveImport, *last[0].ReactiveCapacitiveImport}
				registers.advance(i, *a.LastReadingAt)
				start = a.LastReadingAt.Add(time.Hour)
			}
		}
		if start.After(end) {
			continue
		}
		earliest = minTime(earliest, start)
		var batch []model.MeterReading
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			_, _, err := readings.BulkInsert(ctx, sc, batch)
			total += len(batch)
			batch = batch[:0]
			return err
		}
		for ts := start; !ts.After(end); ts = ts.Add(time.Hour) {
			a, ind, capa := registers.active, registers.inductive, registers.capacitive
			batch = append(batch, model.MeterReading{
				AnalyzerID: id, Ts: ts, Kind: model.ReadingKindLoadProfile, ActiveImport: &a, ReactiveInductiveImport: &ind,
				ReactiveCapacitiveImport: &capa, MultiplierApplied: decimal.NewFromInt(1), SourceProvider: model.IntegrationProviderOSOS, IngestedAt: now,
			})
			registers.advance(i, ts)
			if len(batch) == 5000 {
				if err := flush(); err != nil {
					return total, err
				}
			}
		}
		if err := flush(); err != nil {
			return total, err
		}
		if err := analyzers.TouchLastReading(ctx, sc, id, end); err != nil {
			return total, err
		}
	}
	if total == 0 {
		return 0, nil
	}
	// Monthly and yearly figures are composed from the daily view (R94), so the
	// two finest views are enough for backfilled history to become visible.
	aggregates := admin.NewAggregateRepository(pool)
	for _, view := range []store.AggregateView{store.ViewConsumptionHourly, store.ViewConsumptionDaily} {
		if err := aggregates.Refresh(ctx, view, store.TimeRange{From: earliest.Add(-48 * time.Hour), To: end.Add(time.Hour)}); err != nil {
			return total, err
		}
	}
	return total, nil
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
func (r *demoRegisters) advance(analyzer int, ts time.Time) {
	local := ts.In(istanbul)
	day, night := int64(120), int64(50)
	if analyzer == 1 {
		day, night = 20, 8
	}
	base := night
	if h := local.Hour(); h >= 6 && h < 18 {
		base = day
	}
	if wd := local.Weekday(); wd == time.Saturday || wd == time.Sunday {
		base = base * 4 / 10
	}
	seed := uint32(42000 + local.YearDay()*100 + local.Year()*37000 + local.Hour() + analyzer*7)
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
	Pool  *pgxpool.Pool
	Clock interface{ Now() time.Time }
}

// ExtendDemo implements job.DemoExtender.
func (j DemoJob) ExtendDemo(ctx context.Context) error {
	_, err := ExtendDemo(ctx, j.Pool, j.Clock.Now())
	return err
}
