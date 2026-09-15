// Package gridbox implements the GridBox provider adapter (06-integrations.md
// §3): metering points identified by wiring number (installation_number), a
// bearer-token OAuth2 password grant, a {ResultStatus,ResultObject} envelope
// on every other call, and a three-step multiplier resolution (see
// ResolveMultiplier in mapping.go) whose result the pipeline must never
// re-derive once this package has stamped it on MultiplierApplied.
//
// GridBox resolves its OWN meter multiplier live from provider data
// (last_endex, and — for load_profile fetches — the fetched rows
// themselves): 06 §3's "Multiplier resolution" is entirely this package's
// concern. FetchRequest.Multiplier (Task 1's R3 field, "the analyzer's own
// STORED meter_multiplier, resolved by the pipeline before the call") is
// therefore never read here — it is the pipeline's cached copy of what a
// PRIOR call's ResolveMultiplier returned, for callers that have no
// provider-side multiplier source of their own. This adapter is the
// producer of that value (FetchResult.ResolvedMultiplier), never its
// consumer.
package gridbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

const (
	// maxWindow is the Task 7 brief's "MaxWindow 30 days": Planner.MaxWindow's
	// answer for every kind, and the chunk size splitWindow splits an
	// over-wide fetch window into (R20 — GridBox's pagination unit is a
	// window chunk, not a provider-side cursor: 06 §3's Endpoints table
	// names no cursor/page parameter).
	maxWindow = 30 * 24 * time.Hour

	defaultPageBudget = 10

	// requestEvery/requestBurst/requestTimeout configure this adapter's own
	// httpx.ClientConfig, per provider-defaults.md's `gridbox` row: "Rate /
	// burst" 2 / 2, 30s request timeout. Fix round 1 (I2): the original
	// round set requestEvery to a full second — one request per Every, so
	// that was only 1 req/s, understating the table's 2/2 by half. Every
	// must be 500ms (one request per 500ms = 2 req/s) to match; Burst was
	// already correct. See task-7-report.md's "Fix round 1" section for the
	// mutation proof (revert to time.Second and TestGridBoxDataClientRate
	// fails).
	requestEvery   = 500 * time.Millisecond
	requestBurst   = 2
	requestTimeout = 30 * time.Second
)

// Options configures a Source.
type Options struct {
	// Clock is used to stamp MeterReading.IngestedAt. nil: clock.System().
	Clock clock.Clock
	// PageBudget bounds how many window chunks one FetchReadings call
	// processes before returning a NextCursor. Default 10.
	PageBudget int
}

// Source is the GridBox integration.Adapter.
type Source struct {
	pool       *httpx.Pool
	clock      clock.Clock
	pageBudget int
}

// New builds a Source drawing every HTTP call from pool.
func New(pool *httpx.Pool, o Options) *Source {
	c := o.Clock
	if c == nil {
		c = clock.System()
	}
	pb := o.PageBudget
	if pb <= 0 {
		pb = defaultPageBudget
	}
	return &Source{pool: pool, clock: c, pageBudget: pb}
}

var _ integration.Adapter = (*Source)(nil)

// Provider reports integration.ProviderGridBox.
func (s *Source) Provider() integration.Provider { return integration.ProviderGridBox }

// Kinds is 06 §3's Flow step 2, minus billing when the company has not
// enabled it: load_profile, daily, reset and current_index always; billing
// iff creds.Settings.UseBillingIndexes.
func (s *Source) Kinds(creds integration.Credentials) []model.ReadingKind {
	kinds := []model.ReadingKind{
		model.ReadingKindLoadProfile,
		model.ReadingKindDaily,
		model.ReadingKindReset,
		model.ReadingKindCurrentIndex,
	}
	if creds.Settings.UseBillingIndexes {
		kinds = append(kinds, model.ReadingKindBilling)
	}
	return kinds
}

// MaxWindow is 30 days for every kind (Task 7 brief).
func (s *Source) MaxWindow(model.ReadingKind) time.Duration { return maxWindow }

// gridboxEndpointKeys is every endpoint key 06 §3 assigns GridBox.
var gridboxEndpointKeys = []string{"token", "last_success_date", "last_endex", "load_profiles", "endexes", "energy_values"}

// configError reports a deliberately-classified configuration problem: a
// missing/blank endpoint template, or (fetchConfigError) a blank wiring
// number. Neither is a payload GridBox ever sent us, so ErrMalformedPayload
// is the wrong Kind (adapter review pattern 9); task-7-brief.md defines no
// dedicated configuration sentinel, and Task 1's errors.go has none either,
// so — per the review's own explicitly sanctioned fallback ("or ErrAuth if
// credentials incomplete") — this is reported as ErrAuth: an incomplete
// credential/endpoint configuration is, like a rejected password, a
// "this account cannot be used until fixed" state that must not be retried.
func configError(op string) error {
	return &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderGridBox, Op: op}
}

// tokenConfigError validates the one endpoint Verify/DiscoverMeteringPoints/
// token() itself needs, before any HTTP call is attempted.
func tokenConfigError(creds integration.Credentials) error {
	if strings.TrimSpace(creds.Endpoints["token"]) == "" {
		return configError("config:token")
	}
	return nil
}

// fetchConfigError validates every endpoint FetchReadings needs plus the
// wiring number, before any HTTP call is attempted (adapter review pattern
// 12: "empty installation/device id → error before any call").
func fetchConfigError(creds integration.Credentials, wiringNo string) error {
	for _, key := range gridboxEndpointKeys {
		if strings.TrimSpace(creds.Endpoints[key]) == "" {
			return configError("config:" + key)
		}
	}
	if strings.TrimSpace(wiringNo) == "" {
		return configError("config:wiringNo")
	}
	return nil
}

// dataClientConfig builds the ClientConfig every non-token endpoint call
// uses (dataClient wraps it; a plain function so an internal test can
// assert its fields directly, with no Pool/Source needed — see
// TestGridBoxDataClientRate in source_internal_test.go).
//
// Fix round 1 (I1): provider-defaults.md's `gridbox` row marks "Serialise
// per company: yes" for the whole provider, not only the token exchange —
// the original round gave tokenClient a SerializeKey but left dataClient
// (every last_success_date/last_endex/load_profiles/endexes/energy_values
// call) unserialised, so two concurrent jobs for the same company could
// still race two data calls against each other. dataClient now carries its
// own SerializeKey, scoped to the company like its LimiterKey — deliberately
// the SAME key as LimiterKey's own "gridbox:<company>" string (distinct
// only from tokenClient's own "gridbox:token:<company>", which token()
// keeps as its own key — see tokenClient's doc below for why the two are
// not merged into one lock).
func dataClientConfig(creds integration.Credentials) httpx.ClientConfig {
	key := "gridbox:" + creds.CompanyID.String()
	return httpx.ClientConfig{
		Provider:       integration.ProviderGridBox,
		LimiterKey:     key,
		Every:          requestEvery,
		Burst:          requestBurst,
		RequestTimeout: requestTimeout,
		SerializeKey:   key,
	}
}

// dataClient builds the Client every non-token endpoint call uses.
func (s *Source) dataClient(creds integration.Credentials) *httpx.Client {
	return s.pool.Client(dataClientConfig(creds))
}

// tokenClient is deliberately its own Client, on its own limiter key and
// serialised (SerializeKey) per company: the token call is a non-idempotent
// OAuth2 grant exchange that must never be retried blindly, and two
// concurrent jobs must never race two token exchanges against the same
// credential. Controller ruling R32 (Task 2 fix round 1): a
// `httpx.Request.NoRetry` field is landing on Task 2's branch, not yet on
// this base — see the R32 comment on token() below, the one call site every
// login/refresh goes through.
//
// This key is kept DISTINCT from dataClientConfig's own per-company
// SerializeKey (fix round 1, I1) rather than merged into one lock: a token
// exchange and a data call are different operations with different retry
// rules (token: MaxAttempts effectively 1 once R32 lands; data: the
// Client's normal jittered retries), and merging their locks would have an
// in-flight token refresh block every data call for the same company (and
// vice versa) for no reason the brief asks for — 06 §3 only requires that
// GridBox calls of the SAME kind for the SAME company never race each
// other, not that token and data calls take turns.
func (s *Source) tokenClient(creds integration.Credentials) *httpx.Client {
	return s.pool.Client(httpx.ClientConfig{
		Provider:       integration.ProviderGridBox,
		LimiterKey:     "gridbox:token:" + creds.CompanyID.String(),
		Every:          requestEvery,
		Burst:          1,
		RequestTimeout: requestTimeout,
		SerializeKey:   "gridbox:token:" + creds.CompanyID.String(),
	})
}

// token exchanges creds for a bearer token via the token endpoint's OAuth2
// password grant (06 §3 step 1; legacy gridbox/refresh/route.ts:93-113).
// This is the ONE call site every login/token/refresh request goes through
// — see tokenClient's doc above.
func (s *Source) token(ctx context.Context, creds integration.Credentials) (string, error) {
	if err := tokenConfigError(creds); err != nil {
		return "", err
	}

	form := url.Values{
		"username":   {creds.Username},
		"password":   {creds.Secret.Reveal()},
		"grant_type": {"password"},
	}

	// R32: set NoRetry: true once httpx.Request.NoRetry lands (Task 2 fix
	// round 1) — this exchange must never be retried in-client.
	resp, err := s.tokenClient(creds).Do(ctx, httpx.Request{
		Op:          "token",
		Method:      http.MethodPost,
		Template:    creds.Endpoints["token"],
		Body:        []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err != nil {
		return "", remapTokenAuthFailure(err)
	}

	tr, decErr := decodeTokenResponse(resp.Body)
	if decErr != nil || tr.AccessToken == "" {
		return "", &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderGridBox, Op: "token", HTTPStatus: resp.Status}
	}
	return tr.AccessToken, nil
}

// remapTokenAuthFailure re-classifies a token-endpoint failure that arrived
// as HTTP 400 — classifyStatus's generic 4xx default, ErrMalformedPayload —
// into ErrAuth: GridBox's OAuth2 password grant reports a rejected
// credential as 400 invalid_grant, not 401 (06 §3 step 1: "a 400/401 with
// invalid_grant is ErrAuth"). httpx never returns a failed response's body
// (03 §7: no request or response body ever survives into an error), so this
// cannot inspect the body for "invalid_grant" specifically; every 400 from
// the token endpoint is treated as an auth failure, the only documented
// cause of one there. A 401 is already ErrAuth via classifyStatus and is
// returned unchanged.
func remapTokenAuthFailure(err error) error {
	var ierr *integration.Error
	if !errors.As(err, &ierr) || ierr.HTTPStatus != http.StatusBadRequest {
		return err
	}
	return &integration.Error{
		Kind:       integration.ErrAuth,
		Provider:   ierr.Provider,
		Op:         ierr.Op,
		HTTPStatus: ierr.HTTPStatus,
		RetryAfter: ierr.RetryAfter,
	}
}

// Verify exchanges a token and reports whether creds authenticate.
func (s *Source) Verify(ctx context.Context, creds integration.Credentials) error {
	_, err := s.token(ctx, creds)
	return err
}

// DiscoverMeteringPoints has nothing to call: 06 §3's Endpoints table lists
// no discovery/listing endpoint for GridBox — metering points are addressed
// directly by wiring number (installation_number), configured out of band,
// unlike OSOS's analyzers_list. It still exchanges a token, so a bad
// credential is reported the same way Verify reports it, but always
// returns zero points. R37 (verify in F14): this is a structural reading of
// 06 §3's own endpoint table, not an invented gap — but it has not been
// confirmed that GridBox metering points are never meant to be discoverable
// by some other mechanism outside that table.
func (s *Source) DiscoverMeteringPoints(ctx context.Context, creds integration.Credentials) ([]integration.MeteringPoint, error) {
	if _, err := s.token(ctx, creds); err != nil {
		return nil, err
	}
	return nil, nil
}

// FetchReadings is 06 §3's Flow, steps 1-2, for one kind: authenticate,
// bound the window from below with last_success_date, always fetch
// last_endex (multiplier priority 1, and — kind == current_index — the
// call's only data), then either map that single snapshot or page through
// the kind's windowed endpoint.
func (s *Source) FetchReadings(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	wiringNo := req.Point.InstallationNumber
	if err := fetchConfigError(creds, wiringNo); err != nil {
		return integration.FetchResult{}, err
	}

	token, err := s.token(ctx, creds)
	if err != nil {
		return integration.FetchResult{}, err
	}

	client := s.dataClient(creds)
	from, to := req.From, req.To

	lsdBody, err := s.call(ctx, client, token, "last_success_date", creds.Endpoints["last_success_date"],
		map[string]httpx.Param{"wiringNo": {Value: wiringNo}})
	if err != nil {
		return integration.FetchResult{}, err
	}
	lsd, decErr := decodeLastSuccessDate(lsdBody)
	if decErr != nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderGridBox, Op: "last_success_date"}
	}
	if lsd.LastSuccessDate != nil {
		if t, parseErr := normalize.ISO8601(*lsd.LastSuccessDate); parseErr == nil && t.After(from) {
			from = t
		}
	}

	lastEndexBody, err := s.call(ctx, client, token, "last_endex", creds.Endpoints["last_endex"],
		map[string]httpx.Param{"wiringNo": {Value: wiringNo}})
	if err != nil {
		return integration.FetchResult{}, err
	}
	lastEndex, decErr := decodeLastEndex(lastEndexBody)
	if decErr != nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderGridBox, Op: "last_endex"}
	}

	if !from.Before(to) {
		mult := ResolveMultiplier(&lastEndex, nil)
		return integration.FetchResult{ResolvedMultiplier: &mult}, nil
	}

	if req.Kind == model.ReadingKindCurrentIndex {
		return s.currentIndexResult(req, wiringNo, lastEndex, lastEndexBody, from, to), nil
	}

	return s.windowedResult(ctx, client, token, creds, req, wiringNo, lastEndex, from, to)
}

// call performs one GET through client, decoding the {ResultStatus,
// ResultObject} envelope (06 §3 rule 2) and returning ResultObject's raw
// bytes only when ResultStatus is present and == 1. A response whose
// top-level shape is not even the expected envelope — ResultStatus
// genuinely absent, whether because the key is missing, the whole body is
// `null`, or the key itself is `null` — is ErrMalformedPayload (adapter
// review pattern 6). A present-but-non-1 ResultStatus is R25:
// ErrUpstreamUnavailable, Op set to op, never an empty result.
func (s *Source) call(ctx context.Context, client *httpx.Client, token, op, template string, params map[string]httpx.Param) (json.RawMessage, error) {
	resp, err := client.Do(ctx, httpx.Request{
		Op:       op,
		Method:   http.MethodGet,
		Template: template,
		Params:   params,
		Header:   http.Header{"Authorization": []string{"Bearer " + token}},
	})
	if err != nil {
		return nil, err
	}

	env, decErr := decodeEnvelope(resp.Body)
	if decErr != nil || env.ResultStatus == nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderGridBox, Op: op, HTTPStatus: resp.Status}
	}
	if *env.ResultStatus != 1 {
		return nil, &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderGridBox, Op: op, HTTPStatus: resp.Status}
	}
	return env.ResultObject, nil
}

// currentIndexResult maps last_endex's single snapshot into (at most) one
// current_index reading — 06 §3 Flow step 2: "current_index → last_endex".
// raw is last_endex's own ResultObject bytes, kept verbatim on the reading
// (model.MeterReading.Raw — adapter review pattern 14: never a
// re-marshalled struct).
func (s *Source) currentIndexResult(req integration.FetchRequest, wiringNo string, lastEndex LastEndex, raw json.RawMessage, from, to time.Time) integration.FetchResult {
	mult := ResolveMultiplier(&lastEndex, nil)
	var warnings []integration.Warning
	if mult.Source == integration.MultiplierFallbackOne {
		warnings = append(warnings, fallbackWarning(wiringNo))
	}

	ts, field, tsErr := resolveTimestamp(lastEndex.register)
	if tsErr != nil {
		warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%s at row 0", nonEmptyOr(field, "timestamp"))})
		return integration.FetchResult{ResolvedMultiplier: &mult, Warnings: warnings}
	}
	if ts.Before(from) || !ts.Before(to) {
		return integration.FetchResult{ResolvedMultiplier: &mult, Warnings: warnings}
	}

	reading, mapErr := mapRegisters(lastEndex.register, mult.Value)
	if mapErr != nil {
		warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%v at row 0", mapErr)})
		return integration.FetchResult{ResolvedMultiplier: &mult, Warnings: warnings}
	}
	reading = s.stamp(reading, req, ts, mult.Value, raw)

	return integration.FetchResult{Readings: []model.MeterReading{reading}, ResolvedMultiplier: &mult, Warnings: warnings}
}

// pendingRow is one successfully timestamped, not-yet-mapped row collected
// by windowedResult, deferred until the multiplier is resolved (which, for
// load_profile, itself depends on rows collected in this same pass).
type pendingRow struct {
	ts  time.Time
	reg register
	raw json.RawMessage
}

// windowedResult is 06 §3 Flow step 2 for load_profile/daily/billing/reset:
// page splitWindow's windows (R20; Ruling R36 — NOT normalize.Chunk, which
// currently never produces a window wider than one Istanbul-local day
// regardless of max) through the kind's endpoint, up to pageBudget chunks,
// collecting rows before resolving the multiplier once (06 §3 "Multiplier
// resolution" priority 2 needs whatever load_profile rows this call saw)
// and mapping every row exactly once.
func (s *Source) windowedResult(ctx context.Context, client *httpx.Client, token string, creds integration.Credentials, req integration.FetchRequest, wiringNo string, lastEndex LastEndex, from, to time.Time) (integration.FetchResult, error) {
	chunks := splitWindow(from, to, maxWindow)
	op, isBilling := kindEndpoint(req.Kind)
	template := creds.Endpoints[op]

	var (
		pending  []pendingRow
		profiles []LoadProfileRow
		warnings []integration.Warning
	)

	for i, chunk := range chunks {
		if i >= s.pageBudget {
			next := chunk.From
			return s.finishWindowed(req, wiringNo, lastEndex, pending, profiles, warnings, &next), nil
		}

		// R37 (verify in F14): {endDate} is assumed INCLUSIVE of the named
		// day — chunk.To is this chunk's exclusive [from, to) upper bound
		// (a local midnight), so chunk.To.Add(-time.Nanosecond) formats as
		// the PRECEDING calendar day, the last day the chunk actually
		// covers. 06 §3 does not state whether GridBox's own {endDate}
		// query parameter is inclusive or exclusive; this has not been
		// verified against the real provider.
		params := map[string]httpx.Param{
			"wiringNo":  {Value: wiringNo},
			"startDate": {Value: formatIstanbulDate(chunk.From)},
			"endDate":   {Value: formatIstanbulDate(chunk.To.Add(-time.Nanosecond))},
		}
		if op == "endexes" {
			params["isBilling"] = httpx.Param{Value: strconv.FormatBool(isBilling)}
		}

		body, err := s.call(ctx, client, token, op, template, params)
		if err != nil {
			return integration.FetchResult{}, err
		}
		rawRows, decErr := decodeRegisterRows(body)
		if decErr != nil {
			return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderGridBox, Op: op}
		}

		for rowIdx, rawRow := range rawRows {
			r, rowErr := decodeRegisterRow(rawRow)
			if rowErr != nil {
				warnings = append(warnings, integration.Warning{
					Code:   integration.WarnUnparseableRow,
					Detail: fmt.Sprintf("row %d in chunk %d [%s,%s): %v", rowIdx, i, formatIstanbulDate(chunk.From), formatIstanbulDate(chunk.To), rowErr),
				})
				continue
			}
			ts, field, tsErr := resolveTimestamp(r)
			if tsErr != nil {
				warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%s at row %d in chunk %d", nonEmptyOr(field, "timestamp"), rowIdx, i)})
				continue
			}
			if req.Kind == model.ReadingKindLoadProfile {
				profiles = append(profiles, LoadProfileRow{register: r})
			}
			if ts.Before(from) || !ts.Before(to) {
				continue
			}
			pending = append(pending, pendingRow{ts: ts, reg: r, raw: rawRow})
		}
	}

	return s.finishWindowed(req, wiringNo, lastEndex, pending, profiles, warnings, nil), nil
}

// finishWindowed resolves the multiplier once from whatever this call
// collected, maps every pending row exactly once, and returns the merged,
// Ts-sorted result. A repeated Ts whose mapped registers differ from the
// first occurrence's is not silently dropped: the first occurrence is kept
// and a WarnUnparseableRow notes the conflicting duplicate (adapter review
// pattern 13) — a value-identical repeat (the ordinary case when a provider
// re-sends the same snapshot across overlapping windows) is deduplicated
// silently, since it carries no new information and no conflict.
func (s *Source) finishWindowed(req integration.FetchRequest, wiringNo string, lastEndex LastEndex, pending []pendingRow, profiles []LoadProfileRow, warnings []integration.Warning, next *time.Time) integration.FetchResult {
	mult := ResolveMultiplier(&lastEndex, profiles)
	if mult.Source == integration.MultiplierFallbackOne {
		warnings = append(warnings, fallbackWarning(wiringNo))
	}

	mapped := make([]model.MeterReading, 0, len(pending))
	for rowIdx, p := range pending {
		reading, err := mapRegisters(p.reg, mult.Value)
		if err != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%v at row %d", err, rowIdx)})
			continue
		}
		mapped = append(mapped, s.stamp(reading, req, p.ts, mult.Value, p.raw))
	}

	sort.SliceStable(mapped, func(i, j int) bool { return mapped[i].Ts.Before(mapped[j].Ts) })

	readings := make([]model.MeterReading, 0, len(mapped))
	for i, r := range mapped {
		if i > 0 && r.Ts.Equal(mapped[i-1].Ts) {
			if !readingRegistersEqual(mapped[i-1], r) {
				warnings = append(warnings, integration.Warning{
					Code:   integration.WarnUnparseableRow,
					Detail: fmt.Sprintf("duplicate timestamp %s with conflicting register values; kept the first", r.Ts.Format(time.RFC3339)),
				})
			}
			continue
		}
		readings = append(readings, r)
	}

	return integration.FetchResult{Readings: readings, ResolvedMultiplier: &mult, NextCursor: next, Warnings: warnings}
}

// readingRegistersEqual compares every register field of a and b (never Ts,
// Kind, MultiplierApplied, SourceProvider, IngestedAt or Raw, which a
// legitimate re-send of the same reading may render differently) for
// finishWindowed's duplicate-timestamp check.
func readingRegistersEqual(a, b model.MeterReading) bool {
	pairs := [][2]*decimal.Decimal{
		{a.ActiveImport, b.ActiveImport}, {a.ActiveExport, b.ActiveExport},
		{a.ReactiveInductiveImport, b.ReactiveInductiveImport}, {a.ReactiveCapacitiveImport, b.ReactiveCapacitiveImport},
		{a.ReactiveInductiveExport, b.ReactiveInductiveExport}, {a.ReactiveCapacitiveExport, b.ReactiveCapacitiveExport},
		{a.T1Import, b.T1Import}, {a.T2Import, b.T2Import}, {a.T3Import, b.T3Import},
		{a.T1Export, b.T1Export}, {a.T2Export, b.T2Export}, {a.T3Export, b.T3Export},
		{a.MaxDemandKw, b.MaxDemandKw},
	}
	for _, p := range pairs {
		if !decimalPtrEqual(p[0], p[1]) {
			return false
		}
	}
	return true
}

func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// stamp fills the fields the mapping table does not (mapRegisters' doc):
// AnalyzerID, Ts, Kind, MultiplierApplied, SourceProvider, IngestedAt, Raw.
func (s *Source) stamp(reading model.MeterReading, req integration.FetchRequest, ts time.Time, mult decimal.Decimal, raw json.RawMessage) model.MeterReading {
	reading.AnalyzerID = req.AnalyzerID
	reading.Ts = ts
	reading.Kind = req.Kind
	reading.MultiplierApplied = mult
	reading.SourceProvider = model.IntegrationProviderGridbox
	reading.IngestedAt = s.clock.Now()
	reading.Raw = json.RawMessage(append([]byte(nil), raw...))
	return reading
}

// fallbackWarning is 06 §3 "Multiplier resolution" priority 3's required
// warning, naming the wiring number (Task 7 brief's acceptance test: "res.
// Warnings contains exactly one WarnMultiplierFallback naming the wiring
// number").
func fallbackWarning(wiringNo string) integration.Warning {
	return integration.Warning{
		Code:   integration.WarnMultiplierFallback,
		Detail: fmt.Sprintf("wiring %s: no multiplier source available (no last_endex.Multiplier, no usable load-profile ratio); falling back to 1", wiringNo),
	}
}

// kindEndpoint is 06 §3 Flow step 2's kind -> endpoint table for the
// windowed kinds (current_index is handled separately, before this is
// consulted).
func kindEndpoint(kind model.ReadingKind) (op string, isBilling bool) {
	switch kind {
	case model.ReadingKindLoadProfile:
		return "load_profiles", false
	case model.ReadingKindDaily:
		return "endexes", false
	case model.ReadingKindBilling:
		return "endexes", true
	case model.ReadingKindReset:
		return "energy_values", false
	default:
		return "", false
	}
}

// formatIstanbulDate renders t as a GridBox {startDate}/{endDate}
// YYYY-MM-DD placeholder, in Europe/Istanbul local time (06 §3: "{startDate}
// / {endDate} are YYYY-MM-DD Istanbul local dates").
func formatIstanbulDate(t time.Time) string {
	return t.In(normalize.Istanbul).Format("2006-01-02")
}

// nonEmptyOr returns s, or fallback when s is empty — used so a
// WarnUnparseableRow's Detail always names a field even when resolveTimestamp
// found none of the four candidates present at all.
func nonEmptyOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// splitWindow splits [from, to) into contiguous half-open windows of at
// most max, each ending — where one fits — at an Europe/Istanbul local
// midnight, so a window's far boundary never falls mid-day. Ruling R36:
// this exists because normalize.Chunk's current implementation always
// produces single-Istanbul-day windows regardless of max (it computes
// "tomorrow's midnight" as its only candidate boundary and only shrinks
// from there), which is wrong for GridBox's 30-day MaxWindow — using it
// here would silently turn every fetch into one HTTP call per calendar
// day. splitWindow instead walks forward from each window's start,
// day-aligned, as far as max allows.
func splitWindow(from, to time.Time, max time.Duration) []normalize.Window {
	if !from.Before(to) || max <= 0 {
		return nil
	}
	var windows []normalize.Window
	cur := from
	for cur.Before(to) {
		end := latestMidnightWithinMax(cur, to, max)
		windows = append(windows, normalize.Window{From: cur, To: end})
		cur = end
	}
	return windows
}

// latestMidnightWithinMax returns the boundary for a window starting at
// cur: the latest Europe/Istanbul local midnight that is both strictly
// after cur and no later than min(cur+max, to). If not even one local
// midnight fits within that cap (max shorter than the remainder of cur's
// own local day, or to itself falls before the next midnight), the window
// simply ends at the cap — still respecting max and never crossing to.
func latestMidnightWithinMax(cur, to time.Time, max time.Duration) time.Time {
	limit := cur.Add(max)
	if limit.After(to) {
		limit = to
	}

	local := cur.In(normalize.Istanbul)
	y, m, d := local.Date()
	next := time.Date(y, m, d+1, 0, 0, 0, 0, normalize.Istanbul)
	if next.After(limit) {
		return limit
	}
	best := next
	for {
		candidate := best.AddDate(0, 0, 1)
		if candidate.After(limit) {
			return best
		}
		best = candidate
	}
}

// decodeEnvelope, decodeTokenResponse, decodeLastSuccessDate, decodeLastEndex,
// decodeRegisterRows and decodeRegisterRow each decode into one concrete,
// fully-typed local variable — never into `any`/`interface{}` — per
// TestIntegrationTreesDoNotParseFloats, which flags a json.Decoder.Decode
// call whose argument's static type is, or contains, float32/float64 or an
// interface. Each function therefore builds its own *json.Decoder and calls
// Decode directly against its own concretely-typed &v, rather than sharing
// one helper through an `any`-typed parameter (which the guard would flag
// regardless of what is actually passed at runtime, since the check is
// purely on the argument expression's static type). decodeRegisterRows
// decodes only the OUTER array, into []json.RawMessage (itself a []byte
// alias, not an interface) — never straight into []register — so ONE
// structurally-malformed row (adapter review pattern 7) cannot fail the
// whole page: decodeRegisterRow then decodes each element separately, and
// windowedResult turns a per-row failure into a WarnUnparseableRow instead
// of an ErrMalformedPayload for the entire call.

func decodeEnvelope(body []byte) (envelope, error) {
	var v envelope
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeTokenResponse(body []byte) (tokenResponse, error) {
	var v tokenResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeLastSuccessDate(body json.RawMessage) (lastSuccessDateResponse, error) {
	var v lastSuccessDateResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeLastEndex(body json.RawMessage) (LastEndex, error) {
	var v LastEndex
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeRegisterRows(body json.RawMessage) ([]json.RawMessage, error) {
	var v []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeRegisterRow(raw json.RawMessage) (register, error) {
	var v register
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}
