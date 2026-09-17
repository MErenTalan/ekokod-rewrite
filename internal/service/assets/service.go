// Package assets manages buildings, analyzers and power plants (05 §4,
// R163–R175) and serves the sectoral comparison (R162).
package assets

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Errors specific to assets.
var (
	ErrBuildingHasAnalyzers     = perr.New("building_has_analyzers", 409, "errors.buildings.hasAnalyzers")
	ErrIntegrationNotConfigured = perr.New("integration_not_configured", 409, "errors.analyzers.integrationNotConfigured")
	ErrRefreshInProgress        = perr.New("refresh_in_progress", 409, "errors.analyzers.refreshInProgress")
)

// ActiveWindow is 02 §3.6: a metering point is active when it reported within it.
const ActiveWindow = 7 * 24 * time.Hour

// Enqueuer is the job client.
type Enqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Deps is everything Service needs.
type Deps struct {
	Buildings    store.BuildingRepository
	Analyzers    store.AnalyzerRepository
	Plants       store.PlantRepository
	Tariffs      store.TariffRepository
	Integrations store.IntegrationRepository
	Sector       store.AdminSectorRepository
	Carbon       store.CarbonRepository
	Enqueuer     Enqueuer
	MaxRetry     int
	Clock        clock.Clock
}

// Service implements asset management.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Buildings == nil || d.Analyzers == nil || d.Plants == nil || d.Tariffs == nil || d.Integrations == nil || d.Sector == nil || d.Carbon == nil ||
		d.Enqueuer == nil || d.Clock == nil {
		return nil, errors.New("assets: missing dependency")
	}
	return &Service{d: d}, nil
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

// listAll pages through a repository list.
func listAll[T any](list func(store.Page) ([]T, error)) ([]T, error) {
	const size = 500
	var out []T
	for offset := int32(0); ; offset += size {
		page, err := list(store.Page{Limit: size, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < size {
			return out, nil
		}
	}
}

func keepOrClear(current, in *string) *string {
	switch {
	case in == nil:
		return current
	case *in == "":
		return nil
	default:
		v := *in
		return &v
	}
}
