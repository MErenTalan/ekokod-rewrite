// Package consumption — this file is Task 8's suspect-period write path:
// ConsumptionAndRecord (the only method on Billing that writes), the
// anomaly detail/operator-message shape (R60), and ListAnomalies.
package consumption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ptr returns a pointer to a copy of v — used for OperationalMessage's
// pointer-typed fields (RelatedType), which take no address of a literal.
func ptr[T any](v T) *T { return &v }

// anomalyLockTTL, anomalyLockAcquireWait and anomalyLockAcquireAttempts bound
// the dedup lock ConsumptionAndRecord holds per (analyzer, period_start)
// (C-6): the partial index on consumption_anomalies claims no database-level
// uniqueness, so this lock is what actually prevents two concurrent runs
// from creating two rows for the same period. Acquire itself already polls
// until its ctx is done (platform/lock's contract), so each attempt is given
// its own short, bounded sub-context rather than the caller's own
// (potentially unbounded) ctx directly — a busy key fails an attempt inside
// anomalyLockAcquireWait instead of hanging until the caller's own ctx
// happens to be cancelled. ErrNotAcquired after every attempt is returned as
// a plain error: an anomaly is NEVER created without the lock.
const (
	anomalyLockTTL             = 30 * time.Second
	anomalyLockAcquireWait     = 200 * time.Millisecond
	anomalyLockAcquireAttempts = 5
)

// anomalyDetailCode is R60's fixed "code" field on both the anomaly row's
// detail column and the operator message's Metadata.
const anomalyDetailCode = "consumption.suspect_period"

// anomalyRegisterDetail is one register's entry in anomalyDetail.Registers
// (R60). Delta is omitted (not merely null) for a meter_reset reason, since
// energy.Suspicion.Delta is nil there (M-2) — Reason and ResetRows are
// always present, matching every Suspicion this package ever builds one
// from.
type anomalyRegisterDetail struct {
	Reason    string  `json:"reason"`
	Delta     *string `json:"delta,omitempty"`
	ResetRows int     `json:"reset_rows"`
}

// anomalyDetail is R60's exact detail shape, shared verbatim between a
// consumption_anomalies row's own detail column and the operator message's
// Metadata (R60: "the same detail JSON"). Decimals are strings; no secret,
// no raw payload, no SQL.
type anomalyDetail struct {
	Code        string                           `json:"code"`
	PeriodStart string                           `json:"period_start"`
	PeriodEnd   string                           `json:"period_end"`
	Registers   map[string]anomalyRegisterDetail `json:"registers"`
}

// buildAnomalyDetail renders R60's detail JSON for ONE (analyzer, period,
// reason) anomaly row: regs holds only the registers suspect for THIS
// reason (the caller, groupSuspectByReason, already split a mixed-reason
// period into one call per reason — R58/C-4: one row per reason, not one
// row per register).
func buildAnomalyDetail(period energy.Window, regs map[energy.Register]energy.Suspicion) ([]byte, error) {
	out := anomalyDetail{
		Code:        anomalyDetailCode,
		PeriodStart: period.From.UTC().Format(time.RFC3339),
		PeriodEnd:   period.To.UTC().Format(time.RFC3339),
		Registers:   make(map[string]anomalyRegisterDetail, len(regs)),
	}
	for reg, susp := range regs {
		d := anomalyRegisterDetail{Reason: string(susp.Reason), ResetRows: susp.ResetRows}
		if susp.Delta != nil {
			s := susp.Delta.String()
			d.Delta = &s
		}
		out.Registers[string(reg)] = d
	}
	return json.Marshal(out)
}

// suspectRegistersFromDetail parses an anomaly's own detail JSON (R60's
// shape) back into the set of registers it names — used by ResolveByOverride
// validation (an override must name one of THESE registers) and by
// Billing.Consumption's accepted-resolution substitution (C-6: which
// registers an "accepted" anomaly leaves null).
func suspectRegistersFromDetail(detail json.RawMessage) (map[energy.Register]bool, error) {
	var d anomalyDetail
	if err := json.Unmarshal(detail, &d); err != nil {
		return nil, fmt.Errorf("consumption: decode anomaly detail: %w", err)
	}
	out := make(map[energy.Register]bool, len(d.Registers))
	for reg := range d.Registers {
		out[energy.Register(reg)] = true
	}
	return out, nil
}

// decodeOverrideValues parses a resolved manual_override anomaly's
// OverrideValues column (map[register]decimal-string, ResolveAnomaly's own
// encodeOverrideValues format) back into decimals, for Billing.Consumption's
// substitution (C-6).
func decodeOverrideValues(raw json.RawMessage) (map[energy.Register]decimal.Decimal, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("consumption: decode override values: %w", err)
	}
	out := make(map[energy.Register]decimal.Decimal, len(m))
	for k, v := range m {
		d, err := decimal.NewFromString(v)
		if err != nil {
			return nil, fmt.Errorf("consumption: decode override value for %s: %w", k, err)
		}
		out[energy.Register(k)] = d
	}
	return out, nil
}

// reasonOrder is the deterministic order ConsumptionAndRecord writes one
// anomaly row per distinct reason in, when a period has registers suspect
// for more than one reason. Any reason outside this list (none exist today)
// is appended afterward in map order, never dropped.
var reasonOrder = []energy.Reason{energy.ReasonNegativeDelta, energy.ReasonMeterReset, energy.ReasonMissingReadings}

// groupSuspectByReason splits a Row's mixed-reason Suspect map into one
// sub-map per Reason (R58, C-4: "one row per reason, each listing its
// registers" — never one row per register).
func groupSuspectByReason(suspect map[energy.Register]energy.Suspicion) map[energy.Reason]map[energy.Register]energy.Suspicion {
	out := make(map[energy.Reason]map[energy.Register]energy.Suspicion, len(suspect))
	for reg, susp := range suspect {
		if out[susp.Reason] == nil {
			out[susp.Reason] = make(map[energy.Register]energy.Suspicion)
		}
		out[susp.Reason][reg] = susp
	}
	return out
}

// orderedReasons returns groups' keys in reasonOrder, so ConsumptionAndRecord
// writes anomaly rows (and dedup-checks them) in a stable, deterministic
// order across runs.
func orderedReasons(groups map[energy.Reason]map[energy.Register]energy.Suspicion) []energy.Reason {
	out := make([]energy.Reason, 0, len(groups))
	seen := make(map[energy.Reason]bool, len(groups))
	for _, r := range reasonOrder {
		if _, ok := groups[r]; ok {
			out = append(out, r)
			seen[r] = true
		}
	}
	for r := range groups {
		if !seen[r] {
			out = append(out, r)
		}
	}
	return out
}

// anomalyLockKey is the dedup lock's key: one lock per (analyzer,
// period_start), never per reason — a period with registers suspect for
// several reasons still writes every reason's row under the SAME held
// lease, one check-then-create at a time.
func anomalyLockKey(analyzerID uuid.UUID, periodStart time.Time) string {
	return "consumption.anomaly:" + analyzerID.String() + ":" + periodStart.UTC().Format(time.RFC3339)
}

// acquireAnomalyLock acquires the dedup lease for (analyzerID, periodStart),
// retrying ErrNotAcquired a bounded number of times with a short bounded
// wait per attempt, then failing loudly — an anomaly is NEVER created
// without the lock.
func (b *Billing) acquireAnomalyLock(ctx context.Context, analyzerID uuid.UUID, periodStart time.Time) (lock.Lease, error) {
	if b.deps.Locker == nil {
		return nil, errRequired("BillingDeps.Locker")
	}
	key := anomalyLockKey(analyzerID, periodStart)

	var lastErr error
	for attempt := 0; attempt < anomalyLockAcquireAttempts; attempt++ {
		acquireCtx, cancel := context.WithTimeout(ctx, anomalyLockAcquireWait)
		lease, err := b.deps.Locker.Acquire(acquireCtx, key, anomalyLockTTL)
		cancel()
		if err == nil {
			return lease, nil
		}
		if !errors.Is(err, lock.ErrNotAcquired) {
			return nil, fmt.Errorf("consumption: acquire anomaly lock %q: %w", key, err)
		}
		lastErr = err
	}
	return nil, fmt.Errorf("consumption: could not acquire anomaly lock %q after %d attempts: %w", key, anomalyLockAcquireAttempts, lastErr)
}

// releaseAnomalyLock releases lease, logging (never failing the caller) a
// release error — the lease's TTL bounds the worst case regardless.
func (b *Billing) releaseAnomalyLock(ctx context.Context, lease lock.Lease) {
	if err := lease.Release(ctx); err != nil {
		b.deps.Log.Warn("consumption: release anomaly lock failed", "error", err)
	}
}

// ConsumptionAndRecord runs Consumption and, for every suspect register it
// finds, writes one consumption_anomalies row and one operator message per
// distinct reason in the period (R58-R60, C-4). It is the only method on
// Billing that writes.
//
// Re-running it for the same period does not create a second row for the
// same (analyzer, period_start, period_end, reason) — C-6: the dedup check
// lists BOTH resolved and unresolved anomalies (matching F2's
// negativeDeltaAnomalyExists exactly), serialised per (analyzer,
// period_start) by acquireAnomalyLock, since the table's own partial index
// claims no uniqueness.
func (b *Billing) ConsumptionAndRecord(ctx context.Context, sc store.Scope, req SeriesRequest) ([]Row, error) {
	rows, err := b.Consumption(ctx, sc, req)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if len(row.Suspect) == 0 {
			continue
		}
		if err := b.recordSuspectPeriod(ctx, sc, row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

// recordSuspectPeriod writes one anomaly row (+ operator message) per
// distinct reason in row.Suspect, under ONE held lease for the whole period
// (R58, C-4, C-6).
func (b *Billing) recordSuspectPeriod(ctx context.Context, sc store.Scope, row Row) error {
	groups := groupSuspectByReason(row.Suspect)
	if len(groups) == 0 {
		return nil
	}

	lease, err := b.acquireAnomalyLock(ctx, row.AnalyzerID, row.Window.From)
	if err != nil {
		return err
	}
	defer b.releaseAnomalyLock(ctx, lease)

	for _, reason := range orderedReasons(groups) {
		if err := b.createAnomalyForReason(ctx, sc, row, reason, groups[reason]); err != nil {
			return err
		}
	}
	return nil
}

// createAnomalyForReason implements C-6's dedup-then-create for one
// (analyzer, period, reason): it lists BOTH resolved and unresolved rows
// already covering the exact same period and reason and skips creation when
// one exists, exactly as F2's negativeDeltaAnomalyExists does for its own
// table of anomalies.
func (b *Billing) createAnomalyForReason(ctx context.Context, sc store.Scope, row Row, reason energy.Reason, regs map[energy.Register]energy.Suspicion) error {
	reasonStr := string(reason)

	existing, err := b.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: []uuid.UUID{row.AnalyzerID},
		Reason:      &reasonStr,
		Range:       &store.TimeRange{From: row.Window.From, To: row.Window.To.Add(time.Nanosecond)},
	})
	if err != nil {
		return err
	}
	for _, a := range existing {
		if a.PeriodStart.Equal(row.Window.From) && a.PeriodEnd.Equal(row.Window.To) {
			return nil
		}
	}

	detail, err := buildAnomalyDetail(row.Window, regs)
	if err != nil {
		return err
	}

	created, err := b.deps.Anomalies.Create(ctx, sc, model.ConsumptionAnomaly{
		AnalyzerID:  row.AnalyzerID,
		PeriodStart: row.Window.From,
		PeriodEnd:   row.Window.To,
		Reason:      reasonStr,
		Detail:      detail,
	})
	if err != nil {
		return err
	}

	_, err = b.deps.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID:   ptr(sc.CompanyID),
		Kind:        "system",
		Category:    "consumption-suspect-period",
		Status:      "error",
		Message:     anomalyMessageText(row, reasonStr, regs),
		RelatedType: ptr("consumption_anomaly"),
		RelatedID:   &created.ID,
		Metadata:    detail,
	})
	return err
}

// anomalyMessageText renders R60's "one human-readable sentence".
func anomalyMessageText(row Row, reason string, regs map[energy.Register]energy.Suspicion) string {
	names := make([]string, 0, len(regs))
	for reg := range regs {
		names = append(names, string(reg))
	}
	sort.Strings(names)
	return fmt.Sprintf(
		"Analyzer %s: consumption for %s to %s is suspect (%s) for register(s) %s.",
		row.AnalyzerID, row.Window.From.UTC().Format(time.RFC3339), row.Window.To.UTC().Format(time.RFC3339),
		reason, strings.Join(names, ", "),
	)
}

// anomalyPeriodKey identifies one (period_start, period_end) pair for
// applyResolvedAnomalies' in-memory lookup — a plain UnixNano pair rather
// than two time.Time values, for the same == vs .Equal reason
// analyticsBucketKey documents in analytics.go.
type anomalyPeriodKey struct {
	start, end int64
}

// applyResolvedAnomalies implements C-6: Billing.Consumption itself — not
// only ConsumptionAndRecord — must reflect a resolved manual_override or
// accepted anomaly for a period, so a caller that reads Consumption
// directly still sees the operator's own resolution. It post-processes the
// []Row Consumption already built (mutating it in place; rows is a slice
// over the caller's own backing array) — called from billing.go's
// Consumption with a single line near its return, per the controller's
// note to keep billing.go's own diff minimal ahead of R96's rework there.
//
// Rows are grouped by analyzer so each analyzer's resolved anomalies are
// listed ONCE, over that analyzer's own rows' combined window, rather than
// once per row.
func (b *Billing) applyResolvedAnomalies(ctx context.Context, sc store.Scope, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}

	byAnalyzer := make(map[uuid.UUID][]int, len(rows))
	for i, row := range rows {
		byAnalyzer[row.AnalyzerID] = append(byAnalyzer[row.AnalyzerID], i)
	}

	for analyzerID, idxs := range byAnalyzer {
		from, to := rows[idxs[0]].Window.From, rows[idxs[0]].Window.To
		for _, i := range idxs[1:] {
			if rows[i].Window.From.Before(from) {
				from = rows[i].Window.From
			}
			if rows[i].Window.To.After(to) {
				to = rows[i].Window.To
			}
		}

		anomalies, err := b.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Range:       &store.TimeRange{From: from, To: to.Add(time.Microsecond)},
		})
		if err != nil {
			return err
		}

		byPeriod := make(map[anomalyPeriodKey][]model.ConsumptionAnomaly, len(anomalies))
		for _, a := range anomalies {
			if a.ResolvedAt == nil || a.Resolution == nil {
				continue
			}
			key := anomalyPeriodKey{a.PeriodStart.UnixNano(), a.PeriodEnd.UnixNano()}
			byPeriod[key] = append(byPeriod[key], a)
		}
		if len(byPeriod) == 0 {
			continue
		}

		for _, i := range idxs {
			row := &rows[i]
			key := anomalyPeriodKey{row.Window.From.UnixNano(), row.Window.To.UnixNano()}
			for _, a := range byPeriod[key] {
				if err := applyResolvedAnomaly(row, a); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// applyResolvedAnomaly applies ONE resolved anomaly's effect to row, per the
// resolution recorded on it (C-6). reset_registered needs no application
// here: registering a reset changes meter_readings itself, so the NEXT
// Consumption call derives a sound number the ordinary way, with no
// special-casing in this function.
func applyResolvedAnomaly(row *Row, a model.ConsumptionAnomaly) error {
	if a.Resolution == nil {
		return nil
	}
	switch *a.Resolution {
	case string(ResolveByOverride):
		overrides, err := decodeOverrideValues(a.OverrideValues)
		if err != nil {
			return err
		}
		if row.Values == nil {
			row.Values = make(map[energy.Register]*decimal.Decimal, len(overrides))
		}
		if row.Resolution == nil {
			row.Resolution = make(map[energy.Register]string, len(overrides))
		}
		for reg, v := range overrides {
			val := v
			row.Values[reg] = &val
			row.Resolution[reg] = string(ResolveByOverride)
		}
	case string(ResolveByAccepting):
		regs, err := suspectRegistersFromDetail(a.Detail)
		if err != nil {
			return err
		}
		if row.Resolution == nil {
			row.Resolution = make(map[energy.Register]string, len(regs))
		}
		for reg := range regs {
			if row.Values != nil {
				row.Values[reg] = nil
			}
			row.Resolution[reg] = string(ResolveByAccepting)
		}
	}
	return nil
}

// AnomalyListRequest narrows an anomaly listing (05 §5).
type AnomalyListRequest struct {
	AnalyzerIDs []uuid.UUID
	Unresolved  bool
	Range       *store.TimeRange
	Page        store.Page
}

// ListAnomalies validates sc and req before any I/O (fail-closed): a valid
// Scope and a non-empty AnalyzerIDs, exactly as SeriesRequest requires (R89 —
// an empty AnalyzerIDs is never "every analyzer visible to scope").
func (b *Billing) ListAnomalies(ctx context.Context, sc store.Scope, req AnomalyListRequest) ([]model.ConsumptionAnomaly, error) {
	if !sc.Valid() {
		return nil, ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) == 0 {
		return nil, ErrInvalidRequest
	}
	return b.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: req.AnalyzerIDs,
		Unresolved:  req.Unresolved,
		Range:       req.Range,
		Page:        req.Page,
	})
}
