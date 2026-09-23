package solar

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Adapter is the iSolar client surface the service uses (*isolar.Client).
type Adapter interface {
	Plants(ctx context.Context, creds integration.Credentials) ([]isolar.Plant, error)
	Devices(ctx context.Context, creds integration.Credentials, psID string) ([]isolar.Device, error)
	DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]isolar.YieldSample, error)
	PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]isolar.YieldSample, error)
	PlantDailySeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]isolar.DailyYield, error)
	DeviceRealtime(ctx context.Context, creds integration.Credentials, deviceType int32, psKeys []string) ([]isolar.DeviceSnapshot, error)
	Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]isolar.Fault, error)
}

// CredentialOpener opens a credential for adapter calls (*credentials.Service).
type CredentialOpener interface {
	Open(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Credentials, error)
}

// Deps is what the service needs; the API side leaves the worker-only fields nil.
type Deps struct {
	Plants     store.PlantRepository
	Production store.ProductionRepository
	Totals     store.ProductionTotalsRepository
	Faults     store.FaultRepository
	Solar      store.SolarTariffRepository
	Analytics  store.AnalyticsRepository
	Analyzers  store.AnalyzerRepository
	Bills      store.BillRepository
	Ops        store.OpsRepository
	AdminSolar store.AdminSolarRepository
	SMTP       store.SMTPRepository
	Mail       mail.Sender
	Creds      CredentialOpener
	ISolar     Adapter
	Clock      clock.Clock
	Enqueuer   job.Enqueuer
	Inspector  job.TaskInspector
	MaxRetry   int
}

// Service is the solar module.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System()
	}
	return &Service{d: d}
}
