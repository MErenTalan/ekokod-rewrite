package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// fetchAccumulator carries FetchReadings' running counts across every chunk
// and page of one run, so any failure point can finish the job_runs row with
// an accurate picture of what happened before the failure.
type fetchAccumulator struct {
	processed, skipped       int32
	rejections               map[RejectReason]int32
	warnings                 map[string]int32
	anomaliesCreated         int32
	affectedFrom, affectedTo *time.Time
}

func newFetchAccumulator() *fetchAccumulator {
	return &fetchAccumulator{rejections: map[RejectReason]int32{}, warnings: map[string]int32{}}
}

func (a *fetchAccumulator) addRejections(rs []Rejection) {
	a.skipped += int32(len(rs))
	for _, r := range rs {
		a.rejections[r.Reason]++
	}
}

func (a *fetchAccumulator) addWarnings(ws []integration.Warning) {
	for _, w := range ws {
		a.warnings[w.Code]++
	}
}

func (a *fetchAccumulator) touchAffected(from, to time.Time) {
	if a.affectedFrom == nil || from.Before(*a.affectedFrom) {
		a.affectedFrom = ptrTime(from)
	}
	if a.affectedTo == nil || to.After(*a.affectedTo) {
		a.affectedTo = ptrTime(to)
	}
}

func (a *fetchAccumulator) detail() json.RawMessage {
	return mustJSON(fetchRunDetail{
		RejectionsByReason: a.rejections,
		WarningsByCode:     a.warnings,
		AffectedFrom:       a.affectedFrom,
		AffectedTo:         a.affectedTo,
		AnomaliesCreated:   a.anomaliesCreated,
	})
}

// FetchReadings implements job.Ingestion.FetchReadings (06 §9): resolve the
// window (an explicit p.Window, or resume from ingestion_cursors / a 30-day
// first-run lookback per R18), split it with normalize.Chunk, and for every
// chunk page through the adapter, validating and persisting each page
// before advancing the cursor.
func (s *Service) FetchReadings(ctx context.Context, p job.FetchReadingsPayload) error {
	now := s.deps.Clock.Now()
	sc := store.SystemScope(p.CompanyID)
	companyID := p.CompanyID

	run, err := s.deps.Ops.StartRun(ctx, sc, model.JobRun{
		ID: uuid.New(), CompanyID: &companyID, JobType: job.TypeIntegrationFetchReadings,
		Scope: newFetchRunScope(p.AnalyzerID, p.Kind, p.Window), StartedAt: now,
	})
	if err != nil {
		return err
	}

	analyzer, err := s.deps.Analyzers.Get(ctx, sc, p.AnalyzerID)
	if err != nil {
		errText := err.Error()
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return classifyStoreErr(err) // M14
	}

	if !analyzer.IsActive || analyzer.DeletedAt != nil {
		s.finishRun(ctx, sc, run.ID, "success", 0, 1, 0, nil, nil, now)
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "info", "analyzer inactive", nil)
		return nil
	}

	creds, err := s.deps.Credentials.Open(ctx, sc, p.CredentialID)
	if err != nil {
		errText := err.Error()
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return classifyStoreErr(err) // M14
	}

	src, err := s.deps.Sources.Source(creds.Provider)
	if err != nil {
		errText := redacted(creds, err)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return wrapRedacted(errText, err)
	}

	modelProvider, ok := creds.Provider.ModelProvider()
	if !ok || analyzer.Provider != modelProvider {
		mismatch := &integration.Error{Kind: integration.ErrMalformedPayload, Provider: creds.Provider, Op: "fetch_readings.provider_mismatch"}
		errText := redacted(creds, mismatch)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return wrapRedacted(errText, mismatch)
	}

	// M3: the analyzer's own stored ProviderSubtype must match the
	// credential actually being opened — same provider, but a different
	// subtype means a different distributor/meter family's endpoints
	// (06 §1's per-subtype Endpoints), which could otherwise silently
	// answer for the wrong meter. ErrConfig (non-retryable, SkipRetry via
	// job.ClassifyForRetry) — never a network call is attempted.
	if analyzer.ProviderSubtype != creds.Subtype {
		mismatch := &integration.Error{Kind: integration.ErrConfig, Provider: creds.Provider, Op: "fetch_readings.subtype_mismatch"}
		errText := redacted(creds, mismatch)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return wrapRedacted(errText, mismatch)
	}

	acc := newFetchAccumulator()

	from, to, allowCursorAdvance, err := s.resolveFetchWindow(ctx, sc, p, now)
	if err != nil {
		errText := err.Error()
		s.finishRun(ctx, sc, run.ID, "failed", acc.processed, acc.skipped, 1, &errText, acc.detail(), now)
		return err
	}

	// C1/R13: the sanity-jump streak is scoped to this ONE run (this
	// analyzer, this kind) and must survive every chunk and page of it —
	// see SanityStreak's doc. runOpts is s.opts with a fresh streak
	// attached; every Validate call below uses runOpts, never s.opts
	// directly.
	runOpts := s.opts
	runOpts.SanityStreak = &SanityStreak{}

	chunks := normalize.Chunk(from, to, src.MaxWindow(p.Kind))
	if len(chunks) == 0 {
		// R19: an empty window never advances the cursor, and a run that
		// fetched nothing is not a failure.
		s.finishRun(ctx, sc, run.ID, "success", 0, 0, 0, nil, acc.detail(), now)
		return nil
	}

	pages := 0
	for _, chunk := range chunks {
		var prev *model.MeterReading
		var history []model.MeterReading
		loaded := false
		reqFrom := chunk.From

		for {
			pages++
			if pages > s.opts.MaxPagesPerRun {
				err := fmt.Errorf("ingest: exceeded max pages per run (%d)", s.opts.MaxPagesPerRun)
				return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, err, acc, now)
			}

			if !loaded {
				var lerr error
				prev, lerr = s.deps.Readings.Latest(ctx, sc, analyzer.ID,
					store.TimeRange{From: chunk.From.Add(-35 * 24 * time.Hour), To: chunk.From}, p.Kind)
				if lerr != nil {
					return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, lerr, acc, now)
				}
				history, lerr = s.deps.Readings.Range(ctx, sc, analyzer.ID,
					store.TimeRange{From: chunk.From.Add(-7 * 24 * time.Hour), To: chunk.From}, p.Kind)
				if lerr != nil {
					return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, lerr, acc, now)
				}
				loaded = true
			}

			req := integration.FetchRequest{
				Point:      meteringPointFromAnalyzer(analyzer),
				AnalyzerID: analyzer.ID,
				Multiplier: analyzer.MeterMultiplier,
				Kind:       p.Kind,
				From:       reqFrom,
				To:         chunk.To,
			}
			res, ferr := src.FetchReadings(ctx, creds, req)
			if ferr != nil {
				return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, ferr, acc, now)
			}

			kept, dedupeRejected := Dedupe(res.Readings)
			// I2: an adapter's page is trusted to be about THIS analyzer
			// and to carry only p.Kind (the requested kind) or a reset
			// reading (the one other kind the negative-delta check below
			// consumes) only after this filter — a row stamped with
			// another analyzer's ID, or any other Kind, is rejected before
			// it can reach Validate, BulkInsert, or this analyzer's cursor.
			attributed, attributionRejected := filterAttribution(kept, analyzer.ID, p.Kind)
			valid, validateRejected := Validate(prev, history, attributed, now, runOpts)
			realRejected, sanityFlags := partitionSanityFlags(validateRejected)
			acc.addRejections(dedupeRejected)
			acc.addRejections(attributionRejected)
			acc.addRejections(realRejected)
			acc.addWarnings(res.Warnings)
			for _, f := range sanityFlags {
				s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "error",
					"sustained register jump — review",
					mustJSON(map[string]any{"analyzer_id": analyzer.ID, "kind": p.Kind, "ts": f.Ts}))
			}

			if len(valid) > 0 {
				inserted, updated, ierr := s.deps.Readings.BulkInsert(ctx, sc, valid)
				if ierr != nil {
					return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, ierr, acc, now)
				}
				acc.processed += int32(inserted + updated)

				// M2: hooks receive the range actually persisted THIS PAGE
				// (every kind mixed into valid, reset rows included), not
				// the outer chunk bound — a hook reconciling against what
				// it can see must be told what really landed, not what the
				// chunk planner merely intended to request.
				pageMinTs, pageMaxTs := readingTsBounds(valid)
				acc.touchAffected(pageMinTs, pageMaxTs)

				for _, hook := range s.deps.Hooks[analyzer.Provider] {
					if herr := hook.AfterPersist(ctx, sc, analyzer, p.Kind, pageMinTs, pageMaxTs); herr != nil {
						return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, herr, acc, now)
					}
				}

				// I2: the cursor, TouchLastReading, DetectNegativeDeltas'
				// sequence and the next page's "effective previous
				// reading" must only ever see this analyzer's own p.Kind
				// rows — a reset row sitting between two p.Kind rows in
				// valid (by Ts order) must never anchor the cursor, and
				// must never sit INSIDE the p.Kind delta sequence (which
				// would silently break the adjacency DetectNegativeDeltas
				// relies on to compare consecutive same-kind readings).
				kindRows := readingsOfKind(valid, p.Kind)

				if len(kindRows) > 0 {
					kindMinTs, kindMaxTs := readingTsBounds(kindRows)

					resets := readingsOfKind(valid, model.ReadingKindReset)
					// M1: time.Microsecond, not time.Nanosecond — pgx/
					// Postgres timestamptz columns are microsecond
					// precision, so a 1-nanosecond upper bound round-trips
					// back down to exactly kindMaxTs on the wire and
					// excludes a reset stored exactly there (proven by
					// TestResetSuppressionRangeSurvivesMicrosecondTruncation
					// in ingest_integration_test.go). The generation code
					// documents this same pgx trap.
					rangeResets, rerr := s.deps.Readings.Range(ctx, sc, analyzer.ID,
						store.TimeRange{From: kindMinTs, To: kindMaxTs.Add(time.Microsecond)}, model.ReadingKindReset)
					if rerr != nil {
						return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, rerr, acc, now)
					}
					resets = append(resets, rangeResets...)

					for _, d := range DetectNegativeDeltas(prev, kindRows, resets) {
						exists, eerr := s.negativeDeltaAnomalyExists(ctx, sc, analyzer.ID, d)
						if eerr != nil {
							return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, eerr, acc, now)
						}
						if exists {
							continue
						}
						if _, cerr := s.deps.Anomalies.Create(ctx, sc, model.ConsumptionAnomaly{
							AnalyzerID:  analyzer.ID,
							PeriodStart: d.PrevTs,
							PeriodEnd:   d.CurTs,
							Reason:      "negative_delta",
							Detail:      mustJSON(map[string]string{"register": d.Register, "kind": string(p.Kind)}),
						}); cerr != nil {
							return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, cerr, acc, now)
						}
						acc.anomaliesCreated++
					}

					// R19/I1: the cursor and last-reading watermark advance
					// only when at least one p.Kind row was persisted this
					// page AND this run is allowed to move the live cursor
					// at all (I1: an explicit window that starts after a
					// gap the cursor has not covered yet must never move
					// it — see resolveFetchWindow's doc).
					if allowCursorAdvance {
						if cerr := s.deps.Cursors.RecordSuccess(ctx, sc, analyzer.ID, p.Kind, kindMaxTs, now); cerr != nil {
							return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, cerr, acc, now)
						}
						if terr := s.deps.Analyzers.TouchLastReading(ctx, sc, analyzer.ID, kindMaxTs); terr != nil {
							return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, terr, acc, now)
						}
					}

					last := kindRows[len(kindRows)-1]
					prev = &last
				}

				if hourly := convertHourlyValues(analyzer.ID, analyzer.Provider, res.HourlyValues); len(hourly) > 0 {
					if _, _, herr := s.deps.ProviderSeries.UpsertHourly(ctx, sc, hourly); herr != nil {
						return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, herr, acc, now)
					}
				}

				// R51/I2: a multiplier update is persisted ONLY when the
				// adapter reports ProviderResolved — the value actually
				// came from the provider THIS call (e.g. GridBox's
				// last_endex.Multiplier or its load_profile ratio). A
				// not-provider-resolved value (GridBox's fallback to the
				// stored multiplier itself, or its last-resort fallback to
				// 1) carries no new information and must never overwrite
				// analyzers.meter_multiplier — persisting it anyway is
				// exactly what caused GridBox's daily/reset/current_index/
				// billing kinds (none of which carries its own multiplier
				// source) to ping-pong the stored value back and forth
				// against whatever the concurrently-run load_profile kind
				// last derived.
				if res.ResolvedMultiplier != nil && res.ResolvedMultiplier.ProviderResolved && !res.ResolvedMultiplier.Value.Equal(analyzer.MeterMultiplier) {
					// M1: re-load the analyzer immediately before the
					// update, so a concurrent operator edit to any OTHER
					// field (Update takes the whole row) made since this
					// run loaded `analyzer` at the top survives. On
					// failure, neither report "multiplier changed" (it did
					// not happen) nor mutate the in-memory `analyzer` this
					// run keeps using for its own remaining requests —
					// only a successful write may do either; a failure
					// here is logged and the run continues (the readings
					// this page already persisted are not at risk).
					if fresh, gerr := s.deps.Analyzers.Get(ctx, sc, analyzer.ID); gerr != nil {
						s.deps.Log.ErrorContext(ctx, "ingest: reload analyzer before multiplier update failed", "analyzer_id", analyzer.ID, "error", gerr)
					} else {
						fresh.MeterMultiplier = res.ResolvedMultiplier.Value
						fresh.UpdatedAt = now
						if newA, uerr := s.deps.Analyzers.Update(ctx, sc, fresh); uerr != nil {
							s.deps.Log.ErrorContext(ctx, "ingest: update analyzer multiplier failed", "analyzer_id", analyzer.ID, "error", uerr)
						} else {
							analyzer = newA
							s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "warning", "meter multiplier changed",
								mustJSON(map[string]any{"analyzer_id": analyzer.ID, "multiplier": analyzer.MeterMultiplier.String()}))
						}
					}
				}
			}

			if res.NextCursor == nil {
				break
			}
			// R4: NextCursor must be after this request's From, or the
			// pagination would loop forever (or worse, walk backwards).
			if !res.NextCursor.After(req.From) {
				badCursor := &integration.Error{Kind: integration.ErrMalformedPayload, Provider: creds.Provider, Op: "fetch_readings.next_cursor"}
				return s.failFetchRun(ctx, sc, run.ID, analyzer.ID, p.Kind, creds, badCursor, acc, now)
			}
			reqFrom = *res.NextCursor
		}
	}

	status := fetchRunStatus(0, acc.processed > 0)
	s.finishRun(ctx, sc, run.ID, status, acc.processed, acc.skipped, 0, nil, acc.detail(), now)

	if acc.skipped > 0 {
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "warning",
			fmt.Sprintf("%d readings rejected", acc.skipped), mustJSON(acc.rejections))
	}
	// M5: a fixed, sorted order — acc.warnings is a map, and iterating it
	// directly would emit these messages in an unpredictable order from one
	// run to the next.
	for _, code := range sortedWarningCodes(acc.warnings) {
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "warning", code,
			mustJSON(map[string]int32{"count": acc.warnings[code]}))
	}

	return nil
}

// failFetchRun records the failure on the cursor, finishes the job_runs row
// (partial if anything was persisted before the failure, else failed), and
// returns cause wrapped (I3) so the caller can `return s.failFetchRun(...)`
// without handing the job layer cause's own, possibly credential-bearing,
// text — the wrapper's Error() is exactly errText below; its Unwrap()
// still reaches cause.
func (s *Service) failFetchRun(ctx context.Context, sc store.Scope, runID, analyzerID uuid.UUID, kind model.ReadingKind, creds integration.Credentials, cause error, acc *fetchAccumulator, at time.Time) error {
	errText := redacted(creds, cause)
	if rerr := s.deps.Cursors.RecordFailure(ctx, sc, analyzerID, kind, errText, at); rerr != nil {
		s.deps.Log.ErrorContext(ctx, "ingest: record cursor failure failed", "error", rerr)
	}
	status := fetchRunStatus(1, acc.processed > 0)
	s.finishRun(ctx, sc, runID, status, acc.processed, acc.skipped, 1, &errText, acc.detail(), at)
	s.appendMessage(ctx, sc, sc.CompanyID, "job", "analyzer-refresh", "error", errText, nil)
	return wrapRedacted(errText, cause)
}

// resolveFetchWindow implements the brief's step 4: an explicit p.Window
// wins outright; otherwise From resumes from the stored cursor, or —
// ErrNotFound, or a cursor row with no LastTs yet (RecordFailure can create
// one before any success) — falls back to R18's 30-day lookback. To is
// always now for the cursor-driven form.
//
// allowCursorAdvance is I1/M2: an explicit p.Window may move the LIVE
// cursor forward only when doing so does not skip data the cursor has not
// covered yet — i.e. the window is contiguous with (starts at or before)
// what the cursor already reflects. A window that starts AFTER the stored
// cursor (a gap between what the cursor covers and what this explicit
// fetch is about to persist) must leave the cursor exactly where it was:
// advancing it would make the next cursor-driven run skip straight past
// the gap instead of requesting it.
//
// M2: when there is NO stored cursor at all yet (ErrNotFound, or a row
// with no LastTs), an explicit window must ALSO leave the cursor untouched
// — round 1 returned allowCursorAdvance=true here, which let an explicit
// window (e.g. a backfill) CREATE the live cursor from nothing. A backfill
// controller has no ordering guarantee: a RECENT window processed before
// an OLDER one would set the cursor to the recent window's own bound, and
// the first CURSOR-DRIVEN run would then resume from there instead of
// using its full InitialLookback — silently shortening the very lookback
// R18 exists to guarantee. Only a cursor-driven fetch (p.Window == nil,
// below) may ever create the cursor from nothing.
func (s *Service) resolveFetchWindow(ctx context.Context, sc store.Scope, p job.FetchReadingsPayload, now time.Time) (from, to time.Time, allowCursorAdvance bool, err error) {
	cur, cerr := s.deps.Cursors.Get(ctx, sc, p.AnalyzerID, p.Kind)

	if p.Window != nil {
		switch {
		case cerr == nil && cur.LastTs != nil:
			return p.Window.From, p.Window.To, !cur.LastTs.Before(p.Window.From), nil
		case cerr == nil, errors.Is(cerr, store.ErrNotFound):
			return p.Window.From, p.Window.To, false, nil
		default:
			return time.Time{}, time.Time{}, false, cerr
		}
	}

	switch {
	case cerr == nil && cur.LastTs != nil:
		return *cur.LastTs, now, true, nil
	case cerr == nil, errors.Is(cerr, store.ErrNotFound):
		return now.Add(-s.opts.InitialLookback), now, true, nil
	default:
		return time.Time{}, time.Time{}, false, cerr
	}
}

// negativeDeltaAnomalyExists implements R14's dedup rule: skip creating a
// consumption_anomalies row when one already covers the exact same period
// and register — resolved or not, so a re-fetch never resurrects a resolved
// anomaly.
func (s *Service) negativeDeltaAnomalyExists(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, d NegativeDelta) (bool, error) {
	reason := "negative_delta"
	anomalies, err := s.deps.Anomalies.List(ctx, sc, store.AnomalyFilter{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Reason:      &reason,
		Range:       &store.TimeRange{From: d.PrevTs, To: d.CurTs.Add(time.Nanosecond)},
	})
	if err != nil {
		return false, err
	}
	for _, a := range anomalies {
		if !a.PeriodStart.Equal(d.PrevTs) || !a.PeriodEnd.Equal(d.CurTs) {
			continue
		}
		var detail struct {
			Register string `json:"register"`
		}
		if jerr := json.Unmarshal(a.Detail, &detail); jerr != nil {
			continue
		}
		if detail.Register == d.Register {
			return true, nil
		}
	}
	return false, nil
}

// meteringPointFromAnalyzer rebuilds the MeteringPoint an adapter needs from
// what is stored on the analyzer — everything a prior SyncAnalyzers upsert
// recorded (R27/R30).
func meteringPointFromAnalyzer(a model.Analyzer) integration.MeteringPoint {
	multiplier := a.MeterMultiplier
	return integration.MeteringPoint{
		InstallationNumber: a.InstallationNumber,
		CustomerName:       a.CustomerName,
		Address:            a.Address,
		Province:           a.Province,
		District:           a.District,
		Neighbourhood:      a.Neighbourhood,
		Street:             a.Street,
		TariffType:         a.TariffType,
		TariffKind:         a.TariffKind,
		InstallationKind:   a.InstallationKind,
		InstalledPowerKw:   a.InstalledPowerKw,
		MeterNumber:        a.MeterNumber,
		MeterModel:         a.MeterModel,
		MeterMultiplier:    &multiplier,
		CounterpartyNo:     a.CounterpartyNo,
		MeteringPointName:  a.MeteringPointName,
		EtsoCode:           a.EtsoCode,
		Latitude:           a.Latitude,
		Longitude:          a.Longitude,
		DefinitionType:     a.DefinitionType,
	}
}

// filterAttribution implements I2: a row is trusted to belong to this fetch
// (persisted, counted toward this analyzer's cursor, compared against this
// analyzer's prev/history) only after it passes here. An adapter bug, or a
// misconfigured/malicious upstream, that stamps a returned row with another
// analyzer's ID must never let that row reach Validate or BulkInsert; a row
// whose Kind is neither the requested kind nor model.ReadingKindReset (the
// one other kind the fetch loop's negative-delta step expects mixed into a
// page) is equally untrusted here — reset rows are consumed separately, by
// Ts, later in the loop, and any other Kind has no meaning for this
// request at all.
func filterAttribution(rows []model.MeterReading, analyzerID uuid.UUID, kind model.ReadingKind) (kept []model.MeterReading, rejected []Rejection) {
	for _, r := range rows {
		switch {
		case r.AnalyzerID != analyzerID:
			rejected = append(rejected, Rejection{Ts: r.Ts, Kind: r.Kind, Reason: RejectWrongAnalyzer})
		case r.Kind != kind && r.Kind != model.ReadingKindReset:
			rejected = append(rejected, Rejection{Ts: r.Ts, Kind: r.Kind, Reason: RejectWrongKind})
		default:
			kept = append(kept, r)
		}
	}
	return kept, rejected
}

func readingsOfKind(rows []model.MeterReading, kind model.ReadingKind) []model.MeterReading {
	var out []model.MeterReading
	for _, r := range rows {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// readingTsBounds returns the earliest and latest Ts in rows. rows must be
// non-empty.
func readingTsBounds(rows []model.MeterReading) (min, max time.Time) {
	min, max = rows[0].Ts, rows[0].Ts
	for _, r := range rows[1:] {
		if r.Ts.Before(min) {
			min = r.Ts
		}
		if r.Ts.After(max) {
			max = r.Ts
		}
	}
	return min, max
}

// convertHourlyValues turns an adapter's OSOS cross-check series (06 §2) into
// storable rows, deduplicated on (analyzer_id, ts) so
// ProviderSeriesRepository.UpsertHourly — which refuses a duplicate key
// within one batch, the same rule as ReadingRepository.BulkInsert — never
// sees two rows for the same hour from a single page.
func convertHourlyValues(analyzerID uuid.UUID, provider model.IntegrationProvider, values []integration.HourlyValue) []model.ProviderHourlyValue {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]int, len(values))
	out := make([]model.ProviderHourlyValue, 0, len(values))
	for _, v := range values {
		key := v.Ts.UTC().UnixNano()
		row := model.ProviderHourlyValue{
			AnalyzerID:        analyzerID,
			Ts:                v.Ts,
			ActiveConsumption: v.ActiveConsumption,
			ActiveGeneration:  v.ActiveGeneration,
			SourceProvider:    provider,
		}
		if idx, ok := seen[key]; ok {
			out[idx] = row
			continue
		}
		seen[key] = len(out)
		out = append(out, row)
	}
	return out
}
