package carbon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const maxAccrualDaysBack = 400

// AccrualResult counts what one accrual wrote.
type AccrualResult struct {
	Companies, Buildings, GridRows, GenerationRows int
	Skipped                                        map[string]int
}

// AccrueTask is the job entry: an empty day means yesterday (Istanbul).
func (s *Service) AccrueTask(ctx context.Context, day string) error {
	d := s.today().AddDate(0, 0, -1)
	if day != "" {
		parsed, err := time.Parse(time.DateOnly, day)
		if err != nil {
			return fmt.Errorf("carbon.daily_accrual: day %q: %w", day, err)
		}
		d = parsed
	}
	_, err := s.Accrue(ctx, d)
	return err
}

// Accrue is R311: one grid row and at most one generation row per building
// for the day, converging on re-runs. One company's failure never stops another's.
func (s *Service) Accrue(ctx context.Context, day time.Time) (AccrualResult, error) {
	day = civil(day)
	today := s.today()
	if !day.Before(today) || day.Before(today.AddDate(0, 0, -maxAccrualDaysBack)) {
		return AccrualResult{}, validation("day", "range")
	}
	res := AccrualResult{Skipped: map[string]int{}}
	for offset := int32(0); ; offset += pageSize {
		companies, err := s.d.Tenants.ListCompanies(ctx, store.CompanyFilter{Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return res, err
		}
		for _, c := range companies {
			res.Companies++
			s.accrueCompany(ctx, c.ID, day, &res)
		}
		if len(companies) < pageSize {
			return res, nil
		}
	}
}

func (s *Service) accrueCompany(ctx context.Context, companyID uuid.UUID, day time.Time, res *AccrualResult) {
	sc := store.SystemScope(companyID)
	scope, _ := json.Marshal(map[string]any{"day": day.Format(time.DateOnly)})
	run, runErr := s.d.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &companyID, JobType: job.TypeCarbonAccrual, Scope: scope,
		StartedAt: s.d.Clock.Now(), Status: "running"})
	var processed, failed int32
	grid, err := s.GridFactor(ctx, sc)
	if err != nil {
		failed++
	}
	if grid == nil && err == nil {
		s.message(ctx, sc, "error", "Şebeke emisyon faktörü tanımlı değil; günlük karbon kaydı oluşturulamadı.")
	}
	buildings, err := s.d.Buildings.List(ctx, sc, store.BuildingFilter{Page: store.Page{Limit: pageSize}})
	if err != nil {
		failed++
	}
	for _, b := range buildings {
		res.Buildings++
		if err := s.accrueBuilding(ctx, sc, b.ID, day, grid, res); err != nil {
			failed++
			continue
		}
		processed++
	}
	if runErr == nil {
		status := "success"
		if failed > 0 {
			status = "partial"
		}
		_, _ = s.d.Ops.FinishRun(ctx, sc, run.ID, status, processed, 0, failed, nil, nil, s.d.Clock.Now())
	}
}

func (s *Service) message(ctx context.Context, sc store.Scope, status, text string) {
	companyID := sc.CompanyID
	_, _ = s.d.Ops.AppendMessage(ctx, sc, model.OperationalMessage{CompanyID: &companyID, Kind: "job", Category: "carbon-accrual",
		Status: status, Message: text, CreatedAt: s.d.Clock.Now()})
}

func (s *Service) accrueBuilding(ctx context.Context, sc store.Scope, buildingID uuid.UUID, day time.Time, grid *FactorView, res *AccrualResult) error {
	analyzers, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: &buildingID, Page: store.Page{Limit: pageSize}})
	if err != nil || len(analyzers) == 0 {
		return err
	}
	ids := make([]uuid.UUID, len(analyzers))
	for i, a := range analyzers {
		ids[i] = a.ID
	}
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, istanbul)
	buckets, err := s.d.Analytics.ConsumptionDaily(ctx, sc, ids, store.TimeRange{From: from, To: from.AddDate(0, 0, 1)})
	if err != nil {
		return err
	}
	var consumption, generation decimal.Decimal
	measured := false
	for _, b := range buckets {
		if b.ActiveConsumption != nil {
			consumption, measured = consumption.Add(*b.ActiveConsumption), true
		}
		if b.ActiveGeneration != nil {
			generation = generation.Add(*b.ActiveGeneration)
		}
	}
	if measured {
		if grid == nil {
			res.Skipped["grid_factor_missing"]++
		} else {
			if _, err := s.upsertAutomated(ctx, sc, buildingID, day, domain.SubGrid, consumption, grid,
				domain.Emission(consumption, decimal.NewFromInt(1), grid.BaseFactor), "Günlük şebeke tüketimi (otomatik)", nil); err != nil {
				return err
			}
			res.GridRows++
		}
	}
	if generation.IsPositive() {
		extra := map[string]string{"source": "meter_export"}
		if grid != nil {
			extra["avoided_kgco2e"] = domain.Emission(generation, decimal.NewFromInt(1), grid.BaseFactor).String()
		}
		// Q-F3: rooftop export is renewable, so it carries no emission itself.
		if _, err := s.upsertAutomated(ctx, sc, buildingID, day, domain.SubGeneration, generation, grid, decimal.Zero,
			"Günlük üretim, sayaç ihracatı (otomatik)", extra); err != nil {
			return err
		}
		res.GenerationRows++
	}
	return nil
}

func (s *Service) upsertAutomated(ctx context.Context, sc store.Scope, buildingID uuid.UUID, day time.Time, subKey string,
	kwh decimal.Decimal, f *FactorView, emission decimal.Decimal, desc string, extra map[string]string) (model.CarbonActivity, error) {
	sub, _ := domain.SubByKey(subKey)
	details := map[string]string{}
	for k, v := range extra {
		details[k] = v
	}
	a := model.CarbonActivity{CompanyID: sc.CompanyID, BuildingID: buildingID, MainCategory: sub.Main, SubCategory: sub.Key,
		ActivityType: sub.Key, PeriodStart: day, PeriodEnd: day, Quantity: kwh.Round(6), Unit: "kWh",
		ConversionMultiplier: decimal.NewFromInt(1), EmissionKgco2e: emission, Scope: sub.Scope, IsoCategory: sub.ISO,
		Description: &desc, Status: model.CarbonStatusApproved, IsAutomated: true}
	if f != nil {
		id, key, value := f.ID, f.Key, f.BaseFactor
		a.FactorID, a.FactorKey, a.FactorValue = &id, &key, &value
		if f.Source != nil {
			details[snapSource] = *f.Source
		}
		if f.SourceYear != nil {
			details[snapSourceYr] = fmt.Sprint(*f.SourceYear)
		}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return model.CarbonActivity{}, err
	}
	a.Details = raw
	return s.d.Carbon.UpsertAutomatedActivity(ctx, sc, a)
}
