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
	// meter was actually reset. period.From < ResetTS <= period.To is
	// required before any write — R90's equality rule governs ResetTS ==
	// period.To (handled by energy.Derive itself once the proposed reset is
	// merged in for re-derivation, not by a separate check here).
	ResetTS *time.Time
	// ResetAfter is the meter's values after the reset (required for
	// ResolveByRegisteringReset).
	//
	// ResetAfter is never checked against a separately-computed "required
	// registers" set. Instead the proposed reset is merged into the
	// anomaly's own resets and the WHOLE period is re-derived, in memory,
	// through Billing's own R96 boundary resolution
	// (rederiveBucketWithReset, billing.go's deriveRow) — never a second,
	// independently-maintained completeness check that could disagree with
	// R93. A register ResetAfter omits, when both the period's boundaries
	// report it, comes back meter_reset-suspect from that re-derivation
	// exactly as R93 requires — and ResolveAnomaly then refuses the whole
	// request because the re-derivation left ANY register suspect (never
	// only the ones this anomaly originally named).
	ResetAfter map[energy.Register]decimal.Decimal
	// Overrides is required for ResolveByOverride: every key must be one of
	// the anomaly's own suspect registers (validateOverrideRegisters).
	Overrides map[energy.Register]decimal.Decimal
}

// ResolveAnomaly resolves one consumption_anomalies row (04 §4.4). It also
// resolves any F2 ingestion negative_delta anomaly for the SAME analyzer
// whose [period_start, period_end] lies WITHIN this F3 period, applying the
// SAME resolution to both — an operator registering a reset for a billing
// period should not separately have to clear the ingestion-time row F2
// wrote for the same event. This cascade is restricted to F2-SHAPED rows
// only (resolveOverlappingIngestionAnomalies below); it never touches
// another F3 row and never copies override_values onto a cascaded row.
//
// Resolution is scoped: sc is forwarded, unmodified, to every repository
// call. AnomalyRepository.Get/Resolve already return store.ErrNotFound for
// an anomaly id not visible to sc — resolving another tenant's anomaly is
// therefore ErrNotFound, never a permission error that would reveal the
// anomaly's existence.
//
// Ordering: every validation and authorization check runs BEFORE any write,
// in this order — BillingDeps.Users presence (at the very top, before any
// read at all), Analyzers dependency presence (only when the mode needs
// it), the anomaly's own existence/scope (the first Get), the anomaly's own
// lock (same key scheme as recording), a SECOND Get inside that lock,
// already-resolved conflict on that re-read, resolvedBy's own company
// membership, the Hourly-gap assertion (R103 point 1), then the
// mode-specific checks. Writes happen in this order: (1) the reset reading,
// if any; (2) the F2 cascade (so a resolved F3 row below always means the
// cascade already finished); (3) the F3 row's own Resolve call, last.
func (b *Billing) ResolveAnomaly(ctx context.Context, sc store.Scope, id, resolvedBy uuid.UUID, r Resolution) (model.ConsumptionAnomaly, error) {
	if !sc.Valid() || id == uuid.Nil || resolvedBy == uuid.Nil {
		return model.ConsumptionAnomaly{}, ErrInvalidRequest
	}
	// BillingDeps.Users is required for ResolveAnomaly (never optional) and
	// validated before any read or write — a Billing wired without it must
	// fail every ResolveAnomaly call, never silently skip the resolvedBy
	// membership check the way a nil Users would otherwise allow.
	if b.deps.Users == nil {
		return model.ConsumptionAnomaly{}, errRequired("BillingDeps.Users")
	}
	// BillingDeps.Analyzers is checked at the top, before any read, when the
	// mode will need it — never discovered deep inside buildResetReading
	// after several reads have already happened.
	if r.Mode == ResolveByRegisteringReset && b.deps.Analyzers == nil {
		return model.ConsumptionAnomaly{}, errRequired("BillingDeps.Analyzers")
	}

	// First read: only to learn the anomaly's own (analyzerID, periodStart)
	// for the lock key below — everything that decides whether this call
	// may proceed must be re-checked INSIDE the lock, since this first read
	// can be arbitrarily stale by the time the lock is held.
	anomaly, err := b.deps.Anomalies.Get(ctx, sc, id)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}

	// Take the SAME per-(analyzer, period_start) lease acquireAnomalyLock
	// uses for recording, then re-read the anomaly INSIDE it. Two
	// concurrent ResolveAnomaly calls for the same anomaly now genuinely
	// serialize: whichever loses the race sees ResolvedAt already set on
	// its own re-read and returns ErrConflict, instead of both racing the
	// check-then-act above the lock and both succeeding.
	lease, err := b.acquireAnomalyLock(ctx, anomaly.AnalyzerID, anomaly.PeriodStart)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}
	defer b.releaseAnomalyLock(ctx, lease)

	anomaly, err = b.deps.Anomalies.Get(ctx, sc, id)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}
	// Re-resolving an already-resolved anomaly is a conflict, never a
	// silent overwrite of its resolver/resolution/timestamp.
	if anomaly.ResolvedAt != nil {
		return model.ConsumptionAnomaly{}, ErrConflict
	}
	// resolvedBy's own company membership is validated BEFORE any write,
	// not discovered only by AnomalyRepository.Resolve's own SQL check
	// after a reset reading has already been written.
	if _, err := b.deps.Users.Get(ctx, sc, resolvedBy); err != nil {
		return model.ConsumptionAnomaly{}, err
	}

	// R103 point 1: an Hourly missing_readings ("gap") row can no longer be
	// created at all (createMissingReadingsAnomaly only ever runs at
	// Daily/Monthly/Yearly), so no such row can exist here today. This
	// assertion is defence in depth, not a live case: it closes the door on
	// one reaching ResolveAnomaly some other way (a stale row from before
	// R103, a direct repository write, a future regression in the writer)
	// — resolving it by override OR acceptance is refused outright, which
	// is stricter than R103(1) itself needs to say, since the ruling only
	// has to rule out a row that cannot yet exist. This is a deliberate
	// choice to refuse `accepted` too, not an oversight — a row this
	// defence ever catches is itself a bug, so there is no legitimate path
	// that needs `accepted` to close it.
	if anomaly.Reason == string(energy.ReasonMissingReadings) && r.Mode != ResolveByRegisteringReset {
		if lvl, ok := inferLevel(energy.Window{From: anomaly.PeriodStart, To: anomaly.PeriodEnd}, istanbul); ok && lvl == energy.Hourly {
			return model.ConsumptionAnomaly{}, ErrInvalidRequest
		}
	}

	var overridesJSON []byte
	switch r.Mode {
	case ResolveByRegisteringReset:
		// R97 rule 6: a missing_readings anomaly has no meter to reset —
		// there is no boundary reading to be wrong, only readings that never
		// arrived. This also covers R103 point 1's Hourly gap case: every
		// missing_readings anomaly is rejected here regardless of level,
		// before any read or write.
		if anomaly.Reason == string(energy.ReasonMissingReadings) {
			return model.ConsumptionAnomaly{}, ErrInvalidRequest
		}
		if err := b.validateAndPrepareReset(ctx, sc, anomaly, r); err != nil {
			return model.ConsumptionAnomaly{}, err
		}

	case ResolveByOverride:
		if anomaly.Reason == string(energy.ReasonMissingReadings) {
			// R97 rule 6: manual_override accepts ANY register — there is
			// no suspect-register set to validate against, since a
			// missing_readings anomaly's own detail carries no registers.
			// Every key must still be a real register.
			if len(r.Overrides) == 0 || !allRegistersValid(r.Overrides) {
				return model.ConsumptionAnomaly{}, ErrInvalidRequest
			}
		} else if verr := validateOverrideRegisters(anomaly, r.Overrides); verr != nil {
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

	// The F2 cascade runs BEFORE the F3 row itself is marked resolved, so a
	// resolved F3 row is proof the cascade already finished — never the
	// other way around.
	at := b.deps.Clock.Now()
	if anomaly.Reason != string(energy.ReasonMissingReadings) {
		// R97 rule 6: no F2 cascade for a missing_readings resolution.
		if err := b.resolveOverlappingIngestionAnomalies(ctx, sc, anomaly, resolvedBy, string(r.Mode), at); err != nil {
			return model.ConsumptionAnomaly{}, err
		}
	}

	resolved, err := b.deps.Anomalies.Resolve(ctx, sc, id, resolvedBy, string(r.Mode), overridesJSON, at)
	if err != nil {
		return model.ConsumptionAnomaly{}, err
	}
	return resolved, nil
}

// validateAndPrepareReset, before any write, rebuilds the
// period's own boundaries and re-derives the WHOLE window in memory, through
// Billing's own R96 resolver, with the proposed reset merged into the
// resets slice — never a second, independently-maintained boundary
// selection. It then inserts the actual reset reading once that
// re-derivation comes back clean. Nothing is written if any check fails.
func (b *Billing) validateAndPrepareReset(ctx context.Context, sc store.Scope, anomaly model.ConsumptionAnomaly, r Resolution) error {
	if r.ResetTS == nil || len(r.ResetAfter) == 0 {
		return ErrInvalidRequest
	}
	// Every key must be a real register — setRegisterOnReading silently
	// drops an unrecognised one, which would otherwise let a typo'd key
	// through as if it carried no reset evidence at all.
	if !allRegistersValid(r.ResetAfter) {
		return ErrInvalidRequest
	}
	period := energy.Window{From: anomaly.PeriodStart, To: anomaly.PeriodEnd}
	// period.From < ResetTS <= period.To.
	if !r.ResetTS.After(period.From) || r.ResetTS.After(period.To) {
		return ErrInvalidRequest
	}

	proposed := energy.Reading{TS: *r.ResetTS, Kind: energy.KindReset, Values: make(map[energy.Register]*decimal.Decimal, len(r.ResetAfter))}
	for reg, v := range r.ResetAfter {
		val := v
		proposed.Values[reg] = &val
	}

	row, ok, err := b.rederiveBucketWithReset(ctx, sc, anomaly.AnalyzerID, period, proposed)
	if err != nil {
		return err
	}
	// The re-derivation must emit a row with NO suspect register — a
	// partial or misplaced reset leaves at least one register suspect and
	// is rejected outright, never partially applied.
	if !ok || len(row.Suspect) != 0 {
		return ErrInvalidRequest
	}

	// BulkInsert is an upsert on (analyzer_id, ts, kind) — an operator
	// reset at a ts a provider reset already occupies must never silently
	// overwrite it. Identical values are an idempotent retry (allowed);
	// different values are a conflict.
	existingRows, err := b.deps.Readings.Range(ctx, sc, anomaly.AnalyzerID, store.TimeRange{From: *r.ResetTS, To: r.ResetTS.Add(time.Microsecond)}, model.ReadingKindReset)
	if err != nil {
		return err
	}
	for _, ex := range existingRows {
		if ex.Ts.Equal(*r.ResetTS) && !resetValuesEqual(ex, r.ResetAfter) {
			return ErrConflict
		}
	}

	reading, berr := b.buildResetReading(ctx, sc, anomaly.AnalyzerID, *r.ResetTS, r.ResetAfter)
	if berr != nil {
		return berr
	}
	// If this insert fails, the anomaly is NOT marked resolved: this
	// function returns before ResolveAnomaly ever calls
	// AnomalyRepository.Resolve.
	if _, _, ierr := b.deps.Readings.BulkInsert(ctx, sc, []model.MeterReading{reading}); ierr != nil {
		return ierr
	}
	return nil
}

// resetValuesEqual is the "identical values" check: every register
// in after must equal existing's own value for that register exactly
// (decimal.Equal), and existing must report no OTHER register beyond after
// — anything else is a genuine conflict, not an idempotent retry.
func resetValuesEqual(existing model.MeterReading, after map[energy.Register]decimal.Decimal) bool {
	ex := toEnergyReading(existing)
	seen := make(map[energy.Register]bool, len(after))
	for reg, v := range after {
		seen[reg] = true
		ev := ex.Value(reg)
		if ev == nil || !ev.Equal(v) {
			return false
		}
	}
	for _, reg := range energy.AllRegisters() {
		if seen[reg] {
			continue
		}
		if ex.Value(reg) != nil {
			return false
		}
	}
	return true
}

// rederiveBucketWithReset is the in-memory re-derivation: it loads the SAME
// data loadAnalyzerBoundaryData would load for a one-bucket request over
// period, then derives that one bucket with proposed merged into the
// resets slice — reusing deriveRow (billing.go), never a second boundary
// selection. resetRederivationMode recovers how the period was derived; a
// period it does not recognise is ErrInvalidRequest.
func (b *Billing) rederiveBucketWithReset(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, period energy.Window, proposed energy.Reading) (Row, bool, error) {
	mode, ok := resetRederivationMode(period)
	if !ok {
		return Row{}, false, ErrInvalidRequest
	}
	data, err := b.loadAnalyzerBoundaryData(ctx, sc, analyzerID, mode, []energy.Window{period})
	if err != nil {
		return Row{}, false, err
	}
	row, ok := deriveRow(analyzerID, period, data, []energy.Reading{proposed}, mode.level)
	return row, ok, nil
}

// resetRederivationMode is inferLevel's calendar bucket, else explicit-window
// semantics for a cut-off period (R107). Cut-off periods run between Istanbul
// midnights, so any other non-bucket period stays rejected.
func resetRederivationMode(period energy.Window) (windowMode, bool) {
	if level, ok := inferLevel(period, istanbul); ok {
		return windowMode{level: level}, true
	}
	if !isDayStart(period.From) || !isDayStart(period.To) || validPeriodWindow(period) != nil {
		return windowMode{}, false
	}
	return windowMode{level: energy.Monthly, explicit: true}, true
}

// isDayStart reports whether ts is an Istanbul midnight.
func isDayStart(ts time.Time) bool {
	return energy.Bucket(energy.Daily, ts, istanbul).From.Equal(ts)
}

// allRegistersValid reports whether every key of m is one of
// energy.AllRegisters() — setRegisterOnReading (below) silently drops any
// other key, which would otherwise let a typo'd register name through as if
// it simply carried no evidence at all.
func allRegistersValid[V any](m map[energy.Register]V) bool {
	valid := make(map[energy.Register]bool, len(energy.AllRegisters()))
	for _, reg := range energy.AllRegisters() {
		valid[reg] = true
	}
	for reg := range m {
		if !valid[reg] {
			return false
		}
	}
	return true
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

// inferLevel recovers the energy.Level a (start, end) period was originally
// bucketed at: every period this package ever creates an anomaly for is
// exactly one energy.Buckets() window, so trying each of the four levels'
// own whole-bucket window at period.From and matching it against period
// exactly recovers the level with no ambiguity — ConsumptionAnomaly has no
// Level column to read it back from directly (04 §4.4 defines none). A
// period that is not exactly one whole bucket at any level is the
// "not a bucket" case: the caller treats !ok as ErrInvalidRequest rather
// than silently guessing Hourly.
func inferLevel(period energy.Window, loc *time.Location) (energy.Level, bool) {
	for _, lvl := range []energy.Level{energy.Hourly, energy.Daily, energy.Monthly, energy.Yearly} {
		b := energy.Bucket(lvl, period.From, loc)
		if b.From.Equal(period.From) && b.To.Equal(period.To) {
			return lvl, true
		}
	}
	return "", false
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

// resolveOverlappingIngestionAnomalies resolves every UNRESOLVED F2-SHAPED
// negative_delta anomaly for the SAME analyzer whose period lies FULLY
// WITHIN [anomaly.PeriodStart, anomaly.PeriodEnd] with the SAME resolution
// and resolver as the F3 row being resolved — but NEVER with
// override_values copied onto it (the cascaded row's own OverrideValues
// column is always nil, whatever the F3 resolution carries). It is
// F2-shaped exactly when: reason ==
// negative_delta AND the detail decodes to F2's own shape
// ({"register","kind"}, no "code" field) — never an F3 row (reason
// negative_delta but detail.code == "consumption.suspect_period") and never
// a row of any other reason. It never touches another reason's rows (the
// Reason filter), another analyzer's rows (the AnalyzerIDs filter), or the
// F3 row itself (the id check).
func (b *Billing) resolveOverlappingIngestionAnomalies(ctx context.Context, sc store.Scope, anomaly model.ConsumptionAnomaly, resolvedBy uuid.UUID, resolution string, at time.Time) error {
	reason := string(energy.ReasonNegativeDelta)
	candidates, err := b.listAllAnomalies(ctx, sc, store.AnomalyFilter{
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
		// FULL containment, never mere overlap.
		if c.PeriodStart.Before(anomaly.PeriodStart) || c.PeriodEnd.After(anomaly.PeriodEnd) {
			continue
		}
		if !isF2ShapedDetail(c.Detail) {
			continue
		}
		// override_values is NEVER copied onto a cascaded row, whatever the
		// F3 resolution's own overrides carry.
		if _, err := b.deps.Anomalies.Resolve(ctx, sc, c.ID, resolvedBy, resolution, nil, at); err != nil {
			return err
		}
	}
	return nil
}

// f2AnomalyDetail is F2's own detail shape (internal/ingest/anomaly.go,
// R59): one row per affected register, {"register":"active_import",
// "kind":"load_profile"} — no "code" field, unlike F3's anomalyDetail
// (anomalies.go). isF2ShapedDetail distinguishes the two so the cascade
// never touches an F3 row (Hourly/Daily/Monthly/Yearly rows sharing the
// negative_delta reason and, on occasion, a bucket-aligned (start,end)
// pair with an F2 row).
type f2AnomalyDetail struct {
	Register string `json:"register"`
	Kind     string `json:"kind"`
	Code     string `json:"code,omitempty"`
}

// isF2ShapedDetail reports whether detail decodes to F2's own shape: both
// "register" and "kind" present, and NO "code" field (F3's own detail
// always carries "code":"consumption.suspect_period" — R60).
func isF2ShapedDetail(detail json.RawMessage) bool {
	var d f2AnomalyDetail
	if err := json.Unmarshal(detail, &d); err != nil {
		return false
	}
	return d.Register != "" && d.Kind != "" && d.Code == ""
}
