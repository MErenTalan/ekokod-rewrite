package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	_ "time/tzdata" // Europe/Istanbul without host tzdata

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ConsumptionReader is the invoice-grade consumption the service needs;
// *consumption.Billing satisfies it (R61: never analytics).
type ConsumptionReader interface {
	PeriodConsumptionAndRecord(ctx context.Context, sc store.Scope, req consumption.PeriodRequest) ([]consumption.Row, error)
	Consumption(ctx context.Context, sc store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error)
}

// Deps are the service's collaborators.
type Deps struct {
	Consumption ConsumptionReader
	Buildings   store.BuildingRepository
	Analyzers   store.AnalyzerRepository
	Tariffs     store.TariffRepository
	Params      store.BillingParameterRepository
	Prices      store.PriceRepository
	Anomalies   store.AnomalyRepository
	Bills       store.BillRepository
	Ops         store.OpsRepository
	Clock       clock.Clock
	Log         *slog.Logger
}

// Service generates and reads bills.
type Service struct {
	deps Deps
	loc  *time.Location
}

// New validates d.
func New(d Deps) (*Service, error) {
	for name, missing := range map[string]bool{
		"Consumption": d.Consumption == nil, "Buildings": d.Buildings == nil, "Analyzers": d.Analyzers == nil,
		"Tariffs": d.Tariffs == nil, "Params": d.Params == nil, "Prices": d.Prices == nil, "Anomalies": d.Anomalies == nil,
		"Bills": d.Bills == nil, "Ops": d.Ops == nil, "Clock": d.Clock == nil, "Log": d.Log == nil,
	} {
		if missing {
			return nil, fmt.Errorf("billing: Deps.%s is required", name)
		}
	}
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return nil, errors.Join(errors.New("billing: load Europe/Istanbul"), err)
	}
	return &Service{deps: d, loc: loc}, nil
}
