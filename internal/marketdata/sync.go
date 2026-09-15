package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ptfChunkMax is the size Syncer re-chunks the resolved window to before
// each HourlyPTF call and UpsertHourlyPrices write (task-12 brief: "per
// normalize.Chunk(window, 30*24h) chunk (S7; processed += rows)"). This is
// a SEPARATE chunking pass from epias.Client's own internal 30-day
// chunking of a single HourlyPTF call — the two happen to use the same
// size, but for different reasons: the client's is R20 pagination against
// EPİAŞ's own per-request limit, this one is so each store write stays a
// bounded batch and a failure partway through a long backfill has already
// durably written everything before it.
const ptfChunkMax = 30 * 24 * time.Hour

// PriceSource is what Syncer needs from an EPİAŞ client — satisfied by
// *epias.Client. Declared here, not imported from internal/integration/epias,
// so this package never depends on that one (06 §1 rule 2: the store side
// of a job never reaches back into the adapter package; it only needs the
// adapter's return shapes, which live in internal/domain/model and
// internal/integration).
type PriceSource interface {
	// HourlyPTF returns one model.MarketPrice per published hour in
	// [from, to), UTC, TL/MWh, plus a Warning per row that could not be
	// parsed (already reflected in a shorter result — nothing is
	// fabricated for a warned row).
	HourlyPTF(ctx context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error)
	// YekdemUnitCost returns one model.YekdemMonthly per published (year,
	// month) intersecting [from, to).
	YekdemUnitCost(ctx context.Context, from, to time.Time) ([]model.YekdemMonthly, error)
}

// Syncer implements job.PriceSyncer: it fetches EPİAŞ prices through
// Source and persists them through the platform-wide admin repositories —
// never a tenant Scope (Global Constraints: "platform-wide work goes only
// through the store.Admin* interfaces").
type Syncer struct {
	Source  PriceSource
	Market  store.AdminMarketDataRepository
	Journal store.AdminJournalRepository
	Clock   clock.Clock
	Log     *slog.Logger
}

// New builds a Syncer.
func New(src PriceSource, market store.AdminMarketDataRepository, journal store.AdminJournalRepository, c clock.Clock, log *slog.Logger) *Syncer {
	return &Syncer{Source: src, Market: market, Journal: journal, Clock: c, Log: log}
}

var _ job.PriceSyncer = (*Syncer)(nil)

// syncScope is StartPlatformRun's Scope payload: what window this run
// actually covered, for an operator reading job_runs.
type syncScope struct {
	PTFFrom    time.Time `json:"ptf_from"`
	PTFTo      time.Time `json:"ptf_to"`
	YekdemFrom time.Time `json:"yekdem_from"`
	YekdemTo   time.Time `json:"yekdem_to"`
}

// syncDetail is FinishPlatformRun's Detail payload.
type syncDetail struct {
	MissingDays  int `json:"missing_days"`
	MissingHours int `json:"missing_hours"`
	// YekdemDropped is the count of YEKDEM rows the source silently
	// dropped as unparseable (M2, fix round 1) — 0 unless Source
	// implements the optional yekdemDropReporter capability, since
	// PriceSource.YekdemUnitCost's own return has no Warnings slot to
	// carry this through directly. See yekdemDropReporter's doc comment.
	YekdemDropped int32 `json:"yekdem_dropped,omitempty"`
}

// yekdemDropReporter is an OPTIONAL capability a PriceSource may
// implement — satisfied by *epias.Client — to report how many YEKDEM rows
// its most recent YekdemUnitCost call silently dropped as unparseable.
//
// M2 (task-12-fix1-findings.md, fix round 1): YekdemUnitCost's signature
// (`([]model.YekdemMonthly, error)`, pinned by the task-12 brief) has no
// []integration.Warning slot the way HourlyPTF's does, so a dropped YEKDEM
// row cannot ride a Warning without breaking that pinned interface. This
// package therefore type-asserts for this narrower, optional capability
// instead (the same pattern as io.WriterTo) and, when present, folds the
// count into syncDetail.YekdemDropped — resolving M2 via "surface the drop
// count in the sync run's detail", the finding's explicit fallback for a
// signature a Warning truly cannot fit.
type yekdemDropReporter interface {
	YekdemDropped() int32
}

// SyncPrices implements job.PriceSyncer. It never fabricates a value for a
// period EPİAŞ has not published — a gap is reported (partial run, one
// operational message) and left for billing to flag (F4), never
// approximated.
func (s *Syncer) SyncPrices(ctx context.Context, p job.SyncPricesPayload) error {
	from, to := s.resolveWindow(p.Window)
	yekFrom, yekTo := s.resolveYekdemWindow()

	scope, err := json.Marshal(syncScope{PTFFrom: from, PTFTo: to, YekdemFrom: yekFrom, YekdemTo: yekTo})
	if err != nil {
		return fmt.Errorf("marketdata: encode run scope: %w", err)
	}

	run, err := s.Journal.StartPlatformRun(ctx, model.JobRun{
		JobType:   "epias.sync_prices",
		Scope:     scope,
		StartedAt: s.Clock.Now(),
		Status:    "running",
	})
	if err != nil {
		return err
	}

	var processed, skipped int32
	var allPrices []model.MarketPrice

	for _, chunk := range normalize.Chunk(from, to, ptfChunkMax) {
		prices, warnings, err := s.Source.HourlyPTF(ctx, chunk.From, chunk.To)
		if err != nil {
			s.fail(ctx, run.ID, processed, skipped, err)
			return err
		}
		// D1 (Global Constraints): a batch to AdminMarketDataRepository is
		// deduplicated BEFORE the call — UpsertHourlyPrices refuses a
		// batch containing two rows for the same Ts outright.
		prices = dedupPrices(prices)

		n, err := s.Market.UpsertHourlyPrices(ctx, prices)
		if err != nil {
			s.fail(ctx, run.ID, processed, skipped, err)
			return err
		}
		processed += int32(n)
		// A row EPİAŞ sent but this client could not parse (Warning) is
		// counted skipped: fetched, deliberately not stored, never
		// fabricated.
		skipped += int32(len(warnings))
		allPrices = append(allPrices, prices...)
	}

	yek, err := s.Source.YekdemUnitCost(ctx, yekFrom, yekTo)
	if err != nil {
		s.fail(ctx, run.ID, processed, skipped, err)
		return err
	}
	// M2: see yekdemDropReporter's doc comment — Source may optionally
	// report how many YEKDEM rows it silently dropped as unparseable;
	// when it does, that count rides in syncDetail.YekdemDropped below,
	// not a Warning (YekdemUnitCost's signature has no Warnings slot).
	var yekdemDropped int32
	if r, ok := s.Source.(yekdemDropReporter); ok {
		yekdemDropped = r.YekdemDropped()
	}
	yek = dedupYekdem(yek)
	yn, err := s.Market.UpsertYekdem(ctx, yek)
	if err != nil {
		s.fail(ctx, run.ID, processed, skipped, err)
		return err
	}
	processed += int32(yn)

	// Missing hours are checked only up through the last Istanbul-local day
	// that actually had ANY published price — a day-ahead publication for
	// tomorrow may legitimately not exist yet before ~14:00, and that is
	// not a completeness defect.
	boundary := lastPublishedBoundary(allPrices, to)
	missing := MissingHours(allPrices, from, boundary)

	status := "success"
	if len(missing) > 0 {
		status = "partial"
		if err := s.appendMissingHoursMessage(ctx, missing); err != nil {
			s.logError("could not append missing-hours message", err)
		}
	}

	detail, err := json.Marshal(syncDetail{MissingDays: len(missing), MissingHours: totalMissing(missing), YekdemDropped: yekdemDropped})
	if err != nil {
		detail = nil
	}

	_, err = s.Journal.FinishPlatformRun(ctx, run.ID, status, processed, skipped, 0, nil, detail, s.Clock.Now())
	return err
}

// fail stamps run as failed. errText is err.Error() directly, with no
// separate secret.Redact pass: Source's errors are always
// *integration.Error (or a context-cancellation error), which by
// construction (internal/integration/httpx's classify.go) never carries a
// URL, query string, request/response body or header value — Syncer has
// no credential material of its own to add fragments for, by design (a
// PriceSource is a pure fetch interface; threading the EPİAŞ password or
// ticket into this package would break exactly the separation 06 §1 rule
// 2 exists to keep). Market/Journal errors are ordinary store errors,
// which never carry EPİAŞ secrets either.
func (s *Syncer) fail(ctx context.Context, runID uuid.UUID, processed, skipped int32, err error) {
	errText := err.Error()
	if _, finishErr := s.Journal.FinishPlatformRun(ctx, runID, "failed", processed, skipped, 0, &errText, nil, s.Clock.Now()); finishErr != nil {
		s.logError("could not record failed run", finishErr)
	}
}

func (s *Syncer) logError(msg string, err error) {
	if s.Log == nil {
		return
	}
	s.Log.Error("marketdata: "+msg, "error", err)
}

// appendMissingHoursMessage appends the one operator-facing warning the
// task-12 brief names: "<n> PTF hours missing", Metadata carrying the
// per-day breakdown.
func (s *Syncer) appendMissingHoursMessage(ctx context.Context, missing map[string][]time.Time) error {
	metadata, err := json.Marshal(map[string]any{"days": missing})
	if err != nil {
		return err
	}
	_, err = s.Journal.AppendPlatformMessage(ctx, model.OperationalMessage{
		Kind:      "job",
		Category:  "market-prices",
		Status:    "warning",
		Message:   fmt.Sprintf("%d PTF hours missing", totalMissing(missing)),
		Metadata:  metadata,
		CreatedAt: s.Clock.Now(),
	})
	return err
}

// resolveWindow implements the task-12 brief's default: p.Window, or
// [Istanbul today − 7 days 00:00, Istanbul tomorrow + 1 day 00:00).
func (s *Syncer) resolveWindow(w *job.Window) (time.Time, time.Time) {
	if w != nil {
		return w.From, w.To
	}
	now := s.Clock.Now().In(normalize.Istanbul)
	y, m, d := now.Date()
	from := time.Date(y, m, d-7, 0, 0, 0, 0, normalize.Istanbul)
	to := time.Date(y, m, d+2, 0, 0, 0, 0, normalize.Istanbul)
	return from.UTC(), to.UTC()
}

// resolveYekdemWindow implements the task-12 brief's YEKDEM window: the
// first day of (this run's month − 3) to the first day of next month.
//
// Ruling R39 (task-12-fix1-findings.md, fix round 1): an explicit PTF
// backfill window (p.Window) does NOT backfill YEKDEM. YEKDEM ALWAYS uses
// this trailing [now.month-3, now.month+1) cadence, computed from the
// CURRENT run's clock, regardless of what p.Window a caller passed for
// PTF. A historical YEKDEM backfill (re-fetching unit costs for months
// outside this trailing window) is a separate admin action, out of this
// job's scope — F6/F14. The reasoning stands as before: YEKDEM is monthly
// and cheap to refresh on every run, and a caller backfilling an old PTF
// range via p.Window should not, as a side effect, overwrite the
// platform's current YEKDEM figures with values for a window it never
// asked for.
func (s *Syncer) resolveYekdemWindow() (time.Time, time.Time) {
	now := s.Clock.Now().In(normalize.Istanbul)
	y, m, _ := now.Date()
	from := time.Date(y, m-3, 1, 0, 0, 0, 0, normalize.Istanbul)
	to := time.Date(y, m+1, 1, 0, 0, 0, 0, normalize.Istanbul)
	return from.UTC(), to.UTC()
}

// dedupPrices removes a row sharing a Ts with an earlier one in prices,
// keeping the first occurrence. epias.Client already refuses a duplicate
// Date within one HTTP response (ErrMalformedPayload) and its own chunk
// windows never overlap, so this is defence in depth for
// AdminMarketDataRepository's own "duplicate keys refused" rule (Global
// Constraints D1), not a path expected to ever actually drop a row.
func dedupPrices(prices []model.MarketPrice) []model.MarketPrice {
	seen := make(map[time.Time]bool, len(prices))
	out := make([]model.MarketPrice, 0, len(prices))
	for _, p := range prices {
		key := p.Ts.UTC()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// dedupYekdem removes a row sharing a (Year, Month) with an earlier one,
// keeping the first occurrence. Unlike dedupPrices, this one is NOT purely
// defensive: epias.Client.YekdemUnitCost already dedupes across its own
// internal chunk boundaries, but Syncer calls it exactly once per run, so
// this second pass only matters if that contract ever changes — kept for
// the same D1 reason as dedupPrices.
func dedupYekdem(values []model.YekdemMonthly) []model.YekdemMonthly {
	type key struct{ year, month int16 }
	seen := make(map[key]bool, len(values))
	out := make([]model.YekdemMonthly, 0, len(values))
	for _, v := range values {
		k := key{v.Year, v.Month}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// lastPublishedBoundary returns the exclusive end of the last
// Istanbul-local day for which prices contains ANY price, clipped to
// windowTo. An empty prices reports windowTo unchanged — a total fetch
// failure (zero prices returned across the whole window with no error) is
// deliberately NOT hidden by the "not yet published" carve-out; it is
// reported as every requested hour missing.
func lastPublishedBoundary(prices []model.MarketPrice, windowTo time.Time) time.Time {
	if len(prices) == 0 {
		return windowTo
	}
	maxTs := prices[0].Ts
	for _, p := range prices[1:] {
		if p.Ts.After(maxTs) {
			maxTs = p.Ts
		}
	}
	local := maxTs.In(normalize.Istanbul)
	y, m, d := local.Date()
	dayAfter := time.Date(y, m, d+1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	if dayAfter.Before(windowTo) {
		return dayAfter
	}
	return windowTo
}

func totalMissing(m map[string][]time.Time) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}
