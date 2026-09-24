// Package report loads what the monthly and yearly reports need under a
// scope, builds them with internal/domain/report, and owns their
// generation, files and delivery (01 §7.14, R254–R275).
package report

import (
	"errors"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Deps is every read the reports make.
type Deps struct {
	Buildings   store.BuildingRepository
	Analyzers   store.AnalyzerRepository
	Bills       store.BillRepository
	Plants      store.PlantRepository
	Solar       store.SolarTariffRepository
	Tariffs     store.TariffRepository
	Carbon      store.CarbonRepository
	Analytics   store.AnalyticsRepository
	Consumption *consumption.Analytics
	Clock       clock.Clock
}

// Service builds reports.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Buildings == nil || d.Analyzers == nil || d.Bills == nil || d.Plants == nil || d.Solar == nil ||
		d.Tariffs == nil || d.Carbon == nil || d.Analytics == nil || d.Consumption == nil || d.Clock == nil {
		return nil, errors.New("report: every dependency is required")
	}
	return &Service{d: d}, nil
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()
