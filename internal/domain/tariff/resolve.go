package tariff

import (
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Resolve applies store.TariffRepository.Effective's precedence (02 §4, I-15):
// building rows first, company-wide rows only when no building row qualifies;
// within a tier the greatest EffectiveFrom whose Istanbul date is not after on's,
// ties to the latest CreatedAt. Deleted rows are ignored.
func Resolve(versions []model.Tariff, buildingID uuid.UUID, on time.Time, loc *time.Location) (model.Tariff, bool) {
	var building, company []model.Tariff
	for _, v := range versions {
		switch {
		case v.DeletedAt != nil:
		case v.BuildingID == nil:
			company = append(company, v)
		case *v.BuildingID == buildingID:
			building = append(building, v)
		}
	}
	if t, ok := latest(building, on, loc, func(t model.Tariff) (time.Time, time.Time) { return t.EffectiveFrom, t.CreatedAt }); ok {
		return t, true
	}
	return latest(company, on, loc, func(t model.Tariff) (time.Time, time.Time) { return t.EffectiveFrom, t.CreatedAt })
}

// ResolveParams picks the parameter set in force on on (R106).
func ResolveParams(sets []model.BillingParameters, on time.Time, loc *time.Location) (model.BillingParameters, bool) {
	return latest(sets, on, loc, func(p model.BillingParameters) (time.Time, time.Time) { return p.EffectiveFrom, p.CreatedAt })
}

func latest[T any](items []T, on time.Time, loc *time.Location, keys func(T) (effective, created time.Time)) (T, bool) {
	target := dayKey(on, loc)
	var best T
	found := false
	var bestDay int
	var bestCreated time.Time
	for _, it := range items {
		eff, created := keys(it)
		day := dayKey(eff, loc)
		if day > target {
			continue
		}
		if !found || day > bestDay || (day == bestDay && created.After(bestCreated)) {
			best, bestDay, bestCreated, found = it, day, created, true
		}
	}
	return best, found
}

// dayKey is ts's calendar date in loc as yyyymmdd. A `date` column arrives as
// UTC midnight, which lands on the same date in Istanbul (UTC+3).
func dayKey(ts time.Time, loc *time.Location) int {
	y, m, d := ts.In(loc).Date()
	return y*10000 + int(m)*100 + d
}
