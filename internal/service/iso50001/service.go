// Package iso50001 is the ISO 50001 workbench's service (01 §7.17, 05 §13):
// project state, clause dates, notes, evidence files, templates and export.
package iso50001

import (
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// OwnerType tags this module's stored_files rows; owner_id is the building.
const OwnerType = "iso50001"

// Deps is what the service uses. Auditing is the routes' middleware.
type Deps struct {
	ISO       store.ISO50001Repository
	Files     store.FileRepository
	Buildings store.BuildingRepository
	Store     storage.Store
	Clock     clock.Clock
}

// Service is the ISO 50001 module.
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

var istanbul = func() *time.Location {
	l, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return l
}()

const pageSize = 1000
