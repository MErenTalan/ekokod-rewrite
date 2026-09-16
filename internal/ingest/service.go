package ingest

import (
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
)

// Service implements job.Ingestion: the platform dispatch tick
// (integration.sync_dispatch), per-credential analyzer discovery
// (integration.sync_analyzers) and per-analyzer reading fetches
// (integration.fetch_readings). It is the only code in this codebase that
// writes to meter_readings (06 §1 rule 2: adapters are pure clients).
type Service struct {
	deps Deps
	opts Options
}

var _ job.Ingestion = (*Service)(nil)

// New builds a Service. A nil required dependency returns an error naming
// the field; Hooks and ConsumptionRefresh are the only fields allowed to be
// nil (see Deps' doc). Zero-valued Options fields are filled with their
// documented defaults except MaxRetry and ConsumptionRefreshEnabled, which
// are never defaulted (see Options' doc).
func New(d Deps, o Options) (*Service, error) {
	type namedDep struct {
		name string
		nilv bool
	}
	required := []namedDep{
		{"Analyzers", d.Analyzers == nil},
		{"Readings", d.Readings == nil},
		{"Cursors", d.Cursors == nil},
		{"Anomalies", d.Anomalies == nil},
		{"Ops", d.Ops == nil},
		{"ProviderSeries", d.ProviderSeries == nil},
		{"AdminIngestion", d.AdminIngestion == nil},
		{"AdminJournal", d.AdminJournal == nil},
		{"Credentials", d.Credentials == nil},
		{"Sources", d.Sources == nil},
		{"Enqueuer", d.Enqueuer == nil},
		{"Clock", d.Clock == nil},
		{"Log", d.Log == nil},
	}
	for _, dep := range required {
		if dep.nilv {
			return nil, fmt.Errorf("ingest: Deps.%s is required", dep.name)
		}
	}

	if o.FutureTolerance <= 0 {
		o.FutureTolerance = DefaultFutureTolerance
	}
	if o.SanityMultiple.IsZero() {
		o.SanityMultiple = DefaultSanityMultiple()
	}
	if o.InitialLookback <= 0 {
		o.InitialLookback = DefaultInitialLookback
	}
	if o.MaxPagesPerRun <= 0 {
		o.MaxPagesPerRun = DefaultMaxPagesPerRun
	}

	return &Service{deps: d, opts: o}, nil
}
