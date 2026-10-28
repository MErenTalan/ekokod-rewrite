package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// NewDemoBilling builds the billing engine for a process that has none (the
// CLI and tests); the worker passes its own.
func NewDemoBilling(pool *pgxpool.Pool, log *slog.Logger) (*billingsvc.Service, error) {
	analyzers, anomalies, ops := postgres.NewAnalyzerRepository(pool), postgres.NewAnomalyRepository(pool), postgres.NewOpsRepository(pool)
	cons, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: postgres.NewReadingRepository(pool), Anomalies: anomalies, Ops: ops, Clock: clock.System(), Log: log,
		Locker: lock.NewMemory(nil), Analyzers: analyzers, Users: postgres.NewUserRepository(pool),
	})
	if err != nil {
		return nil, err
	}
	return billingsvc.New(billingsvc.Deps{
		Consumption: cons, Buildings: postgres.NewBuildingRepository(pool), Analyzers: analyzers,
		Tariffs: postgres.NewTariffRepository(pool), Params: postgres.NewBillingParameterRepository(pool),
		Prices: postgres.NewPriceRepository(pool), Anomalies: anomalies, Bills: postgres.NewBillRepository(pool), Ops: ops,
		Clock: clock.System(), Log: log,
	})
}

type demoTariff struct {
	name                    string
	voltage                 model.VoltageLevel
	group                   model.DistributionUserGroup
	term                    model.TariffTerm
	single, t1, t2, t3      string // TL/kWh; empty = unused
	distribution, btv       string
	contractedKw, powerUnit string
	generation              model.GenerationUsage
	generationPrice         string
}

// demoTariffs is one tariff per baseline building, in building order; prices
// are fixed so bills never depend on PTF (R551).
var demoTariffs = []demoTariff{
	{name: "OG Sanayi Çift Terimli Üç Zamanlı", voltage: model.VoltageLevelMV, group: model.UserGroupIndustrial,
		term: model.TariffTermBinomial, t1: "3.1250", t2: "4.6875", t3: "1.9531", distribution: "1.0834", btv: "1",
		contractedKw: "400", powerUnit: "32.10", generation: model.GenerationUsageNone},
	{name: "AG Ticarethane Tek Terimli Tek Zamanlı", voltage: model.VoltageLevelLV, group: model.UserGroupCommercial,
		term: model.TariffTermMonomial, single: "3.4547", distribution: "2.4794", btv: "5", generation: model.GenerationUsageNone},
	{name: "AG Sanayi Tek Zamanlı — Mahsuplaşmalı", voltage: model.VoltageLevelLV, group: model.UserGroupIndustrial,
		term: model.TariffTermMonomial, single: "3.2010", distribution: "2.2850", btv: "1",
		generation: model.GenerationUsageSubtractFromConsumption, generationPrice: "2.0000"},
}

var demoTariffStart = time.Date(2024, 1, 1, 0, 0, 0, 0, istanbul)

// ensureDemoMoney writes tariffs, a template with a bulk assignment, solar
// tariffs, the last 12 closed months of bills computed by the real engine and
// an icmal import that matches the factory's bills. Every step is skipped
// when its rows already exist.
func ensureDemoMoney(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	if err := ensureDemoTariffs(ctx, d, b, now); err != nil {
		return err
	}
	if err := ensureDemoTemplate(ctx, d, b, now); err != nil {
		return err
	}
	if err := ensureDemoSolarTariffs(ctx, d, b, now); err != nil {
		return err
	}
	local := now.In(istanbul)
	thisMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul)
	for m := 12; m >= 1; m-- {
		period := thisMonth.AddDate(0, -m, 0).Format("2006-01")
		for _, a := range b.analyzers {
			if _, err := d.Billing.Generate(ctx, sc, billingsvc.GenerateRequest{Scope: model.BillScopeAnalyzer, SubjectID: a.id, PeriodKey: period}); err != nil {
				return fmt.Errorf("seed demo bill %s %s: %w", a.number, period, err)
			}
		}
		for _, x := range b.buildings {
			if _, err := d.Billing.Generate(ctx, sc, billingsvc.GenerateRequest{Scope: model.BillScopeBuilding, SubjectID: x.id, PeriodKey: period}); err != nil {
				return fmt.Errorf("seed demo bill %s %s: %w", x.name, period, err)
			}
		}
	}
	return ensureDemoIcmal(ctx, d, b, thisMonth, now)
}

func ensureDemoTariffs(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewTariffRepository(d.Pool)
	for i, x := range b.buildings {
		have, err := repo.List(ctx, sc, store.TariffFilter{BuildingID: &x.id, Page: store.Page{Limit: 1}})
		if err != nil {
			return err
		}
		if len(have) > 0 {
			continue
		}
		spec := demoTariffs[i]
		price := func(v string) *decimal.Decimal {
			if v == "" {
				return nil
			}
			return ptr(decimal.RequireFromString(v))
		}
		t := model.Tariff{CompanyID: b.company, BuildingID: &x.id, Name: ptr(spec.name), EffectiveFrom: demoTariffStart,
			Currency: model.CurrencyTRY, EnergyType: model.EnergyTypeGrid, VoltageLevel: spec.voltage, UserGroup: spec.group,
			PriceType: model.PriceTypeSingleTime, Term: spec.term, SupplyCompany: model.SupplyCompanyIncumbent,
			SingleTimePrice: price(spec.single), T1Price: price(spec.t1), T2Price: price(spec.t2), T3Price: price(spec.t3),
			DistributionCost: decimal.RequireFromString(spec.distribution), ReactivePowerPrice: decimal.RequireFromString("3.4937"),
			ContractedPowerKw: price(spec.contractedKw), PowerUnitPrice: price(spec.powerUnit), GenerationUsage: spec.generation,
			GenerationPricePerKwh: price(spec.generationPrice), VatRate: decimal.NewFromInt(20),
			PowerPriceSource: model.PriceSourceFixed, ReactivePriceSource: model.PriceSourceFixed,
			DistributionPriceSource: model.PriceSourceFixed, CreatedBy: ptr(DemoUserID), CreatedAt: now, UpdatedAt: now}
		if spec.single == "" {
			t.PriceType = model.PriceTypeMultiTime
		}
		saved, err := repo.Create(ctx, sc, t)
		if err != nil {
			return fmt.Errorf("seed demo tariff %s: %w", spec.name, err)
		}
		if _, err := repo.ReplaceTaxes(ctx, sc, saved.ID, []model.TariffTax{
			{TariffID: saved.ID, Name: "BTV", Rate: decimal.RequireFromString(spec.btv), SortOrder: 1}}); err != nil {
			return fmt.Errorf("seed demo tariff taxes: %w", err)
		}
		if spec.term == model.TariffTermBinomial {
			if _, err := repo.ReplaceExtraCharges(ctx, sc, saved.ID, []model.TariffExtraCharge{{TariffID: saved.ID,
				Name: "Sayaç okuma bedeli", Basis: model.ExtraChargeBasisFixedPerPeriod, Amount: decimal.RequireFromString("125.00"),
				SortOrder: 1}}); err != nil {
				return fmt.Errorf("seed demo tariff extra charges: %w", err)
			}
		}
	}
	return nil
}

func ensureDemoTemplate(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	templates := postgres.NewTariffTemplateRepository(d.Pool)
	have, err := templates.List(ctx, sc, store.TariffTemplateFilter{Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	spec := demoTariffs[0]
	payload, err := json.Marshal(map[string]any{
		"Tariff": map[string]any{
			"Name": spec.name, "EffectiveFrom": demoTariffStart, "Currency": "TRY", "EnergyType": "grid_energy",
			"VoltageLevel": spec.voltage, "UserGroup": spec.group, "PriceType": "multi_time", "Term": spec.term,
			"SupplyCompany": "incumbent", "T1Price": spec.t1, "T2Price": spec.t2, "T3Price": spec.t3,
			"DistributionCost": spec.distribution, "ReactivePowerPrice": "3.4937", "ContractedPowerKw": spec.contractedKw,
			"PowerUnitPrice": spec.powerUnit, "GenerationUsage": "none", "VatRate": "20",
			"PowerPriceSource": "fixed", "ReactivePriceSource": "fixed", "DistributionPriceSource": "fixed",
		},
		"VatRate": "20",
		"Taxes":   []map[string]any{{"Name": "BTV", "Rate": spec.btv}},
	})
	if err != nil {
		return err
	}
	template, err := templates.Create(ctx, sc, model.TariffTemplate{CompanyID: b.company, Name: "Sanayi OG şablonu",
		Description: ptr("Fabrika binaları için çift terimli üç zamanlı OG tarifesi"), IsDefault: true, Payload: payload,
		CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return fmt.Errorf("seed demo tariff template: %w", err)
	}
	_, err = postgres.NewBulkAssignmentRepository(d.Pool).Create(ctx, sc, model.TariffBulkAssignment{CompanyID: b.company,
		TemplateID: &template.ID, TariffName: ptr(spec.name), EffectiveFrom: demoTariffStart,
		BuildingIDs: []uuid.UUID{b.buildings[0].id}, CreatedBy: ptr(DemoUserID), CreatedAt: now})
	return err
}

func ensureDemoSolarTariffs(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewSolarTariffRepository(d.Pool)
	for _, p := range b.plants {
		have, err := repo.List(ctx, sc, store.SolarTariffFilter{PlantID: &p.id, Page: store.Page{Limit: 1}})
		if err != nil {
			return err
		}
		if len(have) > 0 {
			continue
		}
		t := model.SolarTariff{CompanyID: b.company, PlantID: p.id, EffectiveFrom: demoTariffStart, Currency: model.CurrencyTRY, CreatedAt: now}
		if p.netting >= 0 {
			t.FeedInTariff, t.PurchasePrice, t.Notes = decimal.RequireFromString("2.0000"), ptr(decimal.RequireFromString("3.2010")),
				ptr("Çatı GES ihtiyaç fazlası satış fiyatı")
		} else {
			t.FeedInTariff, t.Notes = decimal.RequireFromString("2.1500"), ptr("Lisanssız arazi GES, YEKDEM fiyatı")
		}
		if _, err := repo.Create(ctx, sc, t); err != nil {
			return fmt.Errorf("seed demo solar tariff %s: %w", p.name, err)
		}
	}
	return nil
}

// ensureDemoIcmal mirrors the factory's last 12 building bills as a supplier
// icmal, so the import review screen has a matching analysis.
func ensureDemoIcmal(ctx context.Context, d DemoDeps, b demoBaseline, thisMonth, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewIcmalRepository(d.Pool)
	have, err := repo.ListImports(ctx, sc, store.IcmalFilter{Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	bills := postgres.NewBillRepository(d.Pool)
	factory, etso := b.buildings[0].id, "40ZDEMO0000001001"
	var rows []model.IcmalRow
	var periods []string
	for m := 12; m >= 1; m-- {
		period := thisMonth.AddDate(0, -m, 0).Format("2006-01")
		bill, err := bills.Current(ctx, sc, model.BillScopeBuilding, factory, period)
		if err != nil {
			return fmt.Errorf("seed demo icmal %s: %w", period, err)
		}
		key := bill.PeriodStart.In(istanbul).Format("200601")
		periods = append(periods, key)
		rows = append(rows, model.IcmalRow{BuildingID: &factory, Period: key, EtsoCode: &etso, TotalKwh: &bill.ActiveImport,
			EnergyCharge: &bill.EnergyCost, DistributionCharge: &bill.DistributionCost, ReactiveCharge: &bill.ReactivePenalty,
			PowerCharge: &bill.PowerCost, InductiveKvarh: &bill.InductiveKvarh, CapacitiveKvarh: &bill.CapacitiveKvarh,
			Vat: &bill.VatCost, Term: ptr("binomial"), VoltageLevel: ptr("mv"), IsMultiTime: ptr(true)})
	}
	imp, err := repo.CreateImport(ctx, sc, model.IcmalImport{CompanyID: b.company, UploadedBy: ptr(DemoUserID),
		FileName: "enerjisa-icmal-" + periods[0] + "-" + periods[len(periods)-1] + ".xlsx", RowCount: int32(len(rows)),
		Status: "pending", CreatedAt: now})
	if err != nil {
		return fmt.Errorf("seed demo icmal import: %w", err)
	}
	if _, err := repo.InsertRows(ctx, sc, imp.ID, rows); err != nil {
		return fmt.Errorf("seed demo icmal rows: %w", err)
	}
	stable := func(v string) map[string]any { return map[string]any{"Value": v, "Samples": len(rows), "Stable": true} }
	result, err := json.Marshal(map[string]any{
		"analyses": []map[string]any{{
			"EtsoCode": etso, "Periods": periods, "EnergyKbk": stable("1.0000"), "DistributionTlPerKwh": stable("1.0834"),
			"PowerUnitPrice": stable("32.1000"), "ReactiveUnitPrice": stable("3.4937"), "VatRate": stable("20"),
			"Taxes": map[string]any{"BTV": stable("1")}, "WithinTolerance": true, "Warnings": []any{},
		}},
		"unmatched": []string{}, "warnings": []any{}, "buildings": map[string]string{etso: factory.String()},
	})
	if err != nil {
		return err
	}
	_, err = repo.UpdateImportResult(ctx, sc, imp.ID, "analysed", result)
	return err
}
