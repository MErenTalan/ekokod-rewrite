package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// E2EISolarPsID is the linked grid plant's iSolarCloud id.
const E2EISolarPsID = "E2E-PS-1"

// ensureSolar gives the solar screens real figures (F9): the grid plant linked
// to a stub iSolar credential (no secrets, so a sync fails honestly), two
// inverters with snapshots, sixty days of daily totals, two faults, A1's first
// analyzer as its netting analyzer and a feed-in tariff. Idempotent.
func ensureSolar(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	plants := postgres.NewPlantRepository(pool)
	kind := "grid"
	list, err := plants.List(ctx, sc, store.PlantFilter{PlantKind: &kind, Page: store.Page{Limit: 1}})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errors.New("seed: the grid plant must exist before ensureSolar")
	}
	plant := list[0]

	// The e2e stack seeds the catalogue first; a bare fixture database may not have.
	if _, err := loadIntegrationDefinitions(ctx, admin.NewCatalogueRepository(pool), discardLog); err != nil {
		return err
	}
	credID := e2eID("isolar-credential")
	if _, err := pool.Exec(ctx, `insert into integration_credentials (id, company_id, definition_id, is_active)
		select $1, $2, id, true from integration_definitions where provider = 'isolar' order by id limit 1
		on conflict do nothing`, credID, f.CompanyA); err != nil {
		return fmt.Errorf("seed: isolar credential: %w", err)
	}
	if plant.IsolarPsID == nil {
		psID, psName, installed, linked := E2EISolarPsID, "E2E Arazi GES (iSolar)", decimal.RequireFromString("100"), now.UTC()
		plant.IsolarPsID, plant.IsolarPsName, plant.IsolarInstalledKw, plant.IsolarLinkedAt = &psID, &psName, &installed, &linked
		plant.IsolarCredentialID = &credID
		if plant, err = plants.SetIsolarLink(ctx, sc, plant); err != nil {
			return err
		}
	}
	if plant.NettingAnalyzerID == nil {
		plant.NettingAnalyzerID = &f.AnalyzerA1
		if plant, err = plants.Update(ctx, sc, plant); err != nil {
			return err
		}
	}
	if err := plants.SetSyncState(ctx, sc, plant.ID, now.Add(-10*time.Minute).UTC(), nil); err != nil {
		return err
	}

	for i, inv := range []struct {
		sn, name, power, today, month, year string
		typ                                 int32
	}{{"E2E-INV-1", "İnverter 1", "42.5", "210.4", "4820.5", "51230", 1}, {"E2E-INV-2", "İnverter 2", "18.25", "96.6", "2210.25", "23480", 14}} {
		typ, key, name := inv.typ, fmt.Sprintf("E2E-KEY-%d", i+1), inv.name
		dev, err := plants.UpsertDevice(ctx, sc, model.PlantDevice{PlantID: plant.ID, DeviceSN: inv.sn, DeviceName: &name, DeviceType: &typ, ProviderKey: &key})
		if err != nil {
			return err
		}
		at, status := now.Add(-10*time.Minute).UTC(), int32(4)
		power, today, total := decimal.RequireFromString(inv.power), decimal.RequireFromString(inv.today), decimal.RequireFromString("152000")
		month, year := decimal.RequireFromString(inv.month), decimal.RequireFromString(inv.year)
		dev.SnapshotAt, dev.FaultStatus, dev.ActivePowerKw, dev.YieldTodayKwh, dev.YieldTotalKwh = &at, &status, &power, &today, &total
		dev.YieldMonthKwh, dev.YieldYearKwh = &month, &year
		if err := plants.UpdateDeviceSnapshot(ctx, sc, dev); err != nil {
			return err
		}
	}

	local := now.In(istanbul)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, istanbul)
	var rows []model.PlantProductionTotal
	var days []time.Time
	for i := 60; i >= 1; i-- {
		day := today.AddDate(0, 0, -i)
		v := decimal.NewFromInt(int64(260 + (i*37)%140))
		rows = append(rows, model.PlantProductionTotal{PlantID: plant.ID, Ts: day.Add(12 * time.Hour).UTC(), ProductionKwh: &v, Basis: model.BasisDailyTotal})
		days = append(days, day)
	}
	if _, err := postgres.NewProductionTotalsRepository(pool).ReplaceDays(ctx, sc, plant.ID, days, rows); err != nil {
		return err
	}
	for _, view := range []string{"plant_production_daily", "plant_production_monthly"} {
		if _, err := pool.Exec(ctx, "call refresh_continuous_aggregate('"+view+"', NULL, NULL)"); err != nil {
			return errors.Join(errors.New("seed: refresh "+view), err)
		}
	}

	level, typ, device := int32(2), int32(1), "İnverter 1"
	closed := today.AddDate(0, 0, -3).Add(15 * time.Hour).UTC()
	faults := []model.PlantFault{
		{PlantID: plant.ID, Ref: E2EISolarPsID + "|E2E-KEY-1|532|e2e-open", Code: "532", Name: "String akımı düşük", Level: &level, Type: &typ,
			DeviceName: &device, OccurredAt: today.Add(-2 * time.Hour).UTC(), FirstSeenAt: now.UTC()},
		{PlantID: plant.ID, Ref: E2EISolarPsID + "|E2E-KEY-1|039|e2e-closed", Code: "039", Name: "Şebeke gerilimi yüksek", Level: &level, Type: &typ,
			DeviceName: &device, OccurredAt: today.AddDate(0, 0, -3).Add(11 * time.Hour).UTC(), ClosedAt: &closed, FirstSeenAt: now.UTC()},
	}
	if _, err := postgres.NewFaultRepository(pool).Upsert(ctx, sc, faults); err != nil {
		return err
	}

	solar := postgres.NewSolarTariffRepository(pool)
	existing, err := solar.List(ctx, sc, store.SolarTariffFilter{PlantID: &plant.ID, Page: store.Page{Limit: 1}})
	if err != nil || len(existing) > 0 {
		return err
	}
	_, err = solar.Create(ctx, sc, model.SolarTariff{CompanyID: f.CompanyA, PlantID: plant.ID,
		EffectiveFrom: time.Date(local.Year()-1, 1, 1, 0, 0, 0, 0, istanbul), FeedInTariff: decimal.RequireFromString("2.000000"),
		Currency: model.CurrencyTRY, CreatedAt: now})
	return err
}
