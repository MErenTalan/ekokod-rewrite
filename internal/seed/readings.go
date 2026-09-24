package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// readingProfile is one analyzer's deterministic load curve: the kWh a working
// hour and a night hour add to the active register (a weekend hour is 40 % of
// either). Index 0 is a factory, 1 a small site, 2 an office.
type readingProfile struct{ day, night int64 }

var readingProfiles = []readingProfile{{120, 50}, {20, 8}, {60, 25}}

// fillHourly writes hourly load-profile readings for ids from history ago (or
// from each analyzer's last reading) up to the current hour, then refreshes the
// hourly and daily aggregates over what it wrote. It is idempotent: a second
// run at the same instant writes nothing.
//
// The demo company (R186) and the e2e fixtures (R194) share it, so both look
// like a real site to every screen.
func fillHourly(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, ids []uuid.UUID, profiles []int,
	history time.Duration, now time.Time) (int, error) {
	analyzerRepo := postgres.NewAnalyzerRepository(pool)
	readingRepo := postgres.NewReadingRepository(pool)
	end := now.UTC().Truncate(time.Hour)
	total := 0
	earliest := end
	for i, id := range ids {
		profile := readingProfiles[profiles[i]%len(readingProfiles)]
		a, err := analyzerRepo.Get(ctx, sc, id)
		if err != nil {
			return total, fmt.Errorf("seed readings for analyzer %s: %w", id, err)
		}
		start := end.Add(-history)
		registers := demoRegisters{active: decimal.NewFromInt(100000), inductive: decimal.NewFromInt(20000), capacitive: decimal.NewFromInt(4000)}
		if a.LastReadingAt != nil {
			last, lerr := readingRepo.Range(ctx, sc, id, store.TimeRange{From: *a.LastReadingAt, To: a.LastReadingAt.Add(time.Second)},
				model.ReadingKindLoadProfile)
			if lerr != nil {
				return total, lerr
			}
			if len(last) == 1 && last[0].ActiveImport != nil && last[0].ReactiveInductiveImport != nil && last[0].ReactiveCapacitiveImport != nil {
				registers = demoRegisters{*last[0].ActiveImport, *last[0].ReactiveInductiveImport, *last[0].ReactiveCapacitiveImport}
				registers.advance(profile, *a.LastReadingAt)
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
			_, _, err := readingRepo.BulkInsert(ctx, sc, batch)
			total += len(batch)
			batch = batch[:0]
			return err
		}
		for ts := start; !ts.After(end); ts = ts.Add(time.Hour) {
			active, inductive, capacitive := registers.active, registers.inductive, registers.capacitive
			batch = append(batch, model.MeterReading{
				AnalyzerID: id, Ts: ts, Kind: model.ReadingKindLoadProfile, ActiveImport: &active,
				ReactiveInductiveImport: &inductive, ReactiveCapacitiveImport: &capacitive,
				MultiplierApplied: decimal.NewFromInt(1), SourceProvider: a.Provider, IngestedAt: now,
			})
			registers.advance(profile, ts)
			if len(batch) == 5000 {
				if err := flush(); err != nil {
					return total, err
				}
			}
		}
		if err := flush(); err != nil {
			return total, err
		}
		if err := analyzerRepo.TouchLastReading(ctx, sc, id, end); err != nil {
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

// ensureBill writes one issued building bill for the previous calendar month,
// so the dashboard's latest-bill card has something real to show (R194). It
// does nothing when the period already has a bill.
func ensureBill(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, buildingID uuid.UUID, now time.Time) error {
	local := now.In(istanbul)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0)
	end := start.AddDate(0, 1, 0)
	period := start.Format("2006-01")
	repo := postgres.NewBillRepository(pool)
	switch _, err := repo.Current(ctx, sc, model.BillScopeBuilding, buildingID, period); {
	case err == nil:
		return nil
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	dec := decimal.RequireFromString
	bill := model.Bill{
		CompanyID: sc.CompanyID, BuildingID: &buildingID, Scope: model.BillScopeBuilding, PeriodKey: period,
		PeriodStart: start, PeriodEnd: end, DaysInPeriod: int32(end.Sub(start).Hours() / 24),
		Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
		ActiveImport: dec("18450.500"), NetConsumption: dec("18450.500"), InductiveKvarh: dec("3321.090"),
		CapacitiveKvarh: dec("553.500"), EnergyCost: dec("31800.25"), DistributionCost: dec("8400.10"),
		PowerCost: dec("2700.00"), ReactivePenalty: decimal.Zero, VatCost: dec("5350.40"), TotalCost: dec("48250.75"),
		ComputedAt: now, IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`),
	}
	lines := []model.BillLine{
		{Code: "energy", Label: "Enerji bedeli", Amount: bill.EnergyCost},
		{Code: "distribution", Label: "Dağıtım bedeli", Amount: bill.DistributionCost},
		{Code: "power", Label: "Güç bedeli", Amount: bill.PowerCost},
		{Code: "vat", Label: "KDV", Amount: bill.VatCost},
	}
	_, err := repo.Create(ctx, sc, bill, lines, nil)
	return err
}
