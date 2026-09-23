package carbon

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Amount is one keyed emission figure.
type Amount struct {
	Key    string
	KgCO2e decimal.Decimal
}

// MonthPair is one month of the year and the year before.
type MonthPair struct{ Current, Previous decimal.Decimal }

// Overview is R310's read model.
type Overview struct {
	Year                                         int
	Total                                        decimal.Decimal
	ActivityCount, RegisteredCount, PendingCount int
	Highest                                      *domain.SubTotal
	ByCategory, ByScope                          []Amount
	Monthly                                      [12]MonthPair
	Recent                                       []model.CarbonActivity
}

const recentCount = 5

// Overview aggregates a building's year (R309, R310).
func (s *Service) Overview(ctx context.Context, sc store.Scope, buildingID uuid.UUID, year int) (Overview, error) {
	selected, err := s.Selected(ctx, sc, buildingID) // also the visibility check
	if err != nil {
		return Overview{}, err
	}
	from, to := time.Date(year-1, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	acts, err := s.allActivities(ctx, sc, buildingID, from, to, nil)
	if err != nil {
		return Overview{}, err
	}
	records := make([]domain.Record, len(acts))
	for i, a := range acts {
		records[i] = record(a)
	}
	cur, prev := domain.BuildYear(records, year), domain.BuildYear(records, year-1)
	o := Overview{Year: year, Total: cur.Total, ActivityCount: cur.Count, RegisteredCount: len(selected),
		PendingCount: cur.Pending, Highest: cur.Highest}
	for _, m := range domain.Mains() {
		o.ByCategory = append(o.ByCategory, Amount{Key: m, KgCO2e: cur.ByMain[m]})
	}
	for _, sc := range model.CarbonScopes() {
		o.ByScope = append(o.ByScope, Amount{Key: string(sc), KgCO2e: cur.ByScope[sc]})
	}
	for m := range o.Monthly {
		o.Monthly[m] = MonthPair{Current: cur.Monthly[m], Previous: prev.Monthly[m]}
	}
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].CreatedAt.After(acts[j].CreatedAt) })
	o.Recent = acts[:min(recentCount, len(acts))]
	return o, nil
}
