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
	Registers   map[string]anomalyRegisterDetail `json:"registers,omitempty"`
	// Boundaries is R97's own addition: which side(s) of a bucket ("start",
	// "end", or both) produced no row at all. It is mutually exclusive with
	// Registers — a missing_readings row never has a register map (there is
	// no derived value to blame a register for), and a negative_delta/
	// meter_reset row never has Boundaries.
	Boundaries []string `json:"boundaries,omitempty"`
}

// anomalyCode is used to peek at just the "code" field of an anomaly's
// detail JSON, without decoding the rest — C1(c): Consumption's override
// substitution (applyResolvedAnomalies) must never apply an F2-shaped row's
// (or any other unrecognised shape's) values, only a row this package
// itself wrote with R60's own "code":"consumption.suspect_period".
type anomalyCode struct {
	Code string `json:"code"`
}

// hasSuspectPeriodCode reports whether detail decodes with R60's own code —
// true for both a register-reasoned row (buildAnomalyDetail) and an R97
// missing_readings row (buildMissingReadingsDetail), false for F2's own
// shape ({"register","kind"}, no "code" at all) or anything unparseable.
func hasSuspectPeriodCode(detail json.RawMessage) bool {
	var d anomalyCode
	if err := json.Unmarshal(detail, &d); err != nil {
		return false
	}
	return d.Code == anomalyDetailCode
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

// buildMissingReadingsDetail renders R97's own detail shape: R60's code,
// period bounds, and "boundaries" naming the unresolved side(s) — no
// "registers" key at all (there is no derived value to attribute to one).
func buildMissingReadingsDetail(period energy.Window, boundaries []string) ([]byte, error) {
	out := anomalyDetail{
		Code:        anomalyDetailCode,
		PeriodStart: period.From.UTC().Format(time.RFC3339),
		PeriodEnd:   period.To.UTC().Format(time.RFC3339),
		Boundaries:  boundaries,
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
	// I1: Locker's presence is checked up front, before Consumption runs at
	// all — never discovered only once the first suspect period appears. A
	// mis-wired Billing (Locker left nil) must fail every
	// ConsumptionAndRecord call, not just the ones lucky enough to hit a
	// suspect period.
	if b.deps.Locker == nil {
		return nil, errRequired("BillingDeps.Locker")
	}
	rows, gapsByAnalyzer, err := b.consumptionWithGaps(ctx, sc, req)
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
	// R97: one missing_readings anomaly per requested bucket that produced
	// no row (and was not already resolved into a row by
	// applyResolvedGapOverrides, billing.go — a bucket is either a row or a
	// gap, never both).
	for analyzerID, gaps := range gapsByAnalyzer {
		for _, gap := range gaps {
			if err := b.recordMissingReadingsGap(ctx, sc, analyzerID, gap); err != nil {
				return nil, err
			}
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
// (analyzer, period, reason): register-shaped rows (negative_delta,
// meter_reset) — R97's missing_readings goes through
// createMissingReadingsAnomaly, sharing the same dedupAndCreateAnomaly core.
func (b *Billing) createAnomalyForReason(ctx context.Context, sc store.Scope, row Row, reason energy.Reason, regs map[energy.Register]energy.Suspicion) error {
	detail, err := buildAnomalyDetail(row.Window, regs)
	if err != nil {
		return err
	}
	message := anomalyMessageText(row, string(reason), regs)
	return b.dedupAndCreateAnomaly(ctx, sc, row.AnalyzerID, row.Window, string(reason), detail, message)
}

// anomalyPageSize bounds every page listAllAnomalies requests: comfortably
// under AnomalyRepository's own maximum page cap (1000, the postgres
// implementation's anomalyMaxPageLimit) so that a SHORT returned page
// unambiguously means "no more rows" — never "the repository silently
// capped a larger request I made" (C3).
const anomalyPageSize = 500

// listAllAnomalies pages through f with anomalyPageSize until a short page
// comes back, returning every matching row (C3): the dedup check and the
// resolved-override lookup must never miss a row merely because it fell
// past a repository's own default page of 100 (P4, P5).
func (b *Billing) listAllAnomalies(ctx context.Context, sc store.Scope, f store.AnomalyFilter) ([]model.ConsumptionAnomaly, error) {
	f.Page = store.Page{Limit: anomalyPageSize, Offset: f.Page.Offset}
	var out []model.ConsumptionAnomaly
	for {
		page, err := b.deps.Anomalies.List(ctx, sc, f)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if int32(len(page)) < anomalyPageSize {
			return out, nil
		}
		f.Page.Offset += anomalyPageSize
	}
}

// dedupAndCreateAnomaly implements C-6's dedup-then-create, shared by every
// reason this package writes: it lists BOTH resolved and unresolved rows
// already covering the exact same (analyzer, period_start, period_end,
// reason) and skips creation when one exists, exactly as F2's
// negativeDeltaAnomalyExists does for its own table of anomalies.
//
// C3: the dedup query narrows to the EXACT period_start instant
// ([window.From, window.From+1µs)) — never the whole [window.From,
// window.To) range, which (since AnomalyList filters on period_start alone)
// would match every OTHER anomaly whose own period_start merely falls
// inside this window (an Hourly bucket sharing a Daily row's start, or 100+
// F2 rows starting inside the hour) — and pages through ALL results (never
// relying on the repository's own default page of 100).
func (b *Billing) dedupAndCreateAnomaly(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, window energy.Window, reason string, detail []byte, message string) error {
	existing, err := b.listAllAnomalies(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Reason:      &reason,
		Range:       &store.TimeRange{From: window.From, To: window.From.Add(time.Microsecond)},
	})
	if err != nil {
		return err
	}
	for _, a := range existing {
		// I4: dedup matches only rows THIS package wrote (R60's own code) —
		// never an F2-shaped row that happens to share the exact same
		// (period_start, period_end) and reason (common: a 1-hour meter's
		// own load_profile step at Hourly, a daily-kind pair at Daily, a
		// billing pair at Monthly). An F2 row is cleared separately, by I-5's
		// cascade once the F3 row it overlaps is resolved — it must never
		// suppress that F3 row's own creation in the first place.
		if !hasSuspectPeriodCode(a.Detail) {
			continue
		}
		if a.PeriodStart.Equal(window.From) && a.PeriodEnd.Equal(window.To) {
			// M1: Create may have succeeded on a prior run while its
			// AppendMessage failed — an unresolved row missing its operator
			// message gets one now rather than staying silently unnotified.
			if a.ResolvedAt == nil {
				return b.ensureAnomalyMessage(ctx, sc, a, message, detail)
			}
			return nil
		}
	}

	created, err := b.deps.Anomalies.Create(ctx, sc, model.ConsumptionAnomaly{
		AnalyzerID:  analyzerID,
		PeriodStart: window.From,
		PeriodEnd:   window.To,
		Reason:      reason,
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
		Message:     message,
		RelatedType: ptr("consumption_anomaly"),
		RelatedID:   &created.ID,
		Metadata:    detail,
	})
	return err
}

// ensureAnomalyMessage implements M1: on a dedup hit against an unresolved
// row, check it already has its operator message (by RelatedID) and append
// one only when it is missing — a prior run's Create that succeeded while
// its own AppendMessage failed must not leave the anomaly forever silent.
func (b *Billing) ensureAnomalyMessage(ctx context.Context, sc store.Scope, a model.ConsumptionAnomaly, message string, detail []byte) error {
	msgs, err := b.deps.Ops.ListMessages(ctx, sc, store.MessageFilter{RelatedID: &a.ID})
	if err != nil {
		return err
	}
	if len(msgs) > 0 {
		return nil
	}
	_, err = b.deps.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID:   ptr(sc.CompanyID),
		Kind:        "system",
		Category:    "consumption-suspect-period",
		Status:      "error",
		Message:     message,
		RelatedType: ptr("consumption_anomaly"),
		RelatedID:   &a.ID,
		Metadata:    detail,
	})
	return err
}

// recordMissingReadingsGap acquires the SAME per-(analyzer, period_start)
// dedup lease createAnomalyForReason's caller (recordSuspectPeriod) does,
// then dedup-creates R97's own missing_readings row for one gap.
func (b *Billing) recordMissingReadingsGap(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, gap bucketGap) error {
	lease, err := b.acquireAnomalyLock(ctx, analyzerID, gap.Window.From)
	if err != nil {
		return err
	}
	defer b.releaseAnomalyLock(ctx, lease)
	return b.createMissingReadingsAnomaly(ctx, sc, analyzerID, gap)
}

// createMissingReadingsAnomaly implements R97 rule 4/5: same dedup-then-
// create core as every other reason, R97's own detail shape and message.
func (b *Billing) createMissingReadingsAnomaly(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, gap bucketGap) error {
	detail, err := buildMissingReadingsDetail(gap.Window, gap.Boundaries)
	if err != nil {
		return err
	}
	message := missingReadingsMessageText(analyzerID, gap)
	return b.dedupAndCreateAnomaly(ctx, sc, analyzerID, gap.Window, string(energy.ReasonMissingReadings), detail, message)
}

// missingReadingsMessageText renders R60's "one human-readable sentence" for
// R97's own reason, naming the missing boundary side(s).
func missingReadingsMessageText(analyzerID uuid.UUID, gap bucketGap) string {
	return fmt.Sprintf(
		"Analyzer %s: consumption for %s to %s is missing readings at the %s boundary.",
		analyzerID, gap.Window.From.UTC().Format(time.RFC3339), gap.Window.To.UTC().Format(time.RFC3339),
		strings.Join(gap.Boundaries, " and "),
	)
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

		// C3: page through ALL resolved anomalies over the analyzer's whole
		// combined window — never rely on the repository's own default page
		// of 100 (P4: a resolved override past the 100th row was silently
		// dropped).
		anomalies, err := b.listAllAnomalies(ctx, sc, store.AnomalyFilter{
			AnalyzerIDs: []uuid.UUID{analyzerID},
			Range:       &store.TimeRange{From: from, To: to.Add(time.Microsecond)},
		})
		if err != nil {
			return err
		}

		byPeriod := make(map[anomalyPeriodKey][]model.ConsumptionAnomaly, len(anomalies))
		for _, a := range anomalies {
			// C1(c): only a row THIS package wrote (R60's own code) may
			// substitute a value — never an F2-shaped row, and never
			// anything unparseable, even if its (period_start, period_end)
			// happens to line up with a bucket exactly (P7).
			if a.ResolvedAt == nil || a.Resolution == nil || !hasSuspectPeriodCode(a.Detail) {
				continue
			}
			// RI-5: a resolved missing_readings anomaly is applied ONLY
			// through applyResolvedGapOverrides (billing.go), which runs
			// exclusively on buckets that still produce NO row from real
			// readings. This loop instead post-processes rows Consumption
			// already DERIVED from real boundaries — applying a stale gap
			// override here would overwrite a real (possibly newly suspect)
			// derivation and delete its Suspect, hiding a fresh negative
			// delta a backfill just introduced. Once real boundaries resolve
			// a bucket, the real derivation wins; if it comes back suspect,
			// ConsumptionAndRecord (anomalies.go) records it as an ordinary
			// suspect period, exactly like any other reason.
			if a.Reason == string(energy.ReasonMissingReadings) {
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
			matches := byPeriod[key]
			if len(matches) == 0 {
				continue
			}
			for _, a := range matches {
				if err := applyResolvedAnomaly(row, a); err != nil {
					return err
				}
			}
			// I6: ratios are recomputed once, after every matching
			// anomaly's effect has been applied, from the row's
			// POST-substitution Values/Suspect state.
			recomputeRatios(row)
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
			// I6: an overridden register is no longer suspect, so Ratios'
			// own ActiveImport-suspect check (and any later re-derivation)
			// treats it as sound evidence.
			delete(row.Suspect, reg)
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
			// M5: only a register STILL suspect on this fresh derivation is
			// nilled — a period that has since become sound for that
			// register (new data backfilled it) is never re-nilled just
			// because an old anomaly once named it.
			if _, stillSuspect := row.Suspect[reg]; !stillSuspect {
				continue
			}
			if row.Values != nil {
				row.Values[reg] = nil
			}
			row.Resolution[reg] = string(ResolveByAccepting)
		}
	}
	return nil
}

// recomputeRatios implements I6: after ANY override substitution, the row's
// reactive ratios are recomputed from the POST-substitution Values (never
// left at whatever energy.Ratios computed before the substitution, which
// stayed nil for as long as active_import itself was suspect). It uses
// energy.Ratio directly (not energy.Ratios, which takes a whole
// Derivation) because the inputs here are the row's own effective Values
// map, not a domain Derivation — Ratio's own nil/non-positive-denominator
// rules already give the right answer whenever a needed register is still
// suspect (nil in Values) or absent.
func recomputeRatios(row *Row) {
	active := row.Values[energy.ActiveImport]
	if _, suspect := row.Suspect[energy.ActiveImport]; suspect {
		row.InductiveRatio, row.CapacitiveRatio = nil, nil
		return
	}
	row.InductiveRatio = energy.Ratio(row.Values[energy.ReactiveInductiveImport], active)
	row.CapacitiveRatio = energy.Ratio(row.Values[energy.ReactiveCapacitiveImport], active)
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
	if req.Range != nil && !req.Range.Valid() {
		// M7: this package's own fail-closed sentinel, not
		// store.ErrInvalidRange passed through raw — every other validation
		// failure in this file is consumption.ErrInvalidRequest, and a
		// caller checking errors.Is(err, consumption.ErrInvalidRequest)
		// should not have to also know this repository-level one.
		return nil, ErrInvalidRequest
	}
	return b.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: req.AnalyzerIDs,
		Unresolved:  req.Unresolved,
		Range:       req.Range,
		Page:        req.Page,
	})
}
