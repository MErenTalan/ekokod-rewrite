package consumption_test

// This file is R97's own test suite (task-8-review.md's R97 implementation
// spec): ConsumptionAndRecord writing missing_readings anomalies for
// requested buckets that produced no row, and how they resolve. All tests
// use in-memory fakes — no database.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// missingReadingsIstanbul is loaded directly (this is an external test
// package, so it cannot reach the consumption package's own private
// istanbul var) — it must match exactly, since energy.Bucket needs the same
// location the package under test uses.
func missingReadingsIstanbul(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

// findAnomaly returns the single anomaly matching (analyzerID, reason,
// periodStart, periodEnd) in anomalies, failing the test if there isn't
// exactly one.
func findMissingReadingsAnomaly(t *testing.T, anomalies *fakeAnomalies, analyzerID uuid.UUID, start, end time.Time) model.ConsumptionAnomaly {
	t.Helper()
	var found []model.ConsumptionAnomaly
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "missing_readings" && a.PeriodStart.Equal(start) && a.PeriodEnd.Equal(end) {
			found = append(found, a)
		}
	}
	require.Len(t, found, 1, "expected exactly one missing_readings anomaly for (%s, %s)", start, end)
	return found[0]
}

// --- R97: which boundary side(s) are named -----------------------------

// TestMissingReadingsEndOnlyGap: R103 point 1 confines missing_readings to
// Daily/Monthly/Yearly, so this (like every other Hourly-shaped test in the
// original R97 suite) now runs at Daily. A single load_profile reading
// exactly at day1's own start resolves day1's START boundary but is too old
// (24h > the 12h Daily tolerance cap) to resolve day1's END boundary — an
// "end"-only gap, R97 spec point 2.
func TestMissingReadingsEndOnlyGap(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day1.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "the single bucket produces no row")

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"end"}, detail["boundaries"])
}

// TestMissingReadingsStartOnlyGap: the analyzer's very first reading falls
// INSIDE the first requested bucket (not before it, and within the Daily
// tolerance of the bucket's own END), so that bucket's start resolves to nil
// while its end resolves to that reading — and the bucket is not
// pre-installation (its end is AFTER the first reading, per R97 ruling
// (ii)).
func TestMissingReadingsStartOnlyGap(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			// 4h before day1.To: within the 12h Daily tolerance of the END
			// boundary, but AFTER day1.From so the START boundary has no
			// candidate at all.
			readingRow(analyzerID, day1.To.Add(-4*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"start"}, detail["boundaries"])
}

// TestMissingReadingsBothSidesGap: at Daily level (12h tolerance, half the
// 24h bucket), a SINGLE load_profile reading sits exactly at the request's
// own first bucket's start — within the look-back clamp, so it is loaded
// (anyReadings is true) — but the SECOND requested day's own boundaries
// both sit more than 12h away from it: neither its start nor its end finds
// a candidate within tolerance, and there is no daily-kind fallback either.
func TestMissingReadingsBothSidesGap(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	istanbul := missingReadingsIstanbul(t)
	day2 := energy.Bucket(energy.Daily, resolveT0, istanbul)
	day3 := energy.Bucket(energy.Daily, day2.To, istanbul)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day2.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day3.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day2.From, To: day3.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day3.From, day3.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"start", "end"}, detail["boundaries"])
	require.NotContains(t, detail, "registers")
}

// --- R97: no readings at all -> no gaps ------------------------------------

// TestMissingReadingsNoGapsWhenAnalyzerHasNoReadingsAtAll: an analyzer with
// zero readings of any boundary kind produces no missing_readings anomalies
// at all — nothing to bill, nothing missing.
func TestMissingReadingsNoGapsWhenAnalyzerHasNoReadingsAtAll(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Empty(t, anomalies.rows, "no readings at all must produce no anomalies")
}

// --- R97: future/open buckets are skipped ----------------------------------

// TestMissingReadingsFutureBucketSkipped: a bucket whose end is after the
// clock's own now must never get a gap, even though it produced no row.
func TestMissingReadingsFutureBucketSkipped(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day1.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	// now is still inside day1: the bucket [day1.From, day1.To) is still
	// open/in the future.
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.From.Add(10*time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, anomalies.rows, "an open/future bucket must never get a gap")
}

// --- R97: resolution rules --------------------------------------------------

// missingReadingsDailyGap seeds a single Daily "start"-only gap for day1
// (the same recipe TestMissingReadingsStartOnlyGap uses): one load_profile
// reading 4h before day1's own end, so day1's START never resolves but its
// END does.
func missingReadingsDailyGap(analyzerID uuid.UUID, day1 energy.Window) fakeReadings {
	return fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day1.To.Add(-4*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
}

// TestMissingReadingsResetRegisteredRejected: reset_registered is always
// ErrInvalidRequest for a missing_readings anomaly, before any read/write —
// pinned directly with noReadings so ANY Range/BulkInsert call would panic
// (item 8's "reset_registered rejected before any read for a gap").
func TestMissingReadingsResetRegisteredRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	// A FRESH Billing whose Readings is noReadings{}: reset_registered must
	// be refused before ANY ReadingRepository call (a panic here would fail
	// the test, proving the rejection happens strictly from the anomaly's
	// own reason, never from a boundary read).
	noReadBilling, nrErr := consumption.NewBilling(consumption.BillingDeps{
		Readings: noReadings{}, Anomalies: anomalies, Ops: &fakeOps{},
		Clock: clock.NewFake(day1.To.Add(time.Hour)), Log: testLog(t),
		Locker: lock.NewMemory(nil), Analyzers: fakeAnalyzers{}, Users: fakeUsers{allowAll: true},
	})
	require.NoError(t, nrErr)

	resetTS := day1.To.Add(-4 * time.Hour)
	_, err = noReadBilling.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	gotAfter, ok := anomalies.get(got.ID)
	require.True(t, ok)
	require.Nil(t, gotAfter.ResolvedAt)
}

// TestMissingReadingsAcceptedProducesNoRowAndIsNotRecreated: accepted is
// allowed, emits no row, and a rerun does not recreate the anomaly.
func TestMissingReadingsAcceptedProducesNoRowAndIsNotRecreated(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows, "accepted emits no row")

	_, err = b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	found := 0
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "missing_readings" {
			found++
		}
	}
	require.Equal(t, 1, found, "an accepted gap must never be recreated")
}

// TestMissingReadingsOverrideEmitsRow: a resolved manual_override
// missing_readings anomaly makes Consumption emit the row from the override
// values, with Resolution set and ratios computed, and Indexes nil (m-2:
// there is no end reading loaded for THIS gap's own resolved-anomaly path
// once it is fully covered by the override — Source stays "" and Indexes
// stays nil per R97 rule 7 when the caller never had an end reading to
// report; when one exists it is used instead, see
// TestMissingReadingsOverrideRowIndexesUseTheEndReadingWhenOneExists below).
func TestMissingReadingsOverrideEmitsRow(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
	})
	require.NoError(t, err)

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "42", rows[0].Values[energy.ActiveImport].String())
	require.Equal(t, "manual_override", rows[0].Resolution[energy.ActiveImport])
	require.Empty(t, rows[0].Suspect)
	// m-2: this gap has an end reading (the one 4h before day1.To), so
	// Indexes must be populated from it, never nil AND never a map of nils.
	require.NotNil(t, rows[0].Indexes)
	require.NotNil(t, rows[0].Indexes[energy.ActiveImport])
	require.Equal(t, "1000", rows[0].Indexes[energy.ActiveImport].String())
}

// TestMissingReadingsOverrideRowIndexesAreNilWithoutAnEndReading is m-2's own
// direct proof: a gap with NEITHER boundary resolved (no readings at all
// near it) synthesizes Indexes as nil, never a map of 12 nil entries.
func TestMissingReadingsOverrideRowIndexesAreNilWithoutAnEndReading(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)
	day0 := energy.Bucket(energy.Daily, day1.From.Add(-24*time.Hour), loc)

	// A reading well before day0 proves the analyzer has readings (so the
	// gap is recorded, RC-1), but leaves NEITHER of day1's own boundaries
	// resolved (both fall outside every kind's tolerance).
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day0.From.Add(-100*24*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "500"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"start", "end"}, detail["boundaries"])

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
	})
	require.NoError(t, err)

	rows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Indexes, "no end reading exists for this gap: Indexes must be nil, never a map of nils")
	require.Equal(t, energy.Kind(""), rows[0].Source)
}

// TestMissingReadingsOverrideAcceptsAnyRegister: manual_override for a
// missing_readings anomaly accepts ANY register — there is no suspect
// register set to validate against (the detail carries no registers).
func TestMissingReadingsOverrideAcceptsAnyRegister(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.T1Import: decimal.RequireFromString("7")},
	})
	require.NoError(t, err, "any register must be accepted for a missing_readings override")
}

// TestMissingReadingsRerunCreatesOneRow: re-running ConsumptionAndRecord for
// the same still-unresolved gap must not create a second anomaly (same
// dedup/lock machinery as every other reason).
func TestMissingReadingsRerunCreatesOneRow(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	_, err = b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)

	found := 0
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "missing_readings" {
			found++
		}
	}
	require.Equal(t, 1, found, "a rerun must not create a second missing_readings row")
}

// TestMissingReadingsPreInstallationBucketSkipped: a bucket that ends at or
// before the analyzer's first-ever boundary reading is pre-installation
// (R97 ruling (ii)) and must never get a gap.
func TestMissingReadingsPreInstallationBucketSkipped(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	// The ONLY reading is exactly at the bucket's own end (day1.To): the
	// whole [day1.From, day1.To) bucket predates the analyzer having any
	// data at all, and there is no OTHER (unclamped) reading before
	// day1.From either, so hasPriorReading is false and this reading itself
	// is the analyzer's earliest.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day1.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(2*time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, anomalies.rows, "a bucket ending at the analyzer's first-ever reading is pre-installation, not missing")
}

// --- R103 point 1: missing_readings never at Hourly -------------------------

// TestNoMissingReadingsAnomalyAtHourly: an Hourly request with plenty of
// buckets producing no row (an outage) must NEVER write a missing_readings
// anomaly — §3.1 absorbs the missing hour into the next emitted row, so
// nothing is actually unbillable.
func TestNoMissingReadingsAnomalyAtHourly(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	// Readings only at h and h+10h: hours h+1h..h+9h all produce no row.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(10*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "2000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(11*time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(10 * time.Hour)}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the bucket ending at h+10h derives a row (§3.1 absorbs the gap)")

	for _, a := range anomalies.rows {
		require.NotEqual(t, "missing_readings", a.Reason, "no Hourly missing_readings anomaly may ever be written")
	}
	require.Empty(t, anomalies.rows)
}

// missingReadingsHourlyDetail builds R97's own detail JSON directly (never
// through production code) for TestHourlyGapAnomalyResolutionRejected: it
// seeds an Hourly-shaped missing_readings row the way one COULD exist if it
// were ever created some other way (a stale row from before R103, a direct
// repository write) — R103 point 1 asserts ResolveAnomaly refuses it
// regardless of how it got there.
func missingReadingsHourlyDetail(t *testing.T, period energy.Window) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"code":         "consumption.suspect_period",
		"period_start": period.From.UTC().Format(time.RFC3339),
		"period_end":   period.To.UTC().Format(time.RFC3339),
		"boundaries":   []string{"start"},
	})
	require.NoError(t, err)
	return b
}

// TestHourlyGapAnomalyResolutionRejected is R103 point 1's own assertion:
// "no Hourly gap row can exist after this change" — resolving one by
// manual_override or accepted, however it got there, is ErrInvalidRequest.
func TestHourlyGapAnomalyResolutionRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	overrideAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "missing_readings", Detail: missingReadingsHourlyDetail(t, hour),
	})
	acceptAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "missing_readings", Detail: missingReadingsHourlyDetail(t, hour),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, overrideAnomaly.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("1")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	_, err = b.ResolveAnomaly(ctx, scope, acceptAnomaly.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	for _, id := range []uuid.UUID{overrideAnomaly.ID, acceptAnomaly.ID} {
		got, ok := anomalies.get(id)
		require.True(t, ok)
		require.Nil(t, got.ResolvedAt, "an Hourly gap row must never be resolved")
	}
}

// --- RC-1: gap detection and gap-override lookup are range-independent -----

// TestSingleOfflineMonthGetsItsAnomaly is RC-1's own probe: an analyzer with
// load_profile readings Mar 1-10 (Istanbul) and nothing after gets a
// missing_readings anomaly for a SINGLE requested month (Apr), not only when
// the request also happens to include March. Before RC-1, "the analyzer has
// readings" and "pre-installation" were decided from the CLAMPED look-back
// pool, which for a single-month request is empty — so the bug reported 0
// anomalies for exactly this case.
func TestSingleOfflineMonthGetsItsAnomaly(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	mar1 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	apr := energy.Bucket(energy.Monthly, time.Date(2026, 4, 1, 0, 0, 0, 0, loc), loc)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, mar1, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, mar1.AddDate(0, 0, 9), model.ReadingKindLoadProfile, map[string]string{"active_import": "1050"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), now)

	// The request covers ONLY April — no March bucket at all.
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly, Range: store.TimeRange{From: apr.From, To: apr.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, apr.From, apr.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"start", "end"}, detail["boundaries"])
}

// TestOutageResumingMidRequestDoesNotTreatEarlierDaysAsPreInstallation is
// RC-1's second probe: the analyzer has a real reading LONG before this
// request's own first bucket (proving it was already in service), then goes
// offline for several days before the request begins, resuming only at the
// request's last bucket. Before RC-1, earliest was computed from the
// CLAMPED loaded slices alone, making the resumption reading look like the
// installation date and skipping every earlier outage day as
// "pre-installation" (0 gaps). RC-1 must record a gap for every outage day.
func TestOutageResumingMidRequestDoesNotTreatEarlierDaysAsPreInstallation(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)
	// day6 is the 6th requested day (day1..day6): the analyzer resumes
	// reporting exactly at its own start.
	day6 := energy.Bucket(energy.Daily, day1.From.AddDate(0, 0, 5), loc)
	// Long before day1: proves genuine prior service, well outside every
	// kind's clamp/tolerance, so it is excluded from every loaded slice.
	ancient := day1.From.AddDate(0, 0, -100)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, ancient, model.ReadingKindLoadProfile, map[string]string{"active_import": "500"}),
			readingRow(analyzerID, day6.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day6.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day6.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)

	// day1 (deep in the outage) must have its own gap anomaly — the bug
	// this test pins skipped it as "pre-installation".
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)
	var detail map[string]any
	require.NoError(t, json.Unmarshal(got.Detail, &detail))
	require.Equal(t, []any{"start", "end"}, detail["boundaries"])

	// Every requested day (all 6) gets EITHER a row or a gap, never silently
	// dropped — total anomaly count for the 5 outage days plus day6's own
	// zero-width end.
	gapCount := 0
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "missing_readings" {
			gapCount++
		}
	}
	require.Equal(t, 6, gapCount, "no outage day may be silently skipped as pre-installation")
}

// TestMonthlyGapUsesBillingKindPriorRangeIndependently is RI2-1's own test:
// R103(2)'s "earliest reading of any boundary kind" (billing.go's
// hasPriorReading, computed from lpHasPrior || billingHasPrior ||
// dailyHasPrior) must be pinned by billing-kind history too, never only
// load_profile's own. This analyzer's ONLY history anywhere is two
// billing-kind snapshots (Jan 1, Feb 1) — no load_profile, no daily, and
// nothing at all inside or after April. April must get the SAME gap,
// ["start","end"], whether Consumption is asked for the whole [Jan,May) or
// for April alone.
//
// Dropping `|| billingHasPrior` (billing.go ~526) makes ONLY the
// April-alone request silently drop the gap: April's own clamp (its first
// bucket minus BillingSnapshotTolerance) excludes Jan 1 and Feb 1, so
// anyReadings depends entirely on the dropped term. The wide request still
// finds it "by accident", because Jan 1 and Feb 1 sit inside THAT request's
// own clamp (its first bucket is January) and so are loaded into
// data.billing regardless of hasPriorReading — the exact range dependence
// R103(2) exists to rule out.
func TestMonthlyGapUsesBillingKindPriorRangeIndependently(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	jan := energy.Bucket(energy.Monthly, time.Date(2026, 1, 1, 0, 0, 0, 0, loc), loc)
	feb1 := jan.To
	apr := energy.Bucket(energy.Monthly, time.Date(2026, 4, 1, 0, 0, 0, 0, loc), loc)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	newReadings := func() fakeReadings {
		return fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindBilling: {
				readingRow(analyzerID, jan.From, model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, feb1, model.ReadingKindBilling, map[string]string{"active_import": "1050"}),
			},
			model.ReadingKindLoadProfile: {},
			model.ReadingKindDaily:       {},
		}}
	}

	aprilGapBoundaries := func(req consumption.SeriesRequest) []any {
		anomalies := &fakeAnomalies{}
		b := resolveBilling(t, newReadings(), anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), now)
		_, err := b.ConsumptionAndRecord(ctx, scope, req)
		require.NoError(t, err)
		got := findMissingReadingsAnomaly(t, anomalies, analyzerID, apr.From, apr.To)
		var detail map[string]any
		require.NoError(t, json.Unmarshal(got.Detail, &detail))
		return detail["boundaries"].([]any)
	}

	narrow := aprilGapBoundaries(consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly,
		Range: store.TimeRange{From: apr.From, To: apr.To},
	})
	wide := aprilGapBoundaries(consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly,
		Range: store.TimeRange{From: jan.From, To: apr.To},
	})

	require.Equal(t, []any{"start", "end"}, narrow, "April requested alone must still get its own gap")
	require.Equal(t, []any{"start", "end"}, wide, "April's gap must be identical inside a wider request")
	require.Equal(t, wide, narrow, "the gap decision must not depend on which other months share the request")
}

// TestDailyGapUsesDailyKindPriorRangeIndependently is RI2-1's second test,
// the same pinning for the daily-kind fallback boundary (dailyHasPrior,
// billing.go ~544). This analyzer's ONLY history anywhere is ten daily-kind
// snapshots (Jan 1-10) — no load_profile, no billing, and nothing at all
// inside or after the request. April 1 must get the SAME gap,
// ["start","end"], whether Consumption is asked for the whole
// [Jan 1, Apr 2) or for April 1 alone.
//
// Dropping `|| dailyHasPrior` (billing.go ~544) makes ONLY the day-alone
// request silently drop the gap: its own clamp (its first bucket minus
// DailySnapshotTolerance) excludes every one of Jan 1-10. The wide request
// still finds it, because Jan 1-10 sit inside THAT request's own clamp
// (its first bucket is January 1) and so are loaded into data.daily
// regardless of hasPriorReading.
func TestDailyGapUsesDailyKindPriorRangeIndependently(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	jan1 := energy.Bucket(energy.Daily, time.Date(2026, 1, 1, 0, 0, 0, 0, loc), loc)
	apr1 := energy.Bucket(energy.Daily, time.Date(2026, 4, 1, 0, 0, 0, 0, loc), loc)
	now := apr1.To.Add(24 * time.Hour)

	newReadings := func() fakeReadings {
		daily := make([]model.MeterReading, 0, 10)
		for i := 0; i < 10; i++ {
			ts := jan1.From.AddDate(0, 0, i)
			daily = append(daily, readingRow(analyzerID, ts, model.ReadingKindDaily, map[string]string{
				"active_import": fmt.Sprintf("%d", 1000+i*10),
			}))
		}
		return fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindDaily:       daily,
			model.ReadingKindLoadProfile: {},
			model.ReadingKindBilling:     {},
		}}
	}

	april1GapBoundaries := func(req consumption.SeriesRequest) []any {
		anomalies := &fakeAnomalies{}
		b := resolveBilling(t, newReadings(), anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), now)
		_, err := b.ConsumptionAndRecord(ctx, scope, req)
		require.NoError(t, err)
		got := findMissingReadingsAnomaly(t, anomalies, analyzerID, apr1.From, apr1.To)
		var detail map[string]any
		require.NoError(t, json.Unmarshal(got.Detail, &detail))
		return detail["boundaries"].([]any)
	}

	narrow := april1GapBoundaries(consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily,
		Range: store.TimeRange{From: apr1.From, To: apr1.To},
	})
	wide := april1GapBoundaries(consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily,
		Range: store.TimeRange{From: jan1.From, To: apr1.To},
	})

	require.Equal(t, []any{"start", "end"}, narrow, "April 1 requested alone must still get its own gap")
	require.Equal(t, []any{"start", "end"}, wide, "April 1's gap must be identical inside a wider request")
	require.Equal(t, wide, narrow, "the gap decision must not depend on which other days share the request")
}

// TestResolvedGapOverrideGivesSameRowForNarrowAndWideRequest is RC-1's third
// probe: a resolved gap override for April must produce the IDENTICAL row
// whether Consumption is asked for [Mar,May) or for [Apr,May) alone — the
// override lookup must never depend on which OTHER buckets happen to be in
// the same request.
func TestResolvedGapOverrideGivesSameRowForNarrowAndWideRequest(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	mar := energy.Bucket(energy.Monthly, time.Date(2026, 3, 1, 0, 0, 0, 0, loc), loc)
	apr := energy.Bucket(energy.Monthly, mar.To, loc)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, mar.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, mar.From.AddDate(0, 0, 9), model.ReadingKindLoadProfile, map[string]string{"active_import": "1050"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), now)

	wideReq := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly, Range: store.TimeRange{From: mar.From, To: apr.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, wideReq)
	require.NoError(t, err)

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, apr.From, apr.To)
	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("9000")},
	})
	require.NoError(t, err)

	wideRows, err := b.Consumption(ctx, scope, wideReq)
	require.NoError(t, err)
	var wideApr *consumption.Row
	for i := range wideRows {
		if wideRows[i].Window.From.Equal(apr.From) {
			wideApr = &wideRows[i]
		}
	}
	require.NotNil(t, wideApr, "the wide [Mar,May) request must emit the April override row")
	require.Equal(t, "9000", wideApr.Values[energy.ActiveImport].String())

	narrowReq := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly, Range: store.TimeRange{From: apr.From, To: apr.To}}
	narrowRows, err := b.Consumption(ctx, scope, narrowReq)
	require.NoError(t, err)
	require.Len(t, narrowRows, 1, "the narrow [Apr,May) request must ALSO emit the April override row, not drop it")
	require.Equal(t, "9000", narrowRows[0].Values[energy.ActiveImport].String())
	require.Equal(t, "manual_override", narrowRows[0].Resolution[energy.ActiveImport])
}

// --- RI-5: a gap override applies only until real data resolves the bucket -

// TestGapOverrideSupersededByRealDerivationOnceBackfilled is RI-5: a
// resolved missing_readings override applies only while the bucket still
// produces no row from real readings. Once a backfilled start reading makes
// day1 derive a REAL (here, negative and therefore suspect) row, the real
// derivation wins — Consumption must return the real Suspect state, never
// the stale override value — and ConsumptionAndRecord must record the fresh
// suspicion as an ordinary negative_delta suspect period, not silently keep
// billing the override.
func TestGapOverrideSupersededByRealDerivationOnceBackfilled(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1) // end-boundary reading = 1000, active_import
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
	})
	require.NoError(t, err)

	// Confirm the override applies BEFORE the backfill (RI-5's "only while
	// the bucket still produces no row" half).
	beforeRows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, beforeRows, 1)
	require.Equal(t, "42", beforeRows[0].Values[energy.ActiveImport].String())

	// Backfill day1's own START reading: 2000, against the existing end
	// reading of 1000 -> a real, NEGATIVE (suspect) derivation. Rebuilt in
	// ascending ts order (never merely appended): fakeReadings documents
	// that its byKind slices must already be sorted, the same precondition
	// the real ReadingRange query's ORDER BY guarantees.
	readings.byKind[model.ReadingKindLoadProfile] = []model.MeterReading{
		readingRow(analyzerID, day1.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "2000"}),
		readings.byKind[model.ReadingKindLoadProfile][0],
	}

	afterRows, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, afterRows, 1)
	require.Nil(t, afterRows[0].Values[energy.ActiveImport], "the real derivation is suspect: Values must be nil, never the stale override")
	require.Contains(t, afterRows[0].Suspect, energy.ActiveImport, "the real negative delta must be reported, never hidden by the stale override")
	require.Empty(t, afterRows[0].Resolution, "a fresh real derivation carries no resolution from the superseded gap override")

	_, err = b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	var negDelta int
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "negative_delta" && a.PeriodStart.Equal(day1.From) && a.PeriodEnd.Equal(day1.To) {
			negDelta++
		}
	}
	require.Equal(t, 1, negDelta, "the backfilled suspect bucket must be recorded as an ordinary suspect period")
}

// --- I1: the gap dedup lock must genuinely serialize too --------------------

// TestConcurrentGapRunsAreSerializedByTheLock is I1's own extension to the
// R97 gap path: recordMissingReadingsGap (anomalies.go) takes the SAME
// per-(analyzer, period_start) lease recordSuspectPeriod does. Two
// goroutines racing ConsumptionAndRecord for the same still-unresolved gap
// must still produce exactly one missing_readings row — removing
// recordMissingReadingsGap's own lock acquisition makes this
// deterministically red, the same two-gate (list + create) barrier shape
// TestConcurrentRunsAreSerializedByTheLockNotByLuck uses.
func TestConcurrentGapRunsAreSerializedByTheLock(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{
		listBarrier:   dedupRangeBarrier(2, 150*time.Millisecond),
		createBarrier: raceBarrier(2, 150*time.Millisecond),
	}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}

	const n = 2
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = b.ConsumptionAndRecord(ctx, scope, req)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	count := 0
	for _, a := range anomalies.rows {
		if a.AnalyzerID == analyzerID && a.Reason == "missing_readings" {
			count++
		}
	}
	require.Equal(t, 1, count, "the gap dedup lock must serialize concurrent creation, never create two rows for the same gap")
}

// --- R97 rule 6: no F2 cascade on a gap resolution --------------------------

// TestGapResolutionNeverCascadesToOverlappingF2Rows pins the "no cascade"
// rule the R97 spec's rule 6 states explicitly: resolving a missing_readings
// anomaly (any mode) must never touch an F2-shaped negative_delta row
// nested inside it, unlike a register-reasoned F3 resolution's own I-5
// cascade.
func TestGapResolutionNeverCascadesToOverlappingF2Rows(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	day1 := energy.Bucket(energy.Daily, resolveT0, loc)
	nested := energy.Window{From: day1.From.Add(2 * time.Hour), To: day1.From.Add(3 * time.Hour)}

	readings := missingReadingsDailyGap(analyzerID, day1)
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), day1.To.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day1.From, To: day1.To}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, day1.From, day1.To)

	f2Row := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})

	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
	})
	require.NoError(t, err)

	gotF2, ok := anomalies.get(f2Row.ID)
	require.True(t, ok)
	require.Nil(t, gotF2.ResolvedAt, "a gap resolution must never cascade to an overlapping F2 row")
}

// --- Minor: override rows are emitted in bucket order, never appended ------

// TestOverrideRowsAreEmittedInBucketOrder is item 8's own minor: with TWO
// gaps resolved by override in a single Monthly request, the synthesized
// rows must come back sorted by their own window, in bucket order — never
// appended after the analyzer's other rows regardless of resolution order.
func TestOverrideRowsAreEmittedInBucketOrder(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc := missingReadingsIstanbul(t)
	mar := energy.Bucket(energy.Monthly, time.Date(2026, 3, 1, 0, 0, 0, 0, loc), loc)
	apr := energy.Bucket(energy.Monthly, mar.To, loc)
	may := energy.Bucket(energy.Monthly, apr.To, loc)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)

	// A reading well before March (hasPriorReading only, clamped out of
	// every loaded slice) plus TWO readings framing April exactly: April
	// derives a REAL, sound row (a positive delta) while March (start
	// unresolved) and May (end unresolved) both gap. Appending
	// override-synthesized rows AFTER the real rows, unsorted, would return
	// [April, March, May] instead of the correct [March, April, May].
	feb := mar.From.AddDate(0, -1, 0)
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, feb, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, apr.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, apr.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), now)

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly, Range: store.TimeRange{From: mar.From, To: may.To}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1, "only April derives a real row")
	require.True(t, rows[0].Window.From.Equal(apr.From))

	mayAnomaly := findMissingReadingsAnomaly(t, anomalies, analyzerID, may.From, may.To)
	marAnomaly := findMissingReadingsAnomaly(t, anomalies, analyzerID, mar.From, mar.To)

	// Resolve MAY (the LAST gap) before MARCH (the FIRST).
	_, err = b.ResolveAnomaly(ctx, scope, mayAnomaly.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("500")},
	})
	require.NoError(t, err)
	_, err = b.ResolveAnomaly(ctx, scope, marAnomaly.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("300")},
	})
	require.NoError(t, err)

	got, err := b.Consumption(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, got, 3, "March (override), April (real) and May (override)")
	require.True(t, got[0].Window.From.Equal(mar.From), "March must come first, in bucket order")
	require.True(t, got[1].Window.From.Equal(apr.From), "April must come second, in bucket order")
	require.True(t, got[2].Window.From.Equal(may.From), "May must come third, in bucket order")
}
