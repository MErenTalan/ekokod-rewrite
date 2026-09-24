package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// E2ECarbonSelection is building A1's declared sub-categories (F10a).
var E2ECarbonSelection = []string{"sub_business_travel", "sub_elec_generation", "sub_grid_electricity",
	"sub_purchased_goods", "sub_space_heating", "sub_waste_disposal"}

// ensureCarbon gives the carbon screens a declaration and four manual records
// (pending, approved, rejected, one across two months), computed from the
// platform catalogue like the service would. Automated rows come from the
// worker's carbon.daily_accrual. Idempotent.
func ensureCarbon(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	repo := postgres.NewCarbonRepository(pool)
	if err := repo.ReplaceSelectedActivities(ctx, sc, f.BuildingA1, E2ECarbonSelection); err != nil {
		return fmt.Errorf("seed carbon selection: %w", err)
	}
	manual := false
	existing, err := repo.ListActivities(ctx, sc, store.CarbonActivityFilter{BuildingIDs: []uuid.UUID{f.BuildingA1}, IsAutomated: &manual})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	factors, err := repo.ListFactors(ctx, sc, store.EmissionFactorFilter{IncludePlatform: true, Page: store.Page{Limit: 1000}})
	if err != nil {
		return err
	}
	// A bare fixture database may not have the catalogue yet (see ensureSolar).
	if len(factors) == 0 {
		if _, _, err := loadEmissionFactors(ctx, admin.NewCatalogueRepository(pool), discardLog); err != nil {
			return err
		}
		if factors, err = repo.ListFactors(ctx, sc, store.EmissionFactorFilter{IncludePlatform: true, Page: store.Page{Limit: 1000}}); err != nil {
			return err
		}
	}
	factorFor := func(sub string) (model.EmissionFactor, error) {
		for _, fa := range factors {
			for _, s := range fa.SubCategories {
				if s == sub {
					return fa, nil
				}
			}
		}
		return model.EmissionFactor{}, fmt.Errorf("seed carbon: no platform factor for %s", sub)
	}
	ist, _ := time.LoadLocation("Europe/Istanbul")
	y, m, _ := now.In(ist).Date()
	month := func(back int) time.Time { return time.Date(y, m-time.Month(back), 1, 0, 0, 0, 0, time.UTC) }
	for _, r := range []struct {
		sub        string
		quantity   string
		start, end time.Time
		status     model.CarbonStatus
		note       string
	}{
		{"sub_space_heating", "1200", month(2), month(1).AddDate(0, 0, -1), model.CarbonStatusApproved, "Kazan dairesi"},
		{"sub_business_travel", "850", month(1), month(1).AddDate(0, 0, 9), model.CarbonStatusPending, "Ankara toplantısı"},
		{"sub_waste_disposal", "2.5", month(1), month(0).AddDate(0, 0, -1), model.CarbonStatusRejected, "Hatalı giriş"},
		{"sub_purchased_goods", "40", month(2).AddDate(0, 0, 14), month(1).AddDate(0, 0, 13), model.CarbonStatusApproved, "Kırtasiye"},
	} {
		fa, err := factorFor(r.sub)
		if err != nil {
			return err
		}
		sub, _ := domain.SubByKey(r.sub)
		q := decimal.RequireFromString(r.quantity)
		one := decimal.NewFromInt(1)
		id, key, value, note := fa.ID, fa.Key, fa.BaseFactor, r.note
		a, err := repo.CreateActivity(ctx, sc, model.CarbonActivity{CompanyID: f.CompanyA, BuildingID: f.BuildingA1,
			MainCategory: sub.Main, SubCategory: sub.Key, ActivityType: sub.Key, PeriodStart: r.start, PeriodEnd: r.end,
			Quantity: q, Unit: fa.BaseUnit, FactorID: &id, FactorKey: &key, FactorValue: &value, ConversionMultiplier: one,
			EmissionKgco2e: domain.Emission(q, one, fa.BaseFactor), Scope: sub.Scope, IsoCategory: sub.ISO,
			Description: &note, Details: []byte(`{}`), Status: model.CarbonStatusPending})
		if err != nil {
			return fmt.Errorf("seed carbon activity %s: %w", r.sub, err)
		}
		if r.status != model.CarbonStatusPending {
			if _, err := repo.SetActivityStatus(ctx, sc, a.ID, r.status, now); err != nil {
				return err
			}
		}
	}
	return nil
}
