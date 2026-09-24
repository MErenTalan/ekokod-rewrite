package analysis

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// SeriesInput is one consumption query: dates are inclusive Istanbul days (R161).
type SeriesInput struct {
	Subject
	Level    energy.Level
	From, To time.Time // Istanbul midnights; To is the last day included
}

// Rows returns the consumption rows for the subject; a building's rows are summed (R160).
func (s *Service) Rows(ctx context.Context, sc store.Scope, in SeriesInput) ([]consumption.Row, bool, error) {
	ids, building, err := s.analyzers(ctx, sc, in.Subject)
	if err != nil {
		return nil, building, err
	}
	if len(ids) == 0 {
		return []consumption.Row{}, building, nil
	}
	if !in.To.After(in.From) && !in.To.Equal(in.From) {
		return nil, building, ErrInvalidParameters.WithParams(map[string]any{"to": []string{"gtefield"}})
	}
	rng := store.TimeRange{From: in.From, To: in.To.AddDate(0, 0, 1)}
	rows, err := s.d.Series.Consumption(ctx, sc, consumption.SeriesRequest{AnalyzerIDs: ids, Level: in.Level, Range: rng})
	if err != nil {
		return nil, building, mapErr(err)
	}
	if building {
		return SumByWindow(rows, len(ids)), true, nil
	}
	return rows, false, nil
}

// AnomalyInput narrows the anomaly list; with no subject every analyzer in scope is used.
type AnomalyInput struct {
	Subject
	Unresolved bool
	Page       store.Page
}

// Anomalies lists suspect periods.
func (s *Service) Anomalies(ctx context.Context, sc store.Scope, in AnomalyInput) ([]model.ConsumptionAnomaly, error) {
	var ids []uuid.UUID
	if in.AnalyzerID == nil && in.BuildingID == nil {
		list, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{Page: store.Page{Limit: 500}})
		if err != nil {
			return nil, err
		}
		for _, a := range list {
			ids = append(ids, a.ID)
		}
	} else {
		var err error
		if ids, _, err = s.analyzers(ctx, sc, in.Subject); err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return []model.ConsumptionAnomaly{}, nil
	}
	list, err := s.d.Anomalies.ListAnomalies(ctx, sc, consumption.AnomalyListRequest{AnalyzerIDs: ids, Unresolved: in.Unresolved, Page: in.Page})
	return list, mapErr(err)
}

// ResolveAnomaly records an operator's resolution.
func (s *Service) ResolveAnomaly(ctx context.Context, sc store.Scope, id, by uuid.UUID, r consumption.Resolution) (model.ConsumptionAnomaly, error) {
	a, err := s.d.Anomalies.ResolveAnomaly(ctx, sc, id, by, r)
	return a, mapErr(err)
}

// ProfileInput is one load-profile query.
type ProfileInput struct {
	AnalyzerID uuid.UUID
	From, To   time.Time // inclusive Istanbul days
	Keys       []domainlp.Key
}

// Profiles returns the averaged profiles and their statistics.
func (s *Service) Profiles(ctx context.Context, sc store.Scope, in ProfileInput) (loadprofile.Result, error) {
	if _, err := s.d.Analyzers.Get(ctx, sc, in.AnalyzerID); err != nil {
		return loadprofile.Result{}, err
	}
	res, err := s.d.Profiles.Profiles(ctx, sc, loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{in.AnalyzerID}, Range: store.TimeRange{From: in.From, To: in.To.AddDate(0, 0, 1)}, Keys: in.Keys,
	})
	return res, mapErr(err)
}

// ProfileKeys is every profile key in display order.
func ProfileKeys() []domainlp.Key {
	return []domainlp.Key{"weekday", "weekend", "winter_weekday", "winter_weekend", "spring_weekday", "spring_weekend",
		"summer_weekday", "summer_weekend", "autumn_weekday", "autumn_weekend"}
}

// Analyzer returns one visible analyzer (used by the anomaly check to 404 early).
func (s *Service) Analyzer(ctx context.Context, sc store.Scope, id uuid.UUID) (model.Analyzer, error) {
	return s.d.Analyzers.Get(ctx, sc, id)
}
