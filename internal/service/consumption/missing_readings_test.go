package consumption_test

// This file is R97's own test suite (task-8-review.md's R97 implementation
// spec): ConsumptionAndRecord writing missing_readings anomalies for
// requested buckets that produced no row, and how they resolve. All tests
// use in-memory fakes — no database.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
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

// TestMissingReadingsEndOnlyGap: two Hourly readings two hours apart (h and
// h+2h) with nothing at h+1h. Bucket [h,h+1h) resolves start=end (both the
// reading at h, R92's zero-width rule), which R97 spec point 2 classifies
// as an "end"-only gap; bucket [h+1h,h+2h) derives a real, sound row.
func TestMissingReadingsEndOnlyGap(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(2*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(3*time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(2 * time.Hour)}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the second bucket derives a row")

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))
	require.Equal(t, `{"code":"consumption.suspect_period","period_start":"2026-06-01T00:00:00Z","period_end":"2026-06-01T01:00:00Z","boundaries":["end"]}`, string(got.Detail))
}

// TestMissingReadingsStartOnlyGap: the analyzer's very first reading falls
// INSIDE the first requested bucket (not before it), so that bucket's start
// resolves to nil while its end resolves to that same reading — and the
// bucket is not pre-installation (its end is AFTER the first reading, per
// R97 ruling (ii)).
func TestMissingReadingsStartOnlyGap(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	rows, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, rows)

	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))
	require.Equal(t, `{"code":"consumption.suspect_period","period_start":"2026-06-01T00:00:00Z","period_end":"2026-06-01T01:00:00Z","boundaries":["start"]}`, string(got.Detail))
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
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
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
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	// now == h + 30m: the bucket [h, h+1h) is still open/in the future.
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(30*time.Minute))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, anomalies.rows, "an open/future bucket must never get a gap")
}

// --- R97: resolution rules --------------------------------------------------

// TestMissingReadingsResetRegisteredRejected: reset_registered is always
// ErrInvalidRequest for a missing_readings anomaly, before any read/write.
func TestMissingReadingsResetRegisteredRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))

	resetTS := h.Add(30 * time.Minute)
	_, err = b.ResolveAnomaly(ctx, scope, got.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
	require.Empty(t, readings.byKind[model.ReadingKindReset], "nothing may be written")
}

// TestMissingReadingsAcceptedProducesNoRowAndIsNotRecreated: accepted is
// allowed, emits no row, and a rerun does not recreate the anomaly.
func TestMissingReadingsAcceptedProducesNoRowAndIsNotRecreated(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))

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
// values, with Resolution set and ratios computed.
func TestMissingReadingsOverrideEmitsRow(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))

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
}

// TestMissingReadingsOverrideAcceptsAnyRegister: manual_override for a
// missing_readings anomaly accepts ANY register — there is no suspect
// register set to validate against (the detail carries no registers).
func TestMissingReadingsOverrideAcceptsAnyRegister(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	got := findMissingReadingsAnomaly(t, anomalies, analyzerID, h, h.Add(time.Hour))

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
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(30*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
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
	h := resolveT0

	// The ONLY reading is exactly at the bucket's own end (h+1h): the whole
	// [h, h+1h) bucket predates the analyzer having any data at all.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(2*time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)
	require.Empty(t, anomalies.rows, "a bucket ending at the analyzer's first-ever reading is pre-installation, not missing")
}
