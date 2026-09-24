// Package carbon is Eko-CM's service (01 §7.16, 05 §12): the effective factor
// catalogue, building selections, activities with server-side emission, the
// overview, the daily accrual and the GHG/ISO reports.
package carbon

import (
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Deps is what the service reads and writes. Auditing is the routes'
// middleware (Route.Entity), not the service's.
type Deps struct {
	Carbon    store.CarbonRepository
	Buildings store.BuildingRepository
	Analyzers store.AnalyzerRepository
	Analytics store.AnalyticsRepository
	Companies store.CompanyRepository
	Ops       store.OpsRepository
	Tenants   store.AdminTenantRepository
	Clock     clock.Clock
}

// Service is the carbon module.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System()
	}
	return &Service{d: d}
}

func validation(field, code string) error {
	return perr.Validation.WithParams(map[string]any{field: []string{code}})
}

// pageSize bounds each store read; the service pages through.
const pageSize = 1000
