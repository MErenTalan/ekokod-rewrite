// Package analysis serves the consumption, generation, load-profile and
// reactive-status reads over F3's services (05 §5, R160, R165, R166).
package analysis

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Errors.
var (
	ErrInvalidParameters       = perr.New("invalid_parameters", 400, "errors.api.invalidParameters")
	ErrBillingParametersAbsent = perr.New("billing_parameters_missing", 500, "errors.billing.parametersMissing")
)

// Series is the consumption read path.
type Series interface {
	Consumption(ctx context.Context, sc store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error)
}

// Anomalies is the anomaly slice of consumption.Billing.
type Anomalies interface {
	ListAnomalies(ctx context.Context, sc store.Scope, req consumption.AnomalyListRequest) ([]model.ConsumptionAnomaly, error)
	ResolveAnomaly(ctx context.Context, sc store.Scope, id, resolvedBy uuid.UUID, r consumption.Resolution) (model.ConsumptionAnomaly, error)
}

// Profiles is the load-profile service.
type Profiles interface {
	Profiles(ctx context.Context, sc store.Scope, req loadprofile.Request) (loadprofile.Result, error)
	// CalendarConfig is the company's weekend days and vacations (R137), shared
	// with the grouped consumption read so the two never classify a day differently.
	CalendarConfig(ctx context.Context, sc store.Scope, r store.TimeRange) (domainlp.Config, error)
}

// Deps is everything Service needs.
type Deps struct {
	Series    Series
	Anomalies Anomalies
	Profiles  Profiles
	Analyzers store.AnalyzerRepository
	Buildings store.BuildingRepository
	Params    store.BillingParameterRepository
	Tariffs   store.TariffRepository
	Clock     clock.Clock
	Location  *time.Location
}

// Service implements the analysis reads.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Series == nil || d.Anomalies == nil || d.Profiles == nil || d.Analyzers == nil || d.Buildings == nil ||
		d.Params == nil || d.Tariffs == nil || d.Clock == nil || d.Location == nil {
		return nil, errors.New("analysis: missing dependency")
	}
	return &Service{d: d}, nil
}

// mapErr turns F3's fail-closed validation sentinels into a 400.
func mapErr(err error) error {
	if errors.Is(err, consumption.ErrInvalidRequest) || errors.Is(err, loadprofile.ErrInvalidRequest) {
		return ErrInvalidParameters
	}
	return err
}

// Subject names what a read is about: exactly one analyzer or one building (R160).
type Subject struct {
	AnalyzerID, BuildingID *uuid.UUID
}

// analyzers resolves a subject into analyzer ids, checking visibility.
func (s *Service) analyzers(ctx context.Context, sc store.Scope, subj Subject) ([]uuid.UUID, bool, error) {
	if (subj.AnalyzerID == nil) == (subj.BuildingID == nil) {
		return nil, false, ErrInvalidParameters.WithParams(map[string]any{"analyzer_id": []string{"exactly_one_of_analyzer_id_building_id"}})
	}
	if subj.AnalyzerID != nil {
		a, err := s.d.Analyzers.Get(ctx, sc, *subj.AnalyzerID)
		if err != nil {
			return nil, false, err
		}
		return []uuid.UUID{a.ID}, false, nil
	}
	if _, err := s.d.Buildings.Get(ctx, sc, *subj.BuildingID); err != nil {
		return nil, true, err
	}
	list, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: subj.BuildingID, Page: store.Page{Limit: consumption.MaxAnalyzersPerRequest + 1}})
	if err != nil {
		return nil, true, err
	}
	if len(list) > consumption.MaxAnalyzersPerRequest {
		return nil, true, ErrInvalidParameters.WithParams(map[string]any{"building_id": []string{"too_many_analyzers"}})
	}
	ids := make([]uuid.UUID, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	return ids, true, nil
}
