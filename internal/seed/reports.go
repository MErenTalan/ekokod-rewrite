package seed

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	reportsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// ensureReports gives the reports screen real figures (F8b): a utility-scale
// plant with a yearly target and a sample of production every month from the
// start of last year, and one completed monthly report for A1's previous
// month, built by the report service itself. Idempotent.
func ensureReports(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	if err := ensureGridPlant(ctx, pool, sc, now); err != nil {
		return err
	}
	local := now.In(istanbul)
	period := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0).Format("2006-01")
	reports := postgres.NewReportRepository(pool)
	kind := model.ReportTypeMonthly
	existing, err := reports.List(ctx, sc, store.ReportFilter{BuildingID: &f.BuildingA1, Type: &kind, Period: &period, Page: store.Page{Limit: 1}})
	if err != nil || len(existing) > 0 {
		return err
	}
	svc, err := reportService(pool, now)
	if err != nil {
		return err
	}
	payload, err := svc.Build(ctx, sc, reportsvc.Request{Type: domain.TypeMonthly, Period: period, Selection: domain.SelectionAll,
		BuildingIDs: []uuid.UUID{f.BuildingA1}})
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	building, err := postgres.NewBuildingRepository(pool).Get(ctx, sc, f.BuildingA1)
	if err != nil {
		return err
	}
	subject, body, err := reportsvc.EmailContent(payload, []string{building.Name}, "tr")
	if err != nil {
		return err
	}
	processed := now.UTC()
	_, err = reports.Upsert(ctx, sc, model.Report{CompanyID: f.CompanyA, BuildingID: f.BuildingA1, Type: kind, Period: period,
		PlantSelection: model.PlantSelectionAll, Payload: raw, Status: model.ReportStatusCompleted,
		EmailSubject: &subject, EmailBody: &body, ProcessedAt: &processed, CreatedAt: processed})
	return err
}

var discardLog = slog.New(slog.DiscardHandler)

func reportService(pool *pgxpool.Pool, now time.Time) (*reportsvc.Service, error) {
	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: postgres.NewAnalyticsRepository(pool), Log: discardLog})
	if err != nil {
		return nil, err
	}
	return reportsvc.New(reportsvc.Deps{
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Bills: postgres.NewBillRepository(pool), Plants: postgres.NewPlantRepository(pool),
		Solar: postgres.NewSolarTariffRepository(pool), Tariffs: postgres.NewTariffRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Analytics: postgres.NewAnalyticsRepository(pool),
		Consumption: analytics, Clock: clock.NewFake(now),
	})
}

// ensureGridPlant writes the plant, its inverter and a monthly production
// sample from January of last year to last month, then materialises the view.
func ensureGridPlant(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, now time.Time) error {
	plants := postgres.NewPlantRepository(pool)
	kind := "grid"
	list, err := plants.List(ctx, sc, store.PlantFilter{PlantKind: &kind, Page: store.Page{Limit: 1}})
	if err != nil {
		return err
	}
	var plant model.PowerPlant
	if len(list) > 0 {
		plant = list[0]
	} else {
		capacity, target := decimal.RequireFromString("100.00"), decimal.RequireFromString("120000")
		if plant, err = plants.Create(ctx, sc, model.PowerPlant{CompanyID: sc.CompanyID, Name: "E2E Arazi GES", PlantKind: kind,
			TotalCapacityKw: &capacity, YearlyTargetKwh: &target, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
	}
	device, err := plants.UpsertDevice(ctx, sc, model.PlantDevice{PlantID: plant.ID, DeviceSN: "E2E-INV-1"})
	if err != nil {
		return err
	}
	local := now.In(istanbul)
	var rows []model.PlantProduction
	for m := time.Date(local.Year()-1, 1, 15, 12, 0, 0, 0, istanbul); m.Before(time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul)); m = m.AddDate(0, 1, 0) {
		v := decimal.RequireFromString("9000")
		rows = append(rows, model.PlantProduction{PlantID: plant.ID, DeviceID: device.ID, Ts: m.UTC(), ProductionKwh: &v, Source: "isolar"})
	}
	if _, _, err := postgres.NewProductionRepository(pool).BulkInsert(ctx, sc, rows); err != nil {
		return err
	}
	for _, view := range []string{"plant_production_daily", "plant_production_monthly"} {
		if _, err := pool.Exec(ctx, "call refresh_continuous_aggregate('"+view+"', NULL, NULL)"); err != nil {
			return errors.Join(errors.New("seed: refresh "+view), err)
		}
	}
	return nil
}
