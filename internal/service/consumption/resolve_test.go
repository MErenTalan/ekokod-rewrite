package consumption_test

// This file is ResolveAnomaly's unit test suite: the F2 cascade
// (resolveOverlappingIngestionAnomalies), the reset_registered
// re-derivation path (validateAndPrepareReset), dedup/lookup pagination
// past the repository's default page size, and the ordering/authorization
// guards around every write. Every test here uses in-memory fakes
// (fakeReadings/fakeAnomalies/fakeOps/fakeAnalyzers/fakeUsers,
// helpers_test.go) and a real lock.NewMemory — no database — so the whole
// file runs under `go test ./internal/service/consumption/... -race`.
//
// anomalies_integration_test.go (real Postgres, real tenancy) covers what
// needs a real database: cross-tenant isolation, a real concurrent-run
// proof against the actual table, and the R93-completeness reset test.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// resolveT0 anchors this file's fixed clock.
var resolveT0 = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// resolveBilling builds a *consumption.Billing wired with every dependency
// this file's tests may need.
func resolveBilling(t *testing.T, readings fakeReadings, anomalies *fakeAnomalies, ops *fakeOps, analyzers fakeAnalyzers, locker lock.Locker, now time.Time) *consumption.Billing {
	t.Helper()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  readings,
		Anomalies: anomalies,
		Ops:       ops,
		Clock:     clock.NewFake(now),
		Log:       testLog(t),
		Locker:    locker,
		Analyzers: analyzers,
		// R103: Users is required for ResolveAnomaly. This file's tests
		// exercise other behaviour, so allowAll accepts whichever
		// uuid.New() resolvedBy each test happens to pass — the tests that
		// specifically cover Users' own membership check build their own
		// BillingDeps directly (TestResolvedByMustBeAUserOfTheCompany and
		// friends).
		Users: fakeUsers{allowAll: true},
	})
	require.NoError(t, err)
	return b
}

// f2Detail is F2's own detail shape (internal/ingest/anomaly.go, R59): one
// row per affected register, no "code" field.
func f2Detail(t *testing.T, register, kind string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{"register": register, "kind": kind})
	require.NoError(t, err)
	return b
}

// f3Detail is this package's own R60 detail shape for a register-reasoned
// row (negative_delta/meter_reset).
func f3Detail(t *testing.T, period energy.Window, reason string, registers ...string) []byte {
	t.Helper()
	regs := make(map[string]any, len(registers))
	for _, r := range registers {
		regs[r] = map[string]any{"reason": reason, "reset_rows": 0}
	}
	detail := map[string]any{
		"code":         "consumption.suspect_period",
		"period_start": period.From.UTC().Format(time.RFC3339),
		"period_end":   period.To.UTC().Format(time.RFC3339),
		"registers":    regs,
	}
	b, err := json.Marshal(detail)
	require.NoError(t, err)
	return b
}

// --- The F2 overlap cascade must never touch an F3 row ---------------------

// TestF2CascadeNeverAppliesToNestedF3Row proves a Daily anomaly resolved
// manual_override must never cascade onto a NESTED Hourly F3 row sharing
// the negative_delta reason, even though it lies fully inside the resolved
// period.
func TestF2CascadeNeverAppliesToNestedF3Row(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	day := energy.Window{From: resolveT0, To: resolveT0.AddDate(0, 0, 1)}
	hour := energy.Window{From: resolveT0.Add(10 * time.Hour), To: resolveT0.Add(11 * time.Hour)}

	anomalies := &fakeAnomalies{}
	dailyAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: day.From, PeriodEnd: day.To,
		Reason: "negative_delta", Detail: f3Detail(t, day, "negative_delta", "active_import"),
	})
	hourlyAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, dailyAnomaly.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("5000")},
	})
	require.NoError(t, err)

	got, ok := anomalies.get(hourlyAnomaly.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt, "the Daily override must never cascade onto the nested Hourly F3 row")
}

// TestF2CascadeNeverAppliesWithinPureF3Hierarchy covers pure F3, no F2 row
// at all: resolving a Daily anomaly must never resolve a nested Hourly F3
// anomaly of the same reason.
func TestF2CascadeNeverAppliesWithinPureF3Hierarchy(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	day := energy.Window{From: resolveT0, To: resolveT0.AddDate(0, 0, 1)}
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	dailyAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: day.From, PeriodEnd: day.To,
		Reason: "negative_delta", Detail: f3Detail(t, day, "negative_delta", "active_import"),
	})
	hourlyAnomaly := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, dailyAnomaly.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("5000")},
	})
	require.NoError(t, err)

	got, ok := anomalies.get(hourlyAnomaly.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt, "a pure F3 hierarchy must never cascade onto a nested row")
}

// TestF2CascadeAppliesOnlyToF2ShapedRows proves an F2-shaped row fully
// contained in the resolved F3 period IS cascaded, while a sibling
// F3-shaped row at the exact same bounds is not.
func TestF2CascadeAppliesOnlyToF2ShapedRows(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}
	nested := energy.Window{From: resolveT0.Add(10 * time.Minute), To: resolveT0.Add(20 * time.Minute)}

	anomalies := &fakeAnomalies{}
	f3 := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})
	f2Row := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})
	f3Sibling := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "negative_delta", Detail: f3Detail(t, nested, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, f3.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	gotF2, ok := anomalies.get(f2Row.ID)
	require.True(t, ok)
	require.NotNil(t, gotF2.ResolvedAt, "the F2-shaped row fully inside the period must be cascaded")

	gotF3Sibling, ok := anomalies.get(f3Sibling.ID)
	require.True(t, ok)
	require.Nil(t, gotF3Sibling.ResolvedAt, "an F3-shaped row must never be cascaded even at identical bounds")
}

// TestF2CascadeNeverCopiesOverrideValues proves a cascaded F2 row's own
// OverrideValues column is always nil, even when the F3 resolution being
// cascaded is itself a manual_override.
func TestF2CascadeNeverCopiesOverrideValues(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}
	nested := energy.Window{From: resolveT0.Add(10 * time.Minute), To: resolveT0.Add(20 * time.Minute)}

	anomalies := &fakeAnomalies{}
	f3 := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})
	f2Row := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, f3.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("5000")},
	})
	require.NoError(t, err)

	gotF2, ok := anomalies.get(f2Row.ID)
	require.True(t, ok)
	require.NotNil(t, gotF2.ResolvedAt)
	require.Empty(t, gotF2.OverrideValues, "a cascaded row must never carry the F3 resolution's own override values")
}

// --- The cascade requires FULL containment, never mere overlap -------------

// TestF2CascadeRequiresFullContainment proves an F2 row that only overlaps
// the resolved period (starts before it) is left untouched, while one fully
// inside it is cascaded.
func TestF2CascadeRequiresFullContainment(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}
	// overlapping STARTS inside the period (so the dedup lock's own
	// candidate query, which filters on period_start alone, still surfaces
	// it) but ENDS after it — the shape the containment check itself must
	// catch.
	overlapping := energy.Window{From: resolveT0.Add(50 * time.Minute), To: resolveT0.Add(70 * time.Minute)}
	contained := energy.Window{From: resolveT0.Add(20 * time.Minute), To: resolveT0.Add(30 * time.Minute)}

	anomalies := &fakeAnomalies{}
	f3 := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})
	overlappingRow := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: overlapping.From, PeriodEnd: overlapping.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})
	containedRow := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: contained.From, PeriodEnd: contained.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, f3.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	gotOverlap, ok := anomalies.get(overlappingRow.ID)
	require.True(t, ok)
	require.Nil(t, gotOverlap.ResolvedAt, "a row that only overlaps the period must stay unresolved")

	gotContained, ok := anomalies.get(containedRow.ID)
	require.True(t, ok)
	require.NotNil(t, gotContained.ResolvedAt, "a row fully inside the period must be cascaded")
}

// TestF2CascadeReasonFilterExcludesOtherReasons proves a row shaped for the
// cascade but reasoned meter_reset (not negative_delta) must never be
// touched, even fully contained.
func TestF2CascadeReasonFilterExcludesOtherReasons(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}
	nested := energy.Window{From: resolveT0.Add(10 * time.Minute), To: resolveT0.Add(20 * time.Minute)}

	anomalies := &fakeAnomalies{}
	f3 := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})
	otherReason := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "meter_reset", Detail: f2Detail(t, "active_import", "load_profile"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, f3.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	got, ok := anomalies.get(otherReason.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt, "a differently-reasoned row must never be cascaded")
}

// --- reset_registered re-derives the period in memory before writing -------

// TestResetOutsidePeriodRejected proves a ResetTS after the period's own
// end must be rejected before any write, and the neighbouring period must
// never be disturbed.
func TestResetOutsidePeriodRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	resetTS := h.Add(90 * time.Minute) // AFTER period.To
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	require.Empty(t, readings.byKind[model.ReadingKindReset], "nothing may be written")
	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt)
}

// TestResetAtOrBeforePeriodStartRejected: a ResetTS equal to period.From is
// outside the (From, To] window ResolveByRegisteringReset requires.
func TestResetAtOrBeforePeriodStartRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	resetTS := h // == period.From, never valid.
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
	require.Empty(t, readings.byKind[model.ReadingKindReset])
}

// TestResetWithWrongAfterValueRejected proves a reset inside the period
// whose after-value still leaves the register suspect must be rejected,
// and nothing written.
func TestResetWithWrongAfterValueRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	// start=1000, prior at +20m=1040, end=190 (negative, uncovered).
	// R55: (1040-1000) + (190-after) must be non-negative and consistent —
	// an after-value of 500 leaves (190-500) negative, so the register stays
	// suspect after the proposed reset is merged in.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	resetTS := h.Add(30 * time.Minute)
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("500")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	require.Empty(t, readings.byKind[model.ReadingKindReset], "nothing may be written")
	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt)
}

// TestResetLeavingAnotherRegisterSuspectRejected proves R93's completeness
// rule, enforced purely by re-derivation: t1_import is reported by both
// boundaries and is NOT itself suspect, but omitting it from ResetAfter
// makes it meter_reset-suspect once the reset participates in the window —
// and validateAndPrepareReset rejects the WHOLE request when ANY register
// comes back suspect.
func TestResetLeavingAnotherRegisterSuspectRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000", "t1_import": "500"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900", "t1_import": "600"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	resetTS := h.Add(30 * time.Minute)
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
		// t1_import deliberately omitted.
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
	require.Empty(t, readings.byKind[model.ReadingKindReset])
}

// TestPeriodThatIsNotABucketRejected proves a period that is not exactly
// one whole bucket at any level is ErrInvalidRequest, never a silent
// Hourly fallback.
func TestPeriodThatIsNotABucketRejected(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {},
		model.ReadingKindReset:       {},
	}}
	anomalies := &fakeAnomalies{}
	// 90 minutes: not a whole bucket at Hourly, Daily, Monthly or Yearly.
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: h, PeriodEnd: h.Add(90 * time.Minute),
		Reason: "negative_delta", Detail: f3Detail(t, energy.Window{From: h, To: h.Add(90 * time.Minute)}, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(2*time.Hour))

	resetTS := h.Add(30 * time.Minute)
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)
}

// TestPeriodThatIsNotABucketRejectedEvenWithRealBoundaryReadings uses REAL
// load_profile readings exactly at the non-bucket period's own start/end
// (rather than no readings at all): a silent "!ok -> treat as Hourly"
// fallback in inferLevel would derive a perfectly plausible, sound value
// from this fixture instead of failing for an unrelated reason, so this
// test actually exercises inferLevel's own rejection rather than merely
// deriveRow failing on empty data.
func TestPeriodThatIsNotABucketRejectedEvenWithRealBoundaryReadings(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(2 * time.Hour)} // not a bucket at any level

	// A reading strictly between h and the proposed reset (h+1h) supplies
	// the "prior" R91 needs, so a re-derivation that silently treated this
	// 2-hour period as Hourly would come back perfectly SOUND (20 + 50 =
	// 70, no suspicion) instead of failing — proving the rejection below
	// comes from inferLevel's own !ok, not from an unrelated derivation
	// failure (R91/no-prior) the way the empty-readings
	// TestPeriodThatIsNotABucketRejected above would.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, period.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, period.From.Add(45*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1020"}),
			readingRow(analyzerID, period.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}}, lock.NewMemory(nil), period.To.Add(time.Hour))

	resetTS := period.From.Add(time.Hour)
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("1050")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt)
}

// TestRegisteringAValidResetSucceeds proves a reset that fully covers every
// register both boundaries report is accepted, written, and the anomaly
// resolved.
func TestRegisteringAValidResetSucceeds(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	resetTS := h.Add(30 * time.Minute)
	resolved, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
	require.Len(t, readings.byKind[model.ReadingKindReset], 1, "the reset reading must be written")

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Suspect)
	require.Equal(t, "130", rows[0].Values[energy.ActiveImport].String())
}

// --- inferLevel must be exercised above Hourly, and on a DST day ----------

// TestRegisteringAValidResetSucceedsAtDailyLevel reproduces the SAME
// numeric relationship TestRegisteringAValidResetSucceeds proves at Hourly
// (1000 start, 1040 prior, 190 end, reset-after 100 -> 130 sound) over a
// whole Daily bucket, so inferLevel is exercised above Hourly and not just
// assumed correct there.
func TestRegisteringAValidResetSucceedsAtDailyLevel(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	day := energy.Bucket(energy.Daily, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, day.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, day.From.Add(20*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, day.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
		model.ReadingKindReset:   {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: day.From, PeriodEnd: day.To,
		Reason: "negative_delta", Detail: f3Detail(t, day, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}}, lock.NewMemory(nil), day.To.Add(consumption.SettleDelayDaily))

	resetTS := day.From.Add(21 * time.Hour)
	resolved, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: day.From, To: day.To},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Suspect)
	require.Equal(t, "130", rows[0].Values[energy.ActiveImport].String())
}

// TestRegisteringAValidResetSucceedsAtMonthlyLevel reproduces the SAME
// numeric relationship at Monthly, over a whole Monthly bucket.
func TestRegisteringAValidResetSucceedsAtMonthlyLevel(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	month := energy.Bucket(energy.Monthly, resolveT0, loc)

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, month.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, month.From.AddDate(0, 0, 20), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, month.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
		model.ReadingKindReset:   {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: month.From, PeriodEnd: month.To,
		Reason: "negative_delta", Detail: f3Detail(t, month, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}}, lock.NewMemory(nil), month.To.Add(consumption.SettleDelayMonthly))

	resetTS := month.From.AddDate(0, 0, 21)
	resolved, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Monthly, Range: store.TimeRange{From: month.From, To: month.To},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Suspect)
	require.Equal(t, "130", rows[0].Values[energy.ActiveImport].String())
}

// TestRegisteringAValidResetSucceedsOnADSTDay proves inferLevel still
// recognises a non-24h Daily bucket as exactly one Daily bucket: 2015-03-29
// is Istanbul's spring-forward day, a 23-hour Daily bucket (energy.Bucket
// itself already handles the DST arithmetic; this proves
// rederiveBucketWithReset actually reaches it).
func TestRegisteringAValidResetSucceedsOnADSTDay(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	dstDay := energy.Bucket(energy.Daily, time.Date(2015, 3, 29, 12, 0, 0, 0, loc), loc)
	require.Equal(t, 23*time.Hour, dstDay.To.Sub(dstDay.From), "2015-03-29 Istanbul must be a 23-hour day")

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, dstDay.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, dstDay.From.Add(10*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, dstDay.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindDaily:   {},
		model.ReadingKindBilling: {},
		model.ReadingKindReset:   {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: dstDay.From, PeriodEnd: dstDay.To,
		Reason: "negative_delta", Detail: f3Detail(t, dstDay, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}}, lock.NewMemory(nil), dstDay.To.Add(consumption.SettleDelayDaily))

	resetTS := dstDay.From.Add(11 * time.Hour)
	resolved, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: dstDay.From, To: dstDay.To},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Suspect)
	require.Equal(t, "130", rows[0].Values[energy.ActiveImport].String())
}

// --- Dedup and override lookups must page past the default 100 -----------

// TestDedupFindsAnExistingRowPastFiveHundredNoiseRows seeds 520 OTHER
// anomalies sharing this analyzer and this exact period_start (different
// period_end, so none of them dedup-match on their own), with the REAL
// matching row seeded LAST. A caller relying on the repository's own
// default page (100) — or even a single page at anomalyPageSize (500) —
// would never see it; listAllAnomalies must page until it does, and
// ConsumptionAndRecord's rerun must NOT create a second row.
func TestDedupFindsAnExistingRowPastFiveHundredNoiseRows(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	const noise = 520
	for i := 0; i < noise; i++ {
		anomalies.seed(model.ConsumptionAnomaly{
			AnalyzerID:  analyzerID,
			PeriodStart: period.From,
			PeriodEnd:   period.From.Add(time.Duration(i+1) * time.Second), // never collides with period.To (3600s away)
			Reason:      "negative_delta",
			Detail:      f2Detail(t, "active_import", "load_profile"),
		})
	}
	real := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}),
		},
	}}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)

	count := 0
	for _, a := range anomalies.rows {
		if a.ID == real.ID || (a.AnalyzerID == analyzerID && a.Reason == "negative_delta" && a.PeriodStart.Equal(period.From) && a.PeriodEnd.Equal(period.To)) {
			count++
		}
	}
	require.Equal(t, 1, count, "the dedup check must find the real row past 520 noise rows, never create a duplicate")
}

// TestOverrideLookupFindsAResolvedRowPastFiveHundredNoiseRows exercises
// Billing.Consumption's own resolved-override lookup directly: 520
// unrelated resolved anomalies share the analyzer and have their own
// period_start scattered WITHIN the single requested Hourly bucket (so the
// Range filter includes them all), none matching the bucket's own exact
// (start,end) — and the real resolved override, at the bucket's own exact
// bounds, is seeded last.
func TestOverrideLookupFindsAResolvedRowPastFiveHundredNoiseRows(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	resolvedAt := h
	res := "accepted"
	const noise = 520
	step := time.Hour / time.Duration(noise+1)
	for i := 0; i < noise; i++ {
		start := period.From.Add(time.Duration(i) * step)
		anomalies.seed(model.ConsumptionAnomaly{
			AnalyzerID:  analyzerID,
			PeriodStart: start,
			PeriodEnd:   start.Add(step),
			Reason:      "negative_delta",
			Detail:      f2Detail(t, "active_import", "load_profile"),
			ResolvedAt:  &resolvedAt,
			Resolution:  &res,
		})
	}
	overrideJSON := []byte(`{"active_import":"999.5"}`)
	realResolvedAt := h
	realRes := "manual_override"
	anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
		ResolvedAt: &realResolvedAt, Resolution: &realRes, OverrideValues: overrideJSON,
	})

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}),
		},
	}}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: period.From, To: period.To},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "999.5", rows[0].Values[energy.ActiveImport].String(), "the resolved override must be found past 520 noise rows")
	require.Equal(t, "manual_override", rows[0].Resolution[energy.ActiveImport])
}

// --- A failed reset insert must never mark the anomaly resolved ----------

// TestAFailedResetInsertNeverResolvesTheAnomaly proves that if BulkInsert
// fails, ResolveAnomaly must return before ever calling
// AnomalyRepository.Resolve.
func TestAFailedResetInsertNeverResolvesTheAnomaly(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{
		byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindLoadProfile: {
				readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
				readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
				readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
			},
			model.ReadingKindReset: {},
		},
		bulkInsertErr: errors.New("boom: simulated write failure"),
	}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour))

	resetTS := h.Add(30 * time.Minute)
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.Error(t, err)

	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt, "a failed insert must never leave the anomaly marked resolved")
}

// --- The dedup key must include period_end, not just period_start --------

// TestDedupKeyRequiresPeriodEndAsWellAsStart proves an Hourly anomaly and a
// Daily anomaly sharing the SAME period_start (both start at midnight) must
// produce TWO separate rows, never one suppressing the other.
func TestDedupKeyRequiresPeriodEndAsWellAsStart(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	istanbul, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	daily := energy.Bucket(energy.Daily, resolveT0, istanbul)
	midnight := daily.From
	hourly := energy.Window{From: midnight, To: midnight.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hourly.From, PeriodEnd: hourly.To,
		Reason: "negative_delta", Detail: f3Detail(t, hourly, "negative_delta", "active_import"),
	})

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {},
		model.ReadingKindDaily:       {},
		model.ReadingKindBilling:     {},
	}}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), midnight.AddDate(0, 0, 1).Add(consumption.SettleDelayDaily))

	// Directly exercise createAnomalyForReason's own dedup key via a second,
	// Daily-shaped suspect row sharing period_start with the Hourly one
	// already seeded above, through ConsumptionAndRecord's own machinery:
	// seed load_profile boundary readings that make the Daily bucket itself
	// suspect (negative delta) too.
	readings.byKind[model.ReadingKindLoadProfile] = []model.MeterReading{
		readingRow(analyzerID, daily.From, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingRow(analyzerID, daily.To, model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}),
	}

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Daily, Range: store.TimeRange{From: daily.From, To: daily.To}}
	_, err = b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)

	var hourlyCount, dailyCount int
	for _, a := range anomalies.rows {
		if a.PeriodStart.Equal(midnight) && a.PeriodEnd.Equal(hourly.To) {
			hourlyCount++
		}
		if a.PeriodStart.Equal(midnight) && a.PeriodEnd.Equal(daily.To) {
			dailyCount++
		}
	}
	require.Equal(t, 1, hourlyCount, "the pre-existing Hourly row must be untouched")
	require.Equal(t, 1, dailyCount, "the Daily row sharing period_start must still be created, never suppressed")
}

// --- The dedup lock must genuinely serialize, not merely "usually work" ---

// raceBarrier holds up to `want` callers until either that many have
// arrived, or `timeout` elapses (whichever first). Under CORRECT locking,
// only one caller is ever inside the barrier at a time (the lock keeps the
// others out), so every call times out alone and the test still passes —
// slower, never wrong. Under a REMOVED lock, both goroutines race into the
// barrier together and it releases immediately, letting both pass the
// dedup check before either creates: the deterministic proof of a missing lock.
func raceBarrier(want int, timeout time.Duration) func() {
	var mu sync.Mutex
	arrived := 0
	release := make(chan struct{})
	var once sync.Once
	return func() {
		mu.Lock()
		arrived++
		n := arrived
		mu.Unlock()
		if n >= want {
			once.Do(func() { close(release) })
		} else {
			select {
			case <-release:
			case <-time.After(timeout):
			}
		}
		// A short, UNCONDITIONAL sleep for every caller, winner and waiter
		// alike: without it, the caller whose own arrival satisfied `want`
		// never blocks at all and Go's scheduler reliably lets it run its
		// ENTIRE dedup-then-create to completion before the other,
		// just-woken caller is scheduled again — which would make even a
		// REMOVED lock look race-free by sheer scheduling luck rather than
		// by correctness. Sleeping here, inside the barrier itself (before
		// either caller's own mutex-protected read), forces both to reach
		// that read at genuinely the same time.
		time.Sleep(2 * time.Millisecond)
	}
}

// dedupRangeBarrier wraps raceBarrier for fakeAnomalies.listBarrier: it
// only engages for the EXACT shape dedupAndCreateAnomaly's own dedup check
// uses — a Range exactly one microsecond wide — never for
// applyResolvedAnomalies' own wider, unlocked List call. Arming the barrier
// unconditionally (on every List call) would fire on that unlocked read
// first, which both goroutines always reach at the same point regardless of
// whether recordSuspectPeriod's own lock later works.
func dedupRangeBarrier(want int, timeout time.Duration) func(store.AnomalyFilter) {
	inner := raceBarrier(want, timeout)
	return func(filt store.AnomalyFilter) {
		if filt.Range != nil && filt.Range.To.Sub(filt.Range.From) == time.Microsecond {
			inner()
		}
	}
}

// TestConcurrentRunsAreSerializedByTheLockNotByLuck holds two goroutines at
// the dedup check (fakeAnomalies.listBarrier, armed only on the dedup
// query's own exact-microsecond shape) AND at the write itself
// (createBarrier) until both have arrived at each gate, which can only
// happen if the lock failed to serialize them. createBarrier is what
// actually forces the double-create: a List-only gate proves both callers
// reached the CHECK together, but Go's scheduler can still let the
// goroutine that never had to block run its entire remaining
// check-then-act to completion before the other, just-woken goroutine
// resumes — gating the WRITE itself is what makes both callers' independent
// "nothing exists yet" decisions already final before either acts on them.
func TestConcurrentRunsAreSerializedByTheLockNotByLuck(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}),
		},
	}}
	anomalies := &fakeAnomalies{
		listBarrier:   dedupRangeBarrier(2, 150*time.Millisecond),
		createBarrier: raceBarrier(2, 150*time.Millisecond),
	}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

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
		if a.AnalyzerID == analyzerID && a.Reason == "negative_delta" {
			count++
		}
	}
	require.Equal(t, 1, count, "the dedup lock must serialize concurrent creation, never create two rows for the same period")
}

// TestConcurrentResolvesAreSerializedByTheAnomalysOwnLock has two goroutines
// call ResolveAnomaly for the SAME anomaly at the same time (accepted vs
// manual_override). Only resolveBarrier gates: a Get-only gate would be
// unnecessary here — arming a barrier on ResolveAnomaly's post-lock re-read
// never distinguishes it from the first, pre-lock existence Get either
// goroutine reaches immediately, so a Get gate alone would let both
// goroutines meet there regardless of whether the lock later works.
// resolveBarrier alone already gates the WRITE itself — the same shape
// TestConcurrentRunsAreSerializedByTheLockNotByLuck uses, for the same
// reason (a read-only gate lets Go's scheduler run the never-blocked
// goroutine's whole check-then-act to completion before the other resumes)
// — and is sufficient: removing ResolveAnomaly's own lock, or removing its
// locked re-read, each independently makes both calls succeed instead of
// one being a conflict.
func TestConcurrentResolvesAreSerializedByTheAnomalysOwnLock(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}

	anomalies := &fakeAnomalies{
		resolveBarrier: raceBarrier(2, 150*time.Millisecond),
	}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})

	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: fakeReadings{}, Anomalies: anomalies, Ops: &fakeOps{},
		Clock: clock.NewFake(resolveT0), Log: testLog(t),
		Locker: lock.NewMemory(nil), Analyzers: fakeAnalyzers{},
		Users: fakeUsers{allowAll: true},
	})
	require.NoError(t, err)

	const n = 2
	var wg sync.WaitGroup
	results := make([]string, n)
	wg.Add(n)
	go func() {
		defer wg.Done()
		_, rerr := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
		switch {
		case rerr == nil:
			results[0] = "ok"
		case errors.Is(rerr, consumption.ErrConflict):
			results[0] = "conflict"
		default:
			results[0] = "error:" + rerr.Error()
		}
	}()
	go func() {
		defer wg.Done()
		_, rerr := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
			Mode:      consumption.ResolveByOverride,
			Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
		})
		switch {
		case rerr == nil:
			results[1] = "ok"
		case errors.Is(rerr, consumption.ErrConflict):
			results[1] = "conflict"
		default:
			results[1] = "error:" + rerr.Error()
		}
	}()
	wg.Wait()

	oks, conflicts := 0, 0
	for _, r := range results {
		switch r {
		case "ok":
			oks++
		case "conflict":
			conflicts++
		default:
			t.Fatalf("unexpected ResolveAnomaly result: %s", r)
		}
	}
	require.Equal(t, 1, oks, "exactly one of two genuinely concurrent resolves must succeed")
	require.Equal(t, 1, conflicts, "the other must see ErrConflict, never silently overwrite the first")
}

// TestResolveAnomalyReReadAndResolveHappenInsideTheLock uses a recorder
// shared between the fakeLocker (refresh_test.go) and fakeAnomalies, for a
// single, sequential ResolveAnomaly call. The recorder's event order proves
// the locked re-read Get and the Resolve call both happen strictly between
// the lock's own acquire and release, observed from a behavioural fake
// rather than an exported test-only hook.
func TestResolveAnomalyReReadAndResolveHappenInsideTheLock(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}

	rec := &recorder{}
	anomalies := &fakeAnomalies{rec: rec}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})

	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: fakeReadings{}, Anomalies: anomalies, Ops: &fakeOps{},
		Clock: clock.NewFake(resolveT0), Log: testLog(t),
		Locker: &fakeLocker{rec: rec}, Analyzers: fakeAnalyzers{},
		Users: fakeUsers{allowAll: true},
	})
	require.NoError(t, err)

	_, err = b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	events := rec.all()
	acquireIdx, releaseIdx := -1, -1
	var getIdxs []int
	resolveIdx := -1
	for i, e := range events {
		switch {
		case strings.HasPrefix(e, "acquire:"):
			acquireIdx = i
		case strings.HasPrefix(e, "release:"):
			releaseIdx = i
		case e == "get":
			getIdxs = append(getIdxs, i)
		case e == "resolve":
			resolveIdx = i
		}
	}

	require.GreaterOrEqual(t, acquireIdx, 0, "the lock must be acquired: %v", events)
	require.Greater(t, releaseIdx, acquireIdx, "the lock must be released after it is acquired: %v", events)
	require.Len(t, getIdxs, 2, "expected exactly two Get calls (pre-lock existence, locked re-read): %v", events)
	require.Less(t, getIdxs[0], acquireIdx, "the first, pre-lock existence Get must happen before the lock is acquired: %v", events)
	require.Greater(t, getIdxs[1], acquireIdx, "the locked re-read Get must happen after the lock is acquired: %v", events)
	require.Less(t, getIdxs[1], releaseIdx, "the locked re-read Get must happen before the lock is released: %v", events)
	require.Greater(t, resolveIdx, acquireIdx, "Resolve must happen after the lock is acquired: %v", events)
	require.Less(t, resolveIdx, releaseIdx, "Resolve must happen before the lock is released: %v", events)
}

// TestNilLockerFailsClosedBeforeConsumptionRuns proves a nil Locker fails
// ConsumptionAndRecord immediately, even for a request whose period turns
// out not to be suspect at all — never silently succeeding until the first
// suspect period happens to appear.
func TestNilLockerFailsClosedBeforeConsumptionRuns(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0

	// A perfectly SOUND period (positive delta, nothing suspect) — under the
	// old behaviour this would succeed even with Locker == nil.
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: readings, Anomalies: anomalies, Ops: &fakeOps{},
		Clock: clock.NewFake(h.Add(time.Hour)), Log: testLog(t),
		Locker: nil, Analyzers: fakeAnalyzers{},
	})
	require.NoError(t, err)

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err = b.ConsumptionAndRecord(ctx, scope, req)
	require.Error(t, err, "a nil Locker must fail ConsumptionAndRecord before Consumption ever runs")
}

// --- BillingDeps.Users is required, checked before any read/write ---------

// TestResolveAnomalyRequiresUsers proves a Billing constructed without
// Users must fail EVERY ResolveAnomaly call, before any read or write — the
// test uses noAnomalies/noReadings/noOps, each of which panics on any call,
// so a panic here would mean the nil check ran too late.
func TestResolveAnomalyRequiresUsers(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: noReadings{}, Anomalies: noAnomalies{}, Ops: noOps{},
		Clock: clock.NewFake(resolveT0), Log: testLog(t),
		Locker: lock.NewMemory(nil), Analyzers: fakeAnalyzers{},
		// Users deliberately left nil.
	})
	require.NoError(t, err)

	_, err = b.ResolveAnomaly(ctx, scope, uuid.New(), uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.Error(t, err, "a nil Users must fail ResolveAnomaly before any read or write")
}

// TestResolvedByMustBeAUserOfTheCompany proves a resolvedBy who is not a
// user of sc.CompanyID is refused BEFORE the reset reading is written.
// Users is now required, so this is the ordinary (always-wired) path.
func TestResolvedByMustBeAUserOfTheCompany(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	notACompanyUser := uuid.New()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: readings, Anomalies: anomalies, Ops: &fakeOps{},
		Clock: clock.NewFake(h.Add(time.Hour)), Log: testLog(t),
		Locker: lock.NewMemory(nil), Analyzers: fakeAnalyzers{analyzer: model.Analyzer{Provider: model.IntegrationProviderOSOS}},
		Users: fakeUsers{validIDs: map[uuid.UUID]bool{}}, // notACompanyUser is NOT in this set
	})
	require.NoError(t, err)

	resetTS := h.Add(30 * time.Minute)
	_, err = b.ResolveAnomaly(ctx, scope, an.ID, notACompanyUser, consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.Error(t, err)
	require.Empty(t, readings.byKind[model.ReadingKindReset], "the reset reading must never be written for a resolver outside the company")

	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt)
}

// --- The F2 cascade runs BEFORE the F3 row is marked resolved -------------

// TestF2CascadeFailureLeavesTheF3RowUnresolved proves that if the F2
// cascade's own Resolve call fails, the F3 row's own Resolve must never be
// reached.
func TestF2CascadeFailureLeavesTheF3RowUnresolved(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}
	nested := energy.Window{From: resolveT0.Add(10 * time.Minute), To: resolveT0.Add(20 * time.Minute)}

	anomalies := &fakeAnomalies{}
	f3 := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})
	f2Row := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: nested.From, PeriodEnd: nested.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})
	anomalies.resolveErr = map[uuid.UUID]error{f2Row.ID: errors.New("boom: simulated cascade failure")}

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	_, err := b.ResolveAnomaly(ctx, scope, f3.ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.Error(t, err)

	got, ok := anomalies.get(f3.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt, "the F3 row must never be marked resolved when the F2 cascade fails")
}

// --- Re-resolving an already-resolved anomaly is a conflict ---------------

// TestReResolvingAnAlreadyResolvedAnomalyIsAConflict proves a second
// resolve attempt must fail with a conflict, never silently overwrite the
// resolver/resolution/timestamp.
func TestReResolvingAnAlreadyResolvedAnomalyIsAConflict(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	hour := energy.Window{From: resolveT0, To: resolveT0.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: hour.From, PeriodEnd: hour.To,
		Reason: "negative_delta", Detail: f3Detail(t, hour, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, fakeReadings{}, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), resolveT0)

	firstResolver := uuid.New()
	_, err := b.ResolveAnomaly(ctx, scope, an.ID, firstResolver, consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)

	secondResolver := uuid.New()
	_, err = b.ResolveAnomaly(ctx, scope, an.ID, secondResolver, consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("42")},
	})
	require.ErrorIs(t, err, consumption.ErrConflict)
	require.ErrorIs(t, err, store.ErrConflict)

	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Equal(t, firstResolver, *got.ResolvedBy, "the first resolver must never be silently overwritten")
	require.Equal(t, "accepted", *got.Resolution)
}

// --- An override recomputes the row's reactive ratios -----------------------

// TestOverrideRecomputesRatios proves that after a manual_override of
// active_import, InductiveRatio/CapacitiveRatio must be computed from the
// override, never left nil from before the substitution.
func TestOverrideRecomputesRatios(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000", "reactive_inductive_import": "100", "reactive_capacitive_import": "50"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900", "reactive_inductive_import": "150", "reactive_capacitive_import": "80"}),
		},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)

	rows, err := b.Consumption(ctx, scope, consumption.SeriesRequest{
		AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: period.From, To: period.To},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "100", rows[0].Values[energy.ActiveImport].String())
	require.NotNil(t, rows[0].InductiveRatio, "the inductive ratio must be recomputed from the override, not left nil")
	require.NotNil(t, rows[0].CapacitiveRatio, "the capacitive ratio must be recomputed from the override, not left nil")
	// reactive_inductive_import derives soundly to 150-100=50, reactive_capacitive_import to 80-50=30;
	// active_import is the override value 100 — ratios are 50/100=0.5 and 30/100=0.3.
	require.True(t, rows[0].InductiveRatio.Equal(decimal.RequireFromString("0.5")), "got %s", rows[0].InductiveRatio.String())
	require.True(t, rows[0].CapacitiveRatio.Equal(decimal.RequireFromString("0.3")), "got %s", rows[0].CapacitiveRatio.String())
}

// --- BulkInsert's upsert must never silently overwrite a provider reset ---

// TestRegisteringAResetAtAnExistingTimestampConflictsOnDifferentValues
// proves an operator reset at a ts a PROVIDER reset already occupies, with
// DIFFERENT values, must be refused as a conflict — never silently
// overwritten by BulkInsert's own upsert semantics.
func TestRegisteringAResetAtAnExistingTimestampConflictsOnDifferentValues(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}
	resetTS := h.Add(30 * time.Minute)

	existingReset := readingRow(analyzerID, resetTS, model.ReadingKindReset, map[string]string{"active_import": "50"})
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {existingReset},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	_, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")}, // DIFFERENT from the existing 50
	})
	require.ErrorIs(t, err, consumption.ErrConflict)
	require.Len(t, readings.byKind[model.ReadingKindReset], 1, "the existing reset must never be overwritten")
	require.Equal(t, "50", readings.byKind[model.ReadingKindReset][0].ActiveImport.String())

	got, ok := anomalies.get(an.ID)
	require.True(t, ok)
	require.Nil(t, got.ResolvedAt)
}

// TestRegisteringAResetAtAnExistingTimestampIsIdempotentOnIdenticalValues
// proves identical values at the same ts are a harmless retry, not a
// conflict.
func TestRegisteringAResetAtAnExistingTimestampIsIdempotentOnIdenticalValues(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}
	resetTS := h.Add(30 * time.Minute)

	existingReset := readingRow(analyzerID, resetTS, model.ReadingKindReset, map[string]string{"active_import": "100"})
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}),
		},
		model.ReadingKindReset: {existingReset},
	}}
	anomalies := &fakeAnomalies{}
	an := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f3Detail(t, period, "negative_delta", "active_import"),
	})

	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	resolved, err := b.ResolveAnomaly(ctx, scope, an.ID, uuid.New(), consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")}, // SAME as existing
	})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
}

// --- An F2 row at exactly a bucket's bounds neither blocks F3 nor ----------
// --- accepts an override ---------------------------------------------------

// TestF2RowAtBucketBoundsNeitherBlocksF3NorAcceptsOverride proves an F2 row
// whose own (period_start, period_end) happen to equal an Hourly bucket's
// exactly must not suppress the F3 row's own creation (the dedup key's
// exact period_end check already covers this), and resolving the F2 row
// itself with manual_override must still fail — F2's detail has no
// registers map to validate against.
func TestF2RowAtBucketBoundsNeitherBlocksF3NorAcceptsOverride(t *testing.T) {
	ctx := context.Background()
	scope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	analyzerID := uuid.New()
	h := resolveT0
	period := energy.Window{From: h, To: h.Add(time.Hour)}

	anomalies := &fakeAnomalies{}
	f2Row := anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: analyzerID, PeriodStart: period.From, PeriodEnd: period.To,
		Reason: "negative_delta", Detail: f2Detail(t, "active_import", "load_profile"),
	})

	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(analyzerID, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(analyzerID, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}),
		},
	}}
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), h.Add(time.Hour).Add(consumption.SettleDelayHourly))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{analyzerID}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	_, err := b.ConsumptionAndRecord(ctx, scope, req)
	require.NoError(t, err)

	var f3Count int
	for _, a := range anomalies.rows {
		if a.ID != f2Row.ID && a.AnalyzerID == analyzerID && a.Reason == "negative_delta" {
			f3Count++
		}
	}
	require.Equal(t, 1, f3Count, "the F2 row at the same bounds must never suppress the F3 row's own creation")

	_, err = b.ResolveAnomaly(ctx, scope, f2Row.ID, uuid.New(), consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("1")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest, "F2's own detail has no registers map to validate an override against")
}
