package seed

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// ensureBillsAndTariffs seeds what the F8a screens need to be walked: the
// analyzer invoices the dashboard lists, a building invoice that differs from
// their sum (02 §6.11, so the R234 note is exercised for real), a template, a
// bulk assignment, an analysed icmal import, a plant with a feed-in price and
// two national-schedule rows. Every step is idempotent: the seeder re-runs.
func ensureBillsAndTariffs(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	local := now.In(istanbul)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0)
	end := start.AddDate(0, 1, 0)
	period := start.Format("2006-01")

	if err := ensureAnalyzerBill(ctx, pool, sc, f.BuildingA1, f.AnalyzerA1, period, start, end,
		"12450.500", "3.120000", "25400.25", now); err != nil {
		return err
	}
	if err := ensureAnalyzerBill(ctx, pool, sc, f.BuildingA2, f.AnalyzerA2, period, start, end,
		"6000.000", "3.120000", "18320.50", now); err != nil {
		return err
	}
	if err := ensureTemplateAndAssignment(ctx, pool, sc, f, start, now); err != nil {
		return err
	}
	if err := ensureIcmalImport(ctx, pool, sc, f, now); err != nil {
		return err
	}
	if err := ensurePlantTariff(ctx, pool, sc, f, start, now); err != nil {
		return err
	}
	return ensureNationalSchedule(ctx, pool, start)
}

func ensureAnalyzerBill(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, building, analyzer uuid.UUID,
	period string, start, end time.Time, kwh, price, total string, now time.Time,
) error {
	repo := postgres.NewBillRepository(pool)
	switch _, err := repo.Current(ctx, sc, model.BillScopeAnalyzer, analyzer, period); {
	case err == nil:
		return nil
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	dec := decimal.RequireFromString
	unit := dec(price)
	energy := dec(total)
	bill := model.Bill{
		CompanyID: sc.CompanyID, BuildingID: &building, AnalyzerID: &analyzer, Scope: model.BillScopeAnalyzer,
		PeriodKey: period, PeriodStart: start, PeriodEnd: end, DaysInPeriod: int32(end.Sub(start).Hours() / 24),
		Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
		ActiveImport: dec(kwh), NetConsumption: dec(kwh), ActiveExport: decimal.Zero,
		EffectiveEnergyPrice: &unit, EnergyCost: energy, TotalCost: energy,
		ComputedAt: now, IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
	}
	lines := []model.BillLine{{Code: model.BillLineEnergy, Label: "Enerji bedeli", Amount: energy}}
	_, err := repo.Create(ctx, sc, bill, lines, []uuid.UUID{analyzer})
	return err
}

func ensureTemplateAndAssignment(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, f Fixtures,
	effectiveFrom, now time.Time,
) error {
	templates := postgres.NewTariffTemplateRepository(pool)
	existing, err := templates.List(ctx, sc, store.TariffTemplateFilter{Page: store.Page{Limit: 10}})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"Tariff": map[string]any{
			"Name": "Ticari AG", "EffectiveFrom": effectiveFrom, "Currency": "TRY", "EnergyType": "grid_energy",
			"VoltageLevel": "lv", "UserGroup": "commercial", "PriceType": "single_time", "Term": "monomial",
			"SupplyCompany": "incumbent", "SingleTimePrice": "3.4547", "DistributionCost": "2.4794",
			"ReactivePowerPrice": "3.4937", "GenerationUsage": "none", "VatRate": "20",
			"PowerPriceSource": "kbk", "ReactivePriceSource": "kbk", "DistributionPriceSource": "kbk",
		},
		"VatRate": "20",
	})
	if err != nil {
		return err
	}
	template, err := templates.Create(ctx, sc, model.TariffTemplate{
		CompanyID: sc.CompanyID, Name: "Ticari AG", IsDefault: true, Payload: payload, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	_, err = postgres.NewBulkAssignmentRepository(pool).Create(ctx, sc, model.TariffBulkAssignment{
		CompanyID: sc.CompanyID, TemplateID: &template.ID, TariffName: &template.Name,
		EffectiveFrom: effectiveFrom, BuildingIDs: []uuid.UUID{f.BuildingA1, f.BuildingA2},
	})
	return err
}

func ensureIcmalImport(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, f Fixtures, now time.Time) error {
	repo := postgres.NewIcmalRepository(pool)
	existing, err := repo.ListImports(ctx, sc, store.IcmalFilter{Page: store.Page{Limit: 10}})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	uploadedBy := f.Users[E2ECompanyAdminEmail]
	imp, err := repo.CreateImport(ctx, sc, model.IcmalImport{
		CompanyID: sc.CompanyID, UploadedBy: &uploadedBy, FileName: "icmal-2025-11-12.csv",
		RowCount: 2, Status: "pending", CreatedAt: now,
	})
	if err != nil {
		return err
	}
	// The stored analysis mirrors what the domain package derives, so the
	// review screen can be walked without uploading a file first.
	result, err := json.Marshal(map[string]any{
		"analyses": []map[string]any{{
			"EtsoCode": "40ZE2E0000000001", "Periods": []string{"202511", "202512"},
			"EnergyKbk":            map[string]any{"Value": "1.0800", "Samples": 2, "Stable": true, "BackCalcErrorPct": "0.4"},
			"DistributionTlPerKwh": map[string]any{"Value": "0.8500", "Samples": 2, "Stable": true},
			"PowerUnitPrice":       map[string]any{"Value": "43.3700", "Samples": 2, "Stable": false},
			"ReactiveUnitPrice":    map[string]any{"Value": "3.4900", "Samples": 2, "Stable": true},
			"ReactiveKbk":          map[string]any{"Value": "1.1000", "Samples": 2, "Stable": true},
			"VatRate":              map[string]any{"Value": "20", "Samples": 2, "Stable": true},
			"Taxes":                map[string]any{"BTV": map[string]any{"Value": "5", "Samples": 2, "Stable": true}},
			"WithinTolerance":      true,
			"Warnings": []map[string]any{
				{"Row": 2, "Code": "power_unstable", "Text": "güç fiyatı dönemler arasında kararsız"},
			},
		}},
		"unmatched": []string{"40ZE2E0000000009"},
		"warnings":  []any{},
		"buildings": map[string]string{"40ZE2E0000000001": f.BuildingA1.String()},
	})
	if err != nil {
		return err
	}
	_, err = repo.UpdateImportResult(ctx, sc, imp.ID, "analysed", result)
	return err
}

func ensurePlantTariff(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, f Fixtures,
	effectiveFrom, now time.Time,
) error {
	plants := postgres.NewPlantRepository(pool)
	existing, err := plants.List(ctx, sc, store.PlantFilter{Page: store.Page{Limit: 10}})
	if err != nil {
		return err
	}
	plantID := uuid.Nil
	if len(existing) > 0 {
		plantID = existing[0].ID
	} else {
		capacity := decimal.RequireFromString("250.00")
		plant, err := plants.Create(ctx, sc, model.PowerPlant{
			CompanyID: sc.CompanyID, Name: "E2E Çatı GES", PlantKind: "rooftop", TotalCapacityKw: &capacity,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		plantID = plant.ID
	}
	solar := postgres.NewSolarTariffRepository(pool)
	tariffs, err := solar.List(ctx, sc, store.SolarTariffFilter{PlantID: &plantID, Page: store.Page{Limit: 10}})
	if err != nil {
		return err
	}
	if len(tariffs) > 0 {
		return nil
	}
	purchase := decimal.RequireFromString("3.100000")
	_, err = solar.Create(ctx, sc, model.SolarTariff{
		CompanyID: sc.CompanyID, PlantID: plantID, EffectiveFrom: effectiveFrom,
		FeedInTariff: decimal.RequireFromString("2.500000"), PurchasePrice: &purchase,
		Currency: model.CurrencyTRY, CreatedAt: now,
	})
	return err
}

func ensureNationalSchedule(ctx context.Context, pool *pgxpool.Pool, effectiveFrom time.Time) error {
	source := "EPDK"
	entries := []model.NationalTariffScheduleEntry{
		{
			EffectiveFrom: effectiveFrom, UserGroup: model.UserGroupCommercial, VoltageLevel: model.VoltageLevelLV,
			Term: model.TariffTermMonomial, EnergyPrice: decimal.RequireFromString("3.100000"),
			DistributionPrice: decimal.RequireFromString("2.400000"), VatRate: decimal.RequireFromString("20"),
			Source: &source,
		},
		{
			EffectiveFrom: effectiveFrom, UserGroup: model.UserGroupIndustrial, VoltageLevel: model.VoltageLevelMV,
			Term: model.TariffTermBinomial, EnergyPrice: decimal.RequireFromString("2.900000"),
			DistributionPrice: decimal.RequireFromString("2.100000"), VatRate: decimal.RequireFromString("20"),
			Source: &source,
		},
	}
	_, err := admin.NewCatalogueRepository(pool).UpsertNationalTariffSchedule(ctx, entries)
	return err
}
