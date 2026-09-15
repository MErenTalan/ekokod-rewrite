package integration

import (
	"context"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Provider identifies which external system an Adapter, a set of
// Credentials or a job payload belongs to. It is a superset of
// model.IntegrationProvider (the SQL enum integration_provider):
// ProviderEPIAS has no row in that enum because its credential is a
// platform-wide config value, never a tenant's integration_credentials row.
type Provider string

// The Provider values. Every provider this platform integrates with has one.
const (
	ProviderOSOS    Provider = "osos"
	ProviderGridBox Provider = "gridbox"
	ProviderARIL    Provider = "aril"
	ProviderPM5340  Provider = "pm5340"
	ProviderISolar  Provider = "isolar"
	// ProviderEPIAS is not in integration_provider: it is a platform
	// credential read from config, never a tenant's integration_credentials
	// row. ModelProvider reports ok=false for it.
	ProviderEPIAS Provider = "epias"
)

// ModelProvider maps p to the SQL enum integration_provider. ok is false
// for ProviderEPIAS, which has no row in that enum.
func (p Provider) ModelProvider() (model.IntegrationProvider, bool) {
	switch p {
	case ProviderOSOS:
		return model.IntegrationProviderOSOS, true
	case ProviderGridBox:
		return model.IntegrationProviderGridbox, true
	case ProviderARIL:
		return model.IntegrationProviderARIL, true
	case ProviderPM5340:
		return model.IntegrationProviderPM5340, true
	case ProviderISolar:
		return model.IntegrationProviderISolar, true
	default:
		return "", false
	}
}

// MeterDataSource is 06-integrations.md §1's common interface, verbatim.
// Every concrete provider adapter implements it, and implements nothing
// else that touches the network: Verify, DiscoverMeteringPoints and
// FetchReadings are the only three calls a caller ever makes.
type MeterDataSource interface {
	Provider() Provider
	Verify(ctx context.Context, creds Credentials) error
	DiscoverMeteringPoints(ctx context.Context, creds Credentials) ([]MeteringPoint, error)
	FetchReadings(ctx context.Context, creds Credentials, req FetchRequest) (FetchResult, error)
}

// Planner is what the ingestion pipeline needs beyond 06's interface: which
// reading kinds to ask a set of credentials for, and how wide a single
// FetchRequest window may be for a given kind (the provider defaults
// table). It is kept separate from MeterDataSource because it never makes
// a network call — it is pure planning logic the pipeline consults before
// building a FetchRequest.
type Planner interface {
	Kinds(creds Credentials) []model.ReadingKind
	MaxWindow(kind model.ReadingKind) time.Duration
}

// Adapter is what the registry holds and the pipeline drives: a
// MeterDataSource that also knows how to plan its own calls.
type Adapter interface {
	MeterDataSource
	Planner
}

// MeteringPoint is one installation as a provider's discovery call reports
// it. Every field beyond InstallationNumber is a pointer because a provider
// may simply not report it; a nil pointer here is "the provider said
// nothing", never "the value is zero" (removed-behaviour 21 applies the
// same rule to readings).
type MeteringPoint struct {
	InstallationNumber string

	CustomerName, Address, Province, District, Neighbourhood, Street *string
	TariffType, TariffKind, InstallationKind                         *string
	InstalledPowerKw, ContractedPowerKw                              *decimal.Decimal
	MeterNumber, MeterModel                                          *string
	// MeterMultiplier is nil when the provider did not report one; it is
	// never assumed to be 1 by this type. Multiplier resolution (R3) is the
	// adapter's job, not this struct's.
	MeterMultiplier                             *decimal.Decimal
	CounterpartyNo, MeteringPointName, EtsoCode *string
	Latitude, Longitude                         *decimal.Decimal
	DefinitionType                              *int16

	// ProviderHighWater is the latest timestamp the provider has ever
	// reported for each reading kind at this point, when the provider's
	// discovery call exposes one (e.g. OSOS's per-point high-water marks).
	ProviderHighWater map[model.ReadingKind]time.Time
}

// FetchRequest asks one adapter for one kind of reading, for one analyzer,
// over one half-open window [From, To) in UTC.
type FetchRequest struct {
	Point MeteringPoint

	// AnalyzerID and Multiplier are R3: the analyzer's own stored
	// meter_multiplier, resolved by the pipeline before the call, so the
	// adapter never has to re-derive it from MeteringPoint.MeterMultiplier.
	AnalyzerID uuid.UUID
	Multiplier decimal.Decimal

	Kind     model.ReadingKind
	From, To time.Time // half-open [From, To), UTC
}

// MultiplierSource names how a FetchResult's meter multiplier was decided,
// so an operator reading job_runs can tell "the provider gave us a
// multiplier" from "we had to guess".
type MultiplierSource string

// The MultiplierSource values. MultiplierFromRequest (R51) is FetchRequest's
// own Multiplier field — the analyzer's stored meter_multiplier, reused
// when neither of the two provider-side sources is available. It is
// deliberately distinct from MultiplierFallbackOne: reusing a value the
// pipeline itself supplied is not "assuming 1", but it is still not
// provider-resolved (see ResolvedMultiplier.ProviderResolved).
const (
	MultiplierFromLastEndex   MultiplierSource = "last_endex"
	MultiplierFromLoadProfile MultiplierSource = "load_profile_ratio"
	MultiplierFromRequest     MultiplierSource = "request_stored"
	MultiplierFallbackOne     MultiplierSource = "fallback_one"
)

// ResolvedMultiplier records which multiplier an adapter actually applied
// and how it was decided. ProviderResolved (R51/I2) is true only when the
// VALUE came from the provider itself this call (MultiplierFromLastEndex,
// MultiplierFromLoadProfile) — never for MultiplierFromRequest (an echo of
// what the pipeline already had) or MultiplierFallbackOne (a guess). The
// pipeline (internal/ingest/fetch.go) persists a multiplier change to
// analyzers.meter_multiplier ONLY when ProviderResolved is true: a
// not-provider-resolved value must never overwrite the analyzer's own
// stored multiplier, or two kinds that disagree on whether the provider
// currently supplies one (e.g. GridBox's load_profile vs. daily/reset/
// current_index/billing) would ping-pong the stored value back and forth
// on every dispatch (I2).
type ResolvedMultiplier struct {
	Value            decimal.Decimal
	Source           MultiplierSource
	ProviderResolved bool
}

// Warning is an operator-facing note attached to a FetchResult. Detail is
// free text but must never itself be, or contain, a secret or a raw
// provider payload — it is meant to be logged and shown in an operational
// message.
type Warning struct {
	Code   string // one of the Warn* constants
	Detail string // operator text; never a secret, never a raw payload
}

// The Warning.Code values.
const (
	WarnMultiplierFallback    = "multiplier_fallback"
	WarnUnparseableRow        = "unparseable_row"
	WarnGenerationIntervalNil = "generation_interval_null"
	WarnMissingPriceHours     = "missing_price_hours"
)

// FetchResult is what one FetchRequest call returns.
type FetchResult struct {
	// Readings are normalised, multiplied and in UTC — 06 §1 rule 1: no
	// provider field names, date formats or quirks escape the adapter.
	Readings           []model.MeterReading
	ResolvedMultiplier *ResolvedMultiplier
	NextCursor         *time.Time // R4: nil when the window is exhausted
	Warnings           []Warning

	// HourlyValues carries OSOS's provider-differenced consumption
	// cross-check series (06 §2, R2). It is populated only by the OSOS
	// adapter and is never mixed into Readings: it is a cross-check value,
	// not a meter index, and the pipeline (Task 10) is what turns it into a
	// model.ProviderHourlyValue for storage.
	HourlyValues []HourlyValue
}

// HourlyValue is provider-differenced consumption for one hour: a
// cross-check against the metered readings, not a register index. Declared
// here (rather than as a store type) so this package has no dependency on
// internal/store — Task 5 owns the store-side model.ProviderHourlyValue that
// the pipeline converts this into.
type HourlyValue struct {
	Ts                time.Time // hour start, UTC
	ActiveConsumption *decimal.Decimal
	ActiveGeneration  *decimal.Decimal
}
