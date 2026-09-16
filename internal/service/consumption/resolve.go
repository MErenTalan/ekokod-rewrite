// Package consumption — this file is Task 8's operator-resolution path:
// ResolveAnomaly and the three ResolutionMode outcomes 04 §4.4 defines.
package consumption

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ResolutionMode is 04 §4.4's closed set.
type ResolutionMode string

const (
	// ResolveByRegisteringReset writes a kind=reset reading so later
	// derivations apply §3.2's formula instead of reporting the period
	// suspect.
	ResolveByRegisteringReset ResolutionMode = "reset_registered"
	// ResolveByOverride stores operator-supplied values in override_values.
	ResolveByOverride ResolutionMode = "manual_override"
	// ResolveByAccepting records that the operator accepts the NULL. The
	// period stays unbillable-by-value but no longer blocks the invoice.
	ResolveByAccepting ResolutionMode = "accepted"
)

// Resolution is an operator's answer to a suspect period (04 §4.4).
type Resolution struct {
	Mode ResolutionMode
	// ResetTS is required for ResolveByRegisteringReset: the instant the
	// meter was actually reset.
	ResetTS *time.Time
	// ResetAfter is the meter's values after the reset (required for
	// ResolveByRegisteringReset).
	//
	// R93 (supersedes an earlier I-16 draft; R93 reverses I-16 in the
	// domain layer too — see energy.Derive's doc comment): ResetAfter MUST
	// contain a value for EVERY register that the anomaly period's own
	// boundary readings report (read via the billing path's own boundary
	// selection for that period, boundaryReadingsForReset below). A reset
	// row that omits a register both boundaries report makes that register
	// meter_reset-suspect under R93, so an incomplete operator reset would
	// turn previously sound registers suspect. A register missing from
	// ResetAfter here is consumption.ErrInvalidRequest, checked BEFORE any
	// write — never silently accepted as "no reset evidence for it".
	ResetAfter map[energy.Register]decimal.Decimal
	// Overrides is required for ResolveByOverride: every key must be one of
	// the anomaly's own suspect registers (validateOverrideRegisters).
	Overrides map[energy.Register]decimal.Decimal
}

// ResolveAnomaly resolves one consumption_anomalies row (04 §4.4). It also
// resolves any F2 ingestion negative_delta anomaly for the SAME analyzer
// whose [period_start, period_end] lies within this F3 period, applying the
// SAME resolution to both (I-5) — an operator registering a reset for a
// billing period should not separately have to clear the ingestion-time row
// F2 wrote for the same event.
//
// Resolution is scoped: sc is forwarded, unmodified, to every repository
// call. AnomalyRepository.Get/Resolve already return store.ErrNotFound for
// an anomaly id not visible to sc — resolving another tenant's anomaly is
// therefore ErrNotFound, never a permission error that would reveal the
// anomaly's existence.
func (b *Billing) ResolveAnomaly(ctx context.Context, sc store.Scope, id, resolvedBy uuid.UUID, r Resolution) (model.ConsumptionAnomaly, error) {
	if !sc.Valid() || id == uuid.Nil || resolvedBy == uuid.Nil {
		return model.ConsumptionAnomaly{}, ErrInvalidRequest
	}

	anomaly, err := b.deps.Anomalies.Get(ctx, sc, id)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}

	var overridesJSON []byte
	switch r.Mode {
	case ResolveByRegisteringReset:
		if r.ResetTS == nil || len(r.ResetAfter) == 0 {
			return model.ConsumptionAnomaly{}, ErrInvalidRequest
		}
		required, rerr := b.requiredResetRegisters(ctx, sc, anomaly)
		if rerr != nil {
			return model.ConsumptionAnomaly{}, rerr
		}
		for reg := range required {
			if _, ok := r.ResetAfter[reg]; !ok {
				// R93: an incomplete reset row would make a previously
				// sound register suspect — refused before any write.
				return model.ConsumptionAnomaly{}, ErrInvalidRequest
			}
		}

		reading, berr := b.buildResetReading(ctx, sc, anomaly.AnalyzerID, *r.ResetTS, r.ResetAfter)
		if berr != nil {
			return model.ConsumptionAnomaly{}, berr
		}
		// If this insert fails, the anomaly is NOT marked resolved (this
		// return happens before AnomalyRepository.Resolve is ever called).
		if _, _, ierr := b.deps.Readings.BulkInsert(ctx, sc, []model.MeterReading{reading}); ierr != nil {
			return model.ConsumptionAnomaly{}, ierr
		}

	case ResolveByOverride:
		if verr := validateOverrideRegisters(anomaly, r.Overrides); verr != nil {
			return model.ConsumptionAnomaly{}, verr
		}
		overridesJSON, err = encodeOverrideValues(r.Overrides)
		if err != nil {
			return model.ConsumptionAnomaly{}, err
		}

	case ResolveByAccepting:
		// Nothing to validate or write beyond the Resolve call below: the
		// period stays unbillable-by-value (Values still nil) but no longer
		// blocks the invoice.

	default:
		return model.ConsumptionAnomaly{}, ErrInvalidRequest
	}

	at := b.deps.Clock.Now()
	resolved, err := b.deps.Anomalies.Resolve(ctx, sc, id, resolvedBy, string(r.Mode), overridesJSON, at)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}

	if err := b.resolveOverlappingIngestionAnomalies(ctx, sc, anomaly, resolvedBy, string(r.Mode), overridesJSON, at); err != nil {
		return model.ConsumptionAnomaly{}, err
	}

	return resolved, nil
}

// validateOverrideRegisters implements "an unknown register is
// ErrInvalidRequest": every key of overrides must be one of the anomaly's
// own suspect registers, read from its own detail JSON (R60's shape).
func validateOverrideRegisters(anomaly model.ConsumptionAnomaly, overrides map[energy.Register]decimal.Decimal) error {
	if len(overrides) == 0 {
		return ErrInvalidRequest
	}
	suspect, err := suspectRegistersFromDetail(anomaly.Detail)
	if err != nil {
		return err
	}
	for reg := range overrides {
		if !suspect[reg] {
			return ErrInvalidRequest
		}
	}
	return nil
}

// encodeOverrideValues renders overrides as the OverrideValues column's
// stored shape: map[register]decimal-string, decodeOverrideValues' inverse
// (anomalies.go).
func encodeOverrideValues(overrides map[energy.Register]decimal.Decimal) ([]byte, error) {
	m := make(map[string]string, len(overrides))
	for reg, v := range overrides {
		m[string(reg)] = v.String()
	}
	return json.Marshal(m)
}

// requiredResetRegisters implements R93: the set of registers the anomaly
// period's OWN boundary readings both report, via the billing path's own
// boundary selection (boundaryReadingsForReset) — never a plain "every
// register that ever appears anywhere".
func (b *Billing) requiredResetRegisters(ctx context.Context, sc store.Scope, anomaly model.ConsumptionAnomaly) (map[energy.Register]bool, error) {
	period := energy.Window{From: anomaly.PeriodStart, To: anomaly.PeriodEnd}
	start, end, err := b.boundaryReadingsForReset(ctx, sc, anomaly.AnalyzerID, period)
	if err != nil {
		return nil, err
	}
	return registersReportedByBoth(start, end), nil
}

// registersReportedByBoth returns the registers BOTH start and end report a
// non-nil value for (R93's "every register that the anomaly period's
// boundary readings report").
func registersReportedByBoth(start, end *energy.Reading) map[energy.Register]bool {
	out := make(map[energy.Register]bool)
	if start == nil || end == nil {
		return out
	}
	for _, reg := range energy.AllRegisters() {
		if start.Value(reg) != nil && end.Value(reg) != nil {
			out[reg] = true
		}
	}
	return out
}

// inferLevel recovers the energy.Level a (start, end) period was originally
// bucketed at: every period this package ever creates an anomaly for is
// exactly one energy.Buckets() window, so trying each of the four levels'
// own whole-bucket window at period.From and matching it against period
// exactly recovers the level with no ambiguity — ConsumptionAnomaly has no
// Level column to read it back from directly (04 §4.4 defines none).
func inferLevel(period energy.Window, loc *time.Location) (energy.Level, bool) {
	for _, lvl := range []energy.Level{energy.Hourly, energy.Daily, energy.Monthly, energy.Yearly} {
		b := energy.Bucket(lvl, period.From, loc)
		if b.From.Equal(period.From) && b.To.Equal(period.To) {
			return lvl, true
		}
	}
	return "", false
}

// boundaryReadingsForReset determines the two boundary readings the billing
// path's own selection (selectBoundarySource, billing.go — same package,
// reused rather than duplicated) would use for period, for R93's
// completeness check. It mirrors consumptionForAnalyzer's per-window
// selection but loads readings for exactly the one window an operator is
// resolving, not a whole request.
func (b *Billing) boundaryReadingsForReset(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, period energy.Window) (start, end *energy.Reading, err error) {
	level, ok := inferLevel(period, istanbul)
	if !ok {
		// Should not happen for a period this package itself created —
		// fall back to the narrowest, safest source rather than fail.
		level = energy.Hourly
	}

	lpLookback, err := b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindLoadProfile, period)
	if err != nil {
		return nil, nil, err
	}
	minLookback := lpLookback

	useBilling := level == energy.Monthly
	var billingLookback time.Time
	if useBilling {
		billingLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindBilling, period)
		if err != nil {
			return nil, nil, err
		}
		if billingLookback.Before(minLookback) {
			minLookback = billingLookback
		}
	}

	useDaily := level != energy.Hourly
	var dailyLookback time.Time
	if useDaily {
		dailyLookback, err = b.firstBoundaryLookback(ctx, sc, analyzerID, model.ReadingKindDaily, period)
		if err != nil {
			return nil, nil, err
		}
		if dailyLookback.Before(minLookback) {
			minLookback = dailyLookback
		}
	}

	rangeTo := period.To.Add(time.Microsecond)

	lpRows, err := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: minLookback, To: rangeTo}, model.ReadingKindLoadProfile)
	if err != nil {
		return nil, nil, err
	}
	loadProfile := toEnergyReadings(lpRows)

	var billing []energy.Reading
	if useBilling {
		billingRows, berr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: billingLookback, To: rangeTo}, model.ReadingKindBilling)
		if berr != nil {
			return nil, nil, berr
		}
		billing = toEnergyReadings(billingRows)
	}

	var daily []energy.Reading
	if useDaily {
		dailyRows, derr := b.deps.Readings.Range(ctx, sc, analyzerID, store.TimeRange{From: dailyLookback, To: rangeTo}, model.ReadingKindDaily)
		if derr != nil {
			return nil, nil, derr
		}
		daily = toEnergyReadings(dailyRows)
	}

	boundaryReadings := selectBoundarySource(level, period, loadProfile, billing, daily)
	start = energy.SelectBoundary(boundaryReadings, period.From)
	end = energy.SelectBoundary(boundaryReadings, period.To)
	return start, end, nil
}

// buildResetReading builds the kind=reset row ResolveByRegisteringReset
// writes: SourceProvider is the analyzer's own provider (BillingDeps.
// Analyzers), and MultiplierApplied is fixed at 1 — an operator-entered
// value is already real, never re-multiplied (Global Constraints: the
// multiplier is applied once, at ingestion).
func (b *Billing) buildResetReading(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, resetTS time.Time, after map[energy.Register]decimal.Decimal) (model.MeterReading, error) {
	if b.deps.Analyzers == nil {
		return model.MeterReading{}, errRequired("BillingDeps.Analyzers")
	}
	analyzer, err := b.deps.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return model.MeterReading{}, err
	}

	reading := model.MeterReading{
		AnalyzerID:        analyzerID,
		Ts:                resetTS,
		Kind:              model.ReadingKindReset,
		MultiplierApplied: decimal.NewFromInt(1),
		SourceProvider:    analyzer.Provider,
		IngestedAt:        b.deps.Clock.Now(),
	}
	for reg, v := range after {
		val := v
		setRegisterOnReading(&reading, reg, &val)
	}
	return reading, nil
}

// setRegisterOnReading is toEnergyReading's inverse for one register
// (convert.go maps model.MeterReading -> energy.Reading; nothing in that
// file goes the other way, since Task 7 never needed to).
func setRegisterOnReading(r *model.MeterReading, reg energy.Register, v *decimal.Decimal) {
	switch reg {
	case energy.ActiveImport:
		r.ActiveImport = v
	case energy.ReactiveInductiveImport:
		r.ReactiveInductiveImport = v
	case energy.ReactiveCapacitiveImport:
		r.ReactiveCapacitiveImport = v
	case energy.T1Import:
		r.T1Import = v
	case energy.T2Import:
		r.T2Import = v
	case energy.T3Import:
		r.T3Import = v
	case energy.ActiveExport:
		r.ActiveExport = v
	case energy.ReactiveInductiveExport:
		r.ReactiveInductiveExport = v
	case energy.ReactiveCapacitiveExport:
		r.ReactiveCapacitiveExport = v
	case energy.T1Export:
		r.T1Export = v
	case energy.T2Export:
		r.T2Export = v
	case energy.T3Export:
		r.T3Export = v
	}
}

// resolveOverlappingIngestionAnomalies implements I-5: every UNRESOLVED
// negative_delta anomaly for the SAME analyzer (never another one) whose
// period lies within [anomaly.PeriodStart, anomaly.PeriodEnd] is resolved
// with the SAME resolution, resolver and timestamp as the F3 row just
// resolved above. It never touches another reason's rows (the Reason filter)
// or the F3 row itself (the id check).
func (b *Billing) resolveOverlappingIngestionAnomalies(ctx context.Context, sc store.Scope, anomaly model.ConsumptionAnomaly, resolvedBy uuid.UUID, resolution string, overrides []byte, at time.Time) error {
	reason := string(energy.ReasonNegativeDelta)
	candidates, err := b.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: []uuid.UUID{anomaly.AnalyzerID},
		Reason:      &reason,
		Unresolved:  true,
		Range:       &store.TimeRange{From: anomaly.PeriodStart, To: anomaly.PeriodEnd.Add(time.Nanosecond)},
	})
	if err != nil {
		return err
	}

	for _, c := range candidates {
		if c.ID == anomaly.ID {
			continue
		}
		if c.PeriodStart.Before(anomaly.PeriodStart) || c.PeriodEnd.After(anomaly.PeriodEnd) {
			continue
		}
		if _, err := b.deps.Anomalies.Resolve(ctx, sc, c.ID, resolvedBy, resolution, overrides, at); err != nil {
			return err
		}
	}
	return nil
}
