// Task 12b: the loader. This file consumes the datasets, Go types and
// validation Task 12a already produced (datasets.go) and writes them through
// store.AdminCatalogueRepository — the ONLY unscoped write surface for
// platform reference data (repository.go's AdminCatalogueRepository doc).
// It performs no parsing or validation of its own beyond what upserting
// requires (assigning FactorID after the parent insert); every dataset value
// it writes has already passed datasets.go's decode+validate pass.
package seed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// Result carries the per-dataset row counts from one call to Load. Every
// count here is what the underlying admin upsert reports as WRITTEN
// (inserted-or-updated: the admin package's `on conflict … do update`
// queries count both, per Postgres's own command-tag semantics for that
// statement shape) — Task 12a's admin methods do not distinguish an insert
// from a convergence update, so "processed" is the count this Result
// reports, per the brief's "counts (inserted/updated/unchanged if the admin
// methods expose it, else processed)".
type Result struct {
	// EmissionFactors is the number of platform emission_factors rows
	// upserted (never a company-owned row: UpsertPlatformFactor hard-codes
	// company_id null in the insert and the natural-key conflict target
	// can therefore never match a company-owned row).
	EmissionFactors int64
	// EmissionFactorConversions is the total number of
	// emission_factor_conversions rows written across every factor, summed
	// from each factor's own ReplacePlatformConversions call. It is a
	// REPLACE count, not an insert count: every call deletes the platform
	// factor's existing conversions first, so this is exactly the shipped
	// dataset's conversion count on every run, never a running total that
	// grows across seed runs.
	EmissionFactorConversions int64
	// IntegrationDefinitions is the number of integration_definitions rows
	// upserted.
	IntegrationDefinitions int64
	// NationalTariffSchedule is the number of national_tariff_schedule rows
	// upserted. It is 0 for the shipped dataset (see datasets.go): the
	// source data is not in this repository.
	NationalTariffSchedule int64
}

// Total is the sum of the three top-level dataset counts (EmissionFactors,
// IntegrationDefinitions, NationalTariffSchedule). It deliberately excludes
// EmissionFactorConversions, a child count of EmissionFactors, so that
// Total() means "top-level catalogue rows processed" consistently across
// datasets. TestSeedRunTwiceProducesTheSameRowCounts asserts
// first.Total() == second.Total(), which holds because every dataset's
// count is a pure function of the embedded data, not of what existed in the
// database before the call.
func (r Result) Total() int64 {
	return r.EmissionFactors + r.IntegrationDefinitions + r.NationalTariffSchedule
}

// Load reads every embedded F1 reference dataset (datasets.go) and writes
// it through store.AdminCatalogueRepository. It is idempotent BY
// CONVERGENCE: every write is an upsert keyed on the table's natural key
// (see admin_catalogue.sql), so running Load twice against the same
// database leaves the same rows in place — including repairing a row an
// operator hand-edited back to the shipped value — rather than duplicating
// rows or silently doing nothing on the second run.
//
// It never touches a company-owned row: UpsertPlatformFactor and
// ReplacePlatformConversions operate only on company_id-null rows by
// construction (see their doc comments in
// internal/store/postgres/admin/catalogue.go), and
// national_tariff_schedule/integration_definitions have no company_id at
// all.
//
// A failure partway through leaves whatever was written before the failure
// in place (this is not wrapped in one outer transaction: each dataset's
// own upsert is already atomic, and re-running Load after fixing the cause
// converges the rest). The returned error names which dataset and, for
// emission factors, which key failed.
func Load(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (Result, error) {
	return load(ctx, admin.NewCatalogueRepository(pool), log)
}

// load is Load's testable core: catalogue is store.AdminCatalogueRepository
// rather than a concrete type so a test can substitute a fake if ever
// needed, though every seed_integration_test.go test in this package uses
// the real Postgres-backed one — the whole point of this loader is proving
// its SQL convergence, which a fake cannot stand in for.
func load(ctx context.Context, catalogue store.AdminCatalogueRepository, log *slog.Logger) (Result, error) {
	var result Result

	factorCount, conversionCount, err := loadEmissionFactors(ctx, catalogue, log)
	if err != nil {
		return Result{}, err
	}
	result.EmissionFactors = factorCount
	result.EmissionFactorConversions = conversionCount

	defCount, err := loadIntegrationDefinitions(ctx, catalogue, log)
	if err != nil {
		return Result{}, err
	}
	result.IntegrationDefinitions = defCount

	tariffCount, err := loadNationalTariffSchedule(ctx, catalogue, log)
	if err != nil {
		return Result{}, err
	}
	result.NationalTariffSchedule = tariffCount

	return result, nil
}

// loadEmissionFactors upserts every platform emission factor, then replaces
// its conversions in full. The two calls are necessarily separate database
// round trips (ReplacePlatformConversions needs the factor's id, which only
// exists once UpsertPlatformFactor has run — a fresh insert generates it,
// server-side, with gen_random_uuid()), but each factor's PAIR of calls
// converges independently: a partial failure leaves earlier factors fully
// seeded and later ones untouched, and re-running Load retries every factor
// from the top, which is safe because both calls are themselves upserts.
func loadEmissionFactors(ctx context.Context, catalogue store.AdminCatalogueRepository, log *slog.Logger) (factorCount, conversionCount int64, err error) {
	factors, err := EmissionFactors()
	if err != nil {
		return 0, 0, fmt.Errorf("load emission factors dataset: %w", err)
	}

	for _, f := range factors {
		stored, err := catalogue.UpsertPlatformFactor(ctx, toModelFactor(f))
		if err != nil {
			return 0, 0, fmt.Errorf("upsert platform emission factor %q: %w", f.Key, err)
		}
		factorCount++

		conversions := toModelConversions(stored.ID, f.Conversions)
		if err := catalogue.ReplacePlatformConversions(ctx, stored.ID, conversions); err != nil {
			return 0, 0, fmt.Errorf("replace conversions for emission factor %q: %w", f.Key, err)
		}
		conversionCount += int64(len(conversions))
	}

	log.Info("seeded emission factors",
		slog.Int64("rows", factorCount), slog.Int64("conversions", conversionCount))
	return factorCount, conversionCount, nil
}

func loadIntegrationDefinitions(ctx context.Context, catalogue store.AdminCatalogueRepository, log *slog.Logger) (int64, error) {
	defs, err := IntegrationDefinitions()
	if err != nil {
		return 0, fmt.Errorf("load integration definitions dataset: %w", err)
	}

	n, err := catalogue.UpsertIntegrationDefinitions(ctx, toModelIntegrationDefinitions(defs))
	if err != nil {
		return 0, fmt.Errorf("upsert integration definitions: %w", err)
	}
	log.Info("seeded integration definitions", slog.Int64("rows", n))
	return n, nil
}

func loadNationalTariffSchedule(ctx context.Context, catalogue store.AdminCatalogueRepository, log *slog.Logger) (int64, error) {
	entries, err := NationalTariffSchedule()
	if err != nil {
		return 0, fmt.Errorf("load national tariff schedule dataset: %w", err)
	}

	n, err := catalogue.UpsertNationalTariffSchedule(ctx, toModelNationalTariffSchedule(entries))
	if err != nil {
		return 0, fmt.Errorf("upsert national tariff schedule: %w", err)
	}
	if n == 0 {
		log.Info("national tariff schedule: 0 rows (source data not in repository)")
	} else {
		log.Info("seeded national tariff schedule", slog.Int64("rows", n))
	}
	return n, nil
}

// --- dataset -> model conversions -------------------------------------
//
// Every function below is a pure field-for-field mapping with no decoding
// or validation of its own: EmissionFactors(), IntegrationDefinitions() and
// NationalTariffSchedule() (datasets.go) have already decoded and validated
// every value (decimal.Decimal parsed from a JSON string, every enum
// checked with its own Valid(), every natural key checked for duplicates)
// before this package ever sees it.

// toModelFactor builds a PLATFORM model.EmissionFactor (CompanyID nil) from
// a decoded dataset entry. ID is left as the zero uuid: AdminUpsertPlatformFactor
// (admin_catalogue.sql) treats a zero id as "generate one on insert, keep
// the existing one on conflict" — never as a real id to write — so the
// factor's identity is the database's to assign and preserve.
func toModelFactor(f EmissionFactor) model.EmissionFactor {
	return model.EmissionFactor{
		ID:            uuid.Nil,
		CompanyID:     nil,
		Key:           f.Key,
		Label:         f.Label,
		MainCategory:  f.MainCategory,
		SubCategories: f.SubCategories,
		CategoryPath:  f.CategoryPath,
		BaseFactor:    f.BaseFactor,
		BaseUnit:      f.BaseUnit,
		FuelType:      f.FuelType,
		VehicleType:   f.VehicleType,
		Scope:         f.Scope,
		IsoCategory:   f.IsoCategory,
		Status:        f.Status,
		Source:        f.Source,
		SourceYear:    f.SourceYear,
		SourceURL:     f.SourceURL,
	}
}

// toModelConversions attaches factorID (only known after the parent factor
// has been upserted) to every conversion of one factor.
func toModelConversions(factorID uuid.UUID, cs []EmissionFactorConversion) []model.EmissionFactorConversion {
	out := make([]model.EmissionFactorConversion, len(cs))
	for i, c := range cs {
		out[i] = model.EmissionFactorConversion{
			FactorID:   factorID,
			Unit:       c.Unit,
			Multiplier: c.Multiplier,
			Label:      c.Label,
		}
	}
	return out
}

func toModelIntegrationDefinitions(defs []IntegrationDefinition) []model.IntegrationDefinition {
	out := make([]model.IntegrationDefinition, len(defs))
	for i, d := range defs {
		out[i] = model.IntegrationDefinition{
			ID:        uuid.Nil,
			Provider:  d.Provider,
			Subtype:   d.Subtype,
			Endpoints: d.Endpoints,
		}
	}
	return out
}

func toModelNationalTariffSchedule(entries []NationalTariffScheduleEntry) []model.NationalTariffScheduleEntry {
	out := make([]model.NationalTariffScheduleEntry, len(entries))
	for i, e := range entries {
		out[i] = model.NationalTariffScheduleEntry{
			ID:                uuid.Nil,
			EffectiveFrom:     e.EffectiveFrom,
			UserGroup:         e.UserGroup,
			VoltageLevel:      e.VoltageLevel,
			Term:              e.Term,
			EnergyPrice:       e.EnergyPrice,
			T1Price:           e.T1Price,
			T2Price:           e.T2Price,
			T3Price:           e.T3Price,
			DistributionPrice: e.DistributionPrice,
			PowerPrice:        e.PowerPrice,
			OverusePrice:      e.OverusePrice,
			DailyThresholdKwh: e.DailyThresholdKwh,
			VatRate:           e.VatRate,
			Source:            e.Source,
		}
	}
	return out
}
