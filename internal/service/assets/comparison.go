package assets

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/comparison"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrBuildingSectorMissing is returned when a building has no sector to compare within.
var ErrBuildingSectorMissing = perr.New("building_sector_missing", 409, "errors.buildings.sectorMissing")

// GridFactorKey is the emission factor the CO2 figure uses (R162).
const GridFactorKey = "grid_electricity_tr_2022"

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Comparison is the sectoral comparison of one building.
type Comparison struct {
	Sector string
	Result comparison.Result
}

// Compare returns the sectoral comparison (02 §10.4, R162): trailing 30 full
// Istanbul days for the daily mean, the previous calendar month for the rest.
func (s *Service) Compare(ctx context.Context, sc store.Scope, id uuid.UUID) (Comparison, error) {
	b, err := s.d.Buildings.Get(ctx, sc, id)
	if err != nil {
		return Comparison{}, err
	}
	if b.Sector == nil || strings.TrimSpace(*b.Sector) == "" {
		return Comparison{}, ErrBuildingSectorMissing
	}
	now := s.d.Clock.Now().In(istanbul)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, istanbul)
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, istanbul)
	daily := store.TimeRange{From: today.AddDate(0, 0, -30), To: today}
	month := store.TimeRange{From: thisMonth.AddDate(0, -1, 0), To: thisMonth}
	rows, err := s.d.Sector.SectorFigures(ctx, *b.Sector, daily, month)
	if err != nil {
		return Comparison{}, err
	}
	peers := make([]comparison.Figures, len(rows))
	for i, r := range rows {
		f := comparison.Figures{BuildingID: r.BuildingID, Monthly: r.MonthlySum, Personnel: r.PersonnelCount, AreaM2: r.TotalAreaM2}
		if r.DailySum != nil && r.DaysWithData > 0 {
			mean := r.DailySum.DivRound(decimal.NewFromInt32(r.DaysWithData), 6)
			f.Daily = &mean
		}
		peers[i] = f
	}
	factor, err := s.gridFactor(ctx, sc)
	if err != nil {
		return Comparison{}, err
	}
	return Comparison{Sector: strings.TrimSpace(*b.Sector), Result: comparison.Compare(id, peers, factor)}, nil
}

// gridFactor prefers the company's own override of the grid factor.
func (s *Service) gridFactor(ctx context.Context, sc store.Scope) (*decimal.Decimal, error) {
	factors, err := s.d.Carbon.ListFactors(ctx, sc, store.EmissionFactorFilter{Keys: []string{GridFactorKey}, IncludePlatform: true})
	if err != nil {
		return nil, err
	}
	var platform *decimal.Decimal
	for _, f := range factors {
		v := f.BaseFactor
		if f.CompanyID != nil {
			return &v, nil
		}
		platform = &v
	}
	return platform, nil
}
