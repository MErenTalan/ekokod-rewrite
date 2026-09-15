// Package aril implements the ARIL provider adapter (06-integrations.md
// §4): metering points are subscriptions identified by SubscriptionSerno, a
// UserCode/Password login exchanges an aril-service-token good for every
// subsequent POST, and every response is an {ErrorCode, <list-or-object>}
// envelope (06 §4 does not name the envelope's own fields — this adapter
// follows task-8-brief.md's own "ErrorCode" vocabulary; see wire.go).
//
// ARIL never resolves its own meter multiplier the way GridBox does: 06 §4
// reports Multiplier once, on the subscription (discovery) row, and every
// FetchReadings call carries req.Multiplier — the pipeline's already-
// resolved copy of Analyzer.MeterMultiplier (Task 1's R3 field) — which
// this package applies exactly once, per register, via normalize.Multiply.
// owner_consumptions is asked NOT to apply its own multiplier
// (WithoutMultiplier: true) so client-side multiplication is the only one
// that ever happens; see fetchLoadProfile's doc for the one place that
// assumption lives (R9, verified against real responses only in F14).
//
// Window splitting (Ruling R36): unlike OSOS, GridBox and EPİAŞ —
// normalize/window.go's own doc names exactly those three as the providers
// for which a Window is "also the pagination unit" — 06 §4 documents no
// per-call window-size limit for owner_consumptions, current_endexes or
// end_of_month_endexes, and task-8-brief.md's MaxWindow (30 days) is
// already the pipeline's own cap on how wide a single FetchRequest's
// [From, To) is. This adapter therefore makes exactly ONE provider call per
// data endpoint per FetchReadings invocation, covering the given window
// directly — never splitting it with normalize.Chunk or a local splitter —
// and FetchResult.NextCursor is always nil (the call, by construction,
// always fully covers what it was asked for). This is the "justify a local
// splitter" branch of Ruling R36 and adapter review pattern 11: no
// splitting happens at all, because nothing in 06 or the brief requires
// it. ARIL's one genuine pagination mechanism is analyzers_list's own
// PageNumber paging (task-8-brief.md item 2, "which is the pagination
// case"), implemented in DiscoverMeteringPoints below and bounded by
// Options.PageBudget exactly like every other adapter's page budget.
package aril

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
	// maxWindow is task-8-brief.md's "MaxWindow 30 days": Planner.MaxWindow's
	// answer for every kind.
	maxWindow = 30 * 24 * time.Hour

	defaultPageBudget = 10

	// subscriptionPageSize is analyzers_list's fixed PageSize (06 §4 /
	// task-8-brief.md item 2: "PostNumber, PageSize: 1000").
	subscriptionPageSize = 1000

	// requestEvery/requestBurst/requestTimeout implement the Provider
	// defaults table's ARIL row: 30s timeout, 2 requests/s, burst 2.
	requestEvery   = 500 * time.Millisecond
	requestBurst   = 2
	requestTimeout = 30 * time.Second

	// istanbulDateTimeLayout is owner_consumptions'/current_endexes'
	// StartDate/EndDate wire format (task-8-brief.md item 3: "YYYY-MM-DD
	// HH:mm:ss Istanbul").
	istanbulDateTimeLayout = "2006-01-02 15:04:05"
)

// Options configures a Source.
type Options struct {
	// Clock is used to stamp MeterReading.IngestedAt. nil: clock.System().
	Clock clock.Clock
	// PageBudget bounds how many analyzers_list pages one
	// DiscoverMeteringPoints call processes. Default 10.
	PageBudget int
}

// Source is the ARIL integration.Adapter.
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

// Provider reports integration.ProviderARIL.
func (s *Source) Provider() integration.Provider { return integration.ProviderARIL }

// Kinds is task-8-brief.md's "Kinds -> [load_profile, current_index,
// billing]", always — ARIL has no per-credential toggle the way GridBox's
// billing kind does.
func (s *Source) Kinds(integration.Credentials) []model.ReadingKind {
	return []model.ReadingKind{
		model.ReadingKindLoadProfile,
		model.ReadingKindCurrentIndex,
		model.ReadingKindBilling,
	}
}

// MaxWindow is 30 days for every kind (task-8-brief.md).
func (s *Source) MaxWindow(model.ReadingKind) time.Duration { return maxWindow }

// configError reports a deliberately-classified configuration problem: a
// missing/blank endpoint template, a blank OwnerSerno, a missing
// DefinitionType or a zero meter multiplier. None of these is a payload
// ARIL ever sent — ErrMalformedPayload would be the wrong Kind (adapter
// review pattern 9). R48/I5: this used to report ErrAuth (round 1's
// documented stop-gap, before integration.ErrConfig existed) — an
// incomplete credential/request configuration is non-retryable like a
// rejected password, but it is NOT an authentication failure, and F3's
// credential-health logic must never treat it as one. Use ErrConfig.
func configError(op string) error {
	return &integration.Error{Kind: integration.ErrConfig, Provider: integration.ProviderARIL, Op: op}
}

func authConfigError(creds integration.Credentials) error {
	if strings.TrimSpace(creds.Endpoints["authentication"]) == "" {
		return configError("config:authentication")
	}
	return nil
}

func discoverConfigError(creds integration.Credentials) error {
	for _, key := range []string{"authentication", "analyzers_list"} {
		if strings.TrimSpace(creds.Endpoints[key]) == "" {
			return configError("config:" + key)
		}
	}
	return nil
}

// fetchConfigError validates every endpoint one FetchReadings kind needs,
// the OwnerSerno (installation number), the DefinitionType (OwnerType) ARIL
// requires on every data call, and a non-zero multiplier (adapter review
// pattern 12) — all before any HTTP call is attempted.
func fetchConfigError(creds integration.Credentials, req integration.FetchRequest, endpointKeys []string) error {
	for _, key := range endpointKeys {
		if strings.TrimSpace(creds.Endpoints[key]) == "" {
			return configError("config:" + key)
		}
	}
	if strings.TrimSpace(req.Point.InstallationNumber) == "" {
		return configError("config:ownerSerno")
	}
	if req.Point.DefinitionType == nil {
		return configError("config:definitionType")
	}
	if req.Multiplier.IsZero() {
		return configError("config:multiplier")
	}
	return nil
}

// authClient is the one Client the authentication call is built from: its
// own LimiterKey and SerializeKey, per company — two concurrent jobs must
// never race two logins against the same credential (06 §6's iSolar
// refresh-token precedent: "serialised per company with a lock so
// concurrent jobs cannot both refresh and invalidate each other's token").
func (s *Source) authClient(creds integration.Credentials) *httpx.Client {
	return s.pool.Client(httpx.ClientConfig{
		Provider:       integration.ProviderARIL,
		LimiterKey:     "aril:auth:" + creds.CompanyID.String(),
		Every:          requestEvery,
		Burst:          requestBurst,
		RequestTimeout: requestTimeout,
		SerializeKey:   "aril:auth:" + creds.CompanyID.String(),
	})
}

// dataClientConfig builds the ClientConfig every non-authentication call
// uses (dataClient wraps it; a plain function so an internal test can
// assert its fields directly, with no Pool/Source needed — mirrors
// GridBox's own dataClientConfig, gridbox/source.go).
//
// Fix round 1 (I1): provider-defaults.md's `aril` row marks "Serialise per
// company: yes" for the whole provider, not only the authentication
// exchange — the original round gave authClient a SerializeKey but left
// dataClient (every analyzers_list/owner_consumptions/current_endexes/
// end_of_month_endexes call) unserialised, so two concurrent jobs for the
// same company could still race two data calls against each other.
// dataClient now carries its own SerializeKey, scoped to the company like
// its LimiterKey — deliberately the SAME key as LimiterKey's own
// "aril:<company>" string, distinct only from authClient's own
// "aril:auth:<company>" key. authClient keeps its own, separate key rather
// than sharing this one (mirrors GridBox's tokenClient/dataClientConfig
// split, gridbox/source.go): a login exchange and a data call are different
// operations with different retry rules (authenticate: R32 NoRetry, exactly
// one attempt; data calls: the Client's normal jittered retries), and
// merging their locks would have an in-flight login block every data call
// for the same company (and vice versa) for no reason 06 §4 or 06 §6's
// iSolar refresh-token precedent asks for — only calls of the SAME kind for
// the SAME company must never race each other.
func dataClientConfig(creds integration.Credentials) httpx.ClientConfig {
	key := "aril:" + creds.CompanyID.String()
	return httpx.ClientConfig{
		Provider:       integration.ProviderARIL,
		LimiterKey:     key,
		Every:          requestEvery,
		Burst:          requestBurst,
		RequestTimeout: requestTimeout,
		SerializeKey:   key,
	}
}

// dataClient is the Client every non-authentication call is built from.
func (s *Source) dataClient(creds integration.Credentials) *httpx.Client {
	return s.pool.Client(dataClientConfig(creds))
}

// authenticate exchanges creds for an aril-service-token (06 §4: "POST
// {UserCode, Password}"). This is the ONE call site every ARIL login goes
// through — see authClient's doc above and the NoRetry comment below (R32).
func (s *Source) authenticate(ctx context.Context, creds integration.Credentials) (string, error) {
	if err := authConfigError(creds); err != nil {
		return "", err
	}

	body, err := json.Marshal(authRequest{UserCode: creds.Username, Password: creds.Secret.Reveal()})
	if err != nil {
		return "", &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "authentication"}
	}

	// R32: exactly one attempt. This is a non-idempotent login POST; an
	// in-client retry after a network blip could race a concurrent login
	// against the same credential rather than simply repeating a read.
	resp, err := s.authClient(creds).Do(ctx, httpx.Request{
		Op:          "authentication",
		Method:      http.MethodPost,
		Template:    creds.Endpoints["authentication"],
		Body:        body,
		ContentType: "application/json",
		NoRetry:     true,
	})
	if err != nil {
		return "", err
	}

	token, decErr := decodeToken(resp.Body)
	if decErr != nil {
		return "", &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "authentication", HTTPStatus: resp.Status}
	}
	if token == "" {
		// 06 §4 / task-8-brief.md item 1: "A 401 or an empty token is
		// ErrAuth." A 401 is already ErrAuth via httpx's own status
		// classification (Do never reaches here for one); this covers a
		// 200 whose decoded token is the empty string.
		return "", &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderARIL, Op: "authentication", HTTPStatus: resp.Status}
	}
	return token, nil
}

// authHeader is 06 §4's "Authentication header: aril-service-token:
// <token>", sent on every call after authenticate.
func authHeader(token string) http.Header {
	return http.Header{"aril-service-token": []string{token}}
}

// Verify exchanges a token and reports whether creds authenticate.
func (s *Source) Verify(ctx context.Context, creds integration.Credentials) error {
	_, err := s.authenticate(ctx, creds)
	return err
}

// DiscoverMeteringPoints pages analyzers_list (06 §4 "Discover" /
// task-8-brief.md item 2): pages advance until a page has fewer than
// subscriptionPageSize rows or Options.PageBudget is hit — task-8-brief.md
// item 2's own words, "which is the pagination case", is why this is the
// call the fixture matrix's "pagination" subtest exercises (see
// source_test.go), not FetchReadings (which never internally paginates —
// see this file's package doc).
//
// A row that fails to decode, or whose SubscriptionSerno cannot be parsed,
// is silently skipped: this method's signature ([]MeteringPoint, error)
// carries no per-row Warning channel the way FetchResult does.
func (s *Source) DiscoverMeteringPoints(ctx context.Context, creds integration.Credentials) ([]integration.MeteringPoint, error) {
	if err := discoverConfigError(creds); err != nil {
		return nil, err
	}
	token, err := s.authenticate(ctx, creds)
	if err != nil {
		return nil, err
	}

	client := s.dataClient(creds)
	var points []integration.MeteringPoint

	for page := 1; page <= s.pageBudget; page++ {
		body, marshalErr := json.Marshal(subscriptionsRequest{PageNumber: page, PageSize: subscriptionPageSize})
		if marshalErr != nil {
			return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "analyzers_list"}
		}

		resp, callErr := client.Do(ctx, httpx.Request{
			Op:          "analyzers_list",
			Method:      http.MethodPost,
			Template:    creds.Endpoints["analyzers_list"],
			Body:        body,
			ContentType: "application/json",
			Header:      authHeader(token),
		})
		if callErr != nil {
			return nil, callErr
		}

		sr, decErr := decodeSubscriptions(resp.Body)
		if decErr != nil {
			return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "analyzers_list", HTTPStatus: resp.Status}
		}
		if code, ok := errorCode(sr.ErrorCode); ok && code != 0 {
			// task-8-brief.md item 6 / removed-behaviour 27: an ErrorCode
			// on a data call is a real upstream failure, never an empty
			// result.
			return nil, &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderARIL, Op: "analyzers_list", HTTPStatus: resp.Status}
		}
		if sr.ResultList == nil {
			return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "analyzers_list", HTTPStatus: resp.Status}
		}

		rows := *sr.ResultList
		for _, raw := range rows {
			if pt, ok := mapSubscription(raw); ok {
				points = append(points, pt)
			}
		}
		if len(rows) < subscriptionPageSize {
			return points, nil
		}
	}
	// M8: the loop exhausted Options.PageBudget while the LAST page fetched
	// was still full (subscriptionPageSize rows) — the provider-shape
	// signal that more pages likely exist. Round 1 silently returned the
	// partial `points` here; align with iSolar's R44 (isolar/client.go's
	// pageBudgetExceeded doc): budget exhaustion is a deliberately-
	// classified, non-retryable error, never a quiet truncation an operator
	// has no way to notice.
	return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "analyzers_list"}
}

// FetchReadings dispatches to the endpoint task-8-brief.md's items 3-5 name
// for req.Kind.
func (s *Source) FetchReadings(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	switch req.Kind {
	case model.ReadingKindLoadProfile:
		return s.fetchLoadProfile(ctx, creds, req)
	case model.ReadingKindCurrentIndex:
		return s.fetchCurrentIndex(ctx, creds, req)
	case model.ReadingKindBilling:
		return s.fetchBilling(ctx, creds, req)
	default:
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "fetch_readings:" + string(req.Kind)}
	}
}

// fetchLoadProfile is task-8-brief.md item 3: POST owner_consumptions and
// map every LoadProfiles[] row.
//
// WithoutMultiplier: true is always sent — this is the ONE place that
// assumption lives (Ruling R9). The plan notes ARIL's WithoutMultiplier
// semantics are unverified against a real response and must be checked in
// F14; this adapter implements exactly what task-8-brief.md states
// (WithoutMultiplier: true, multiply client-side with req.Multiplier
// exactly once) and isolates the assumption here so F14 has one place to
// revisit if the real service does not honour it the way this expects.
func (s *Source) fetchLoadProfile(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	if err := fetchConfigError(creds, req, []string{"authentication", "owner_consumptions"}); err != nil {
		return integration.FetchResult{}, err
	}
	token, err := s.authenticate(ctx, creds)
	if err != nil {
		return integration.FetchResult{}, err
	}

	body, marshalErr := json.Marshal(consumptionsRequest{
		OwnerSerno:          req.Point.InstallationNumber,
		StartDate:           formatIstanbulDateTime(req.From),
		EndDate:             formatIstanbulDateTime(req.To),
		IncludeLoadProfiles: true,
		OwnerType:           *req.Point.DefinitionType,
		WithoutMultiplier:   true, // R9/F14 — see doc above.
		MergeResult:         false,
	})
	if marshalErr != nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "owner_consumptions"}
	}

	resp, callErr := s.dataClient(creds).Do(ctx, httpx.Request{
		Op:          "owner_consumptions",
		Method:      http.MethodPost,
		Template:    creds.Endpoints["owner_consumptions"],
		Body:        body,
		ContentType: "application/json",
		Header:      authHeader(token),
	})
	if callErr != nil {
		return integration.FetchResult{}, callErr
	}

	cr, decErr := decodeConsumptions(resp.Body)
	if decErr != nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "owner_consumptions", HTTPStatus: resp.Status}
	}
	if code, ok := errorCode(cr.ErrorCode); ok && code != 0 {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderARIL, Op: "owner_consumptions", HTTPStatus: resp.Status}
	}
	if cr.LoadProfiles == nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: "owner_consumptions", HTTPStatus: resp.Status}
	}

	var readings []model.MeterReading
	var warnings []integration.Warning
	for i, raw := range *cr.LoadProfiles {
		var row loadProfileRow
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if decErr := dec.Decode(&row); decErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("row %d in owner_consumptions: %v", i, decErr)})
			continue
		}
		ts, tsErr := parseProfileDate(row.ProfileDate)
		if tsErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("ProfileDate at row %d", i)})
			continue
		}
		if ts.Before(req.From) || !ts.Before(req.To) {
			continue
		}
		reading, mapErr := mapRegisters(row.registers, req.Multiplier)
		if mapErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%v at row %d", mapErr, i)})
			continue
		}
		readings = append(readings, s.stamp(reading, req, ts, raw))
	}

	readings, dupWarnings := dedupeByTimestamp(readings)
	warnings = append(warnings, dupWarnings...)

	return integration.FetchResult{Readings: readings, Warnings: warnings}, nil
}

// endexRowWithRaw is one decoded current_endexes/end_of_month_endexes row
// plus its own original bytes (adapter review pattern 14: Raw is never a
// re-marshalled struct).
type endexRowWithRaw struct {
	wire endexRow
	raw  json.RawMessage
}

// callEndexes issues one POST to op (current_endexes or
// end_of_month_endexes: task-8-brief.md items 4-5), decodes its ResultList
// and returns each row already unmarshalled with its original bytes. A
// single structurally-malformed row is isolated as a WarnUnparseableRow
// (adapter review pattern 7), never failing the whole call.
func (s *Source) callEndexes(ctx context.Context, token string, creds integration.Credentials, req integration.FetchRequest, op string) ([]endexRowWithRaw, []integration.Warning, error) {
	body, marshalErr := json.Marshal(endexesRequest{
		OwnerSerno:     req.Point.InstallationNumber,
		StartDate:      formatIstanbulDateTime(req.From),
		EndDate:        formatIstanbulDateTime(req.To),
		DefinitionType: *req.Point.DefinitionType,
		EndexDirection: 0,
	})
	if marshalErr != nil {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: op}
	}

	resp, callErr := s.dataClient(creds).Do(ctx, httpx.Request{
		Op:          op,
		Method:      http.MethodPost,
		Template:    creds.Endpoints[op],
		Body:        body,
		ContentType: "application/json",
		Header:      authHeader(token),
	})
	if callErr != nil {
		return nil, nil, callErr
	}

	er, decErr := decodeEndexes(resp.Body)
	if decErr != nil {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: op, HTTPStatus: resp.Status}
	}
	if code, ok := errorCode(er.ErrorCode); ok && code != 0 {
		return nil, nil, &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderARIL, Op: op, HTTPStatus: resp.Status}
	}
	if er.ResultList == nil {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderARIL, Op: op, HTTPStatus: resp.Status}
	}

	var rows []endexRowWithRaw
	var warnings []integration.Warning
	for i, raw := range *er.ResultList {
		var row endexRow
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if decErr := dec.Decode(&row); decErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("row %d in %s: %v", i, op, decErr)})
			continue
		}
		rows = append(rows, endexRowWithRaw{wire: row, raw: raw})
	}
	return rows, warnings, nil
}

// fetchCurrentIndex is task-8-brief.md item 4: POST current_endexes with
// EndexDirection: 0 and map every row, including its own MaxDemand.
func (s *Source) fetchCurrentIndex(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	if err := fetchConfigError(creds, req, []string{"authentication", "current_endexes"}); err != nil {
		return integration.FetchResult{}, err
	}
	token, err := s.authenticate(ctx, creds)
	if err != nil {
		return integration.FetchResult{}, err
	}

	rows, warnings, err := s.callEndexes(ctx, token, creds, req, "current_endexes")
	if err != nil {
		return integration.FetchResult{}, err
	}

	var readings []model.MeterReading
	for i, row := range rows {
		ts, tsErr := parseProfileDate(row.wire.ProfileDate)
		if tsErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("ProfileDate at row %d", i)})
			continue
		}
		if ts.Before(req.From) || !ts.Before(req.To) {
			continue
		}
		reading, mapErr := mapRegisters(row.wire.registers, req.Multiplier)
		if mapErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%v at row %d", mapErr, i)})
			continue
		}
		md, mdErr := mapMaxDemand(row.wire.MaxDemand, req.Multiplier)
		if mdErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("MaxDemand at row %d", i)})
		} else {
			reading.MaxDemandKw = md
		}
		readings = append(readings, s.stamp(reading, req, ts, row.raw))
	}

	readings, dupWarnings := dedupeByTimestamp(readings)
	warnings = append(warnings, dupWarnings...)

	return integration.FetchResult{Readings: readings, Warnings: warnings}, nil
}

// monthKey identifies one Europe/Istanbul local calendar month, for
// fetchBilling's max-demand aggregation (06 §4 "Max demand": "the maximum
// across that month's records").
type monthKey struct {
	year  int
	month time.Month
}

// istanbulMonthBounds returns the half-open [start, end) Europe/Istanbul
// local calendar month t falls in, both instants built from time.Date on
// t's Istanbul-local Y/M — never by adding a fixed duration — the same
// drift-free construction normalize.Chunk uses for its own boundaries.
func istanbulMonthBounds(t time.Time) (start, end time.Time) {
	local := t.In(normalize.Istanbul)
	y, m, _ := local.Date()
	start = time.Date(y, m, 1, 0, 0, 0, 0, normalize.Istanbul)
	return start, start.AddDate(0, 1, 0)
}

// maxDemandDate resolves the timestamp current_endexes' own MaxDemand value
// is keyed to a month by: MaxDemandDate when present, falling back to
// ProfileDate. Legacy arilService.ts:143 keys its own monthly aggregation
// by "endex.MaxDemandDate || endex.EndexDate" (bcem-energy's ARIL row has
// no separate EndexDate here — ProfileDate is this wire shape's equivalent
// per-row date) — R49.
func maxDemandDate(row endexRow) (time.Time, error) {
	if row.MaxDemandDate != nil {
		if ts, err := parseProfileDate(*row.MaxDemandDate); err == nil {
			return ts, nil
		}
	}
	return parseProfileDate(row.ProfileDate)
}

// fetchBilling is task-8-brief.md item 5, fixed per R49 (C1): POST
// end_of_month_endexes for the request window, then — a second call within
// this same FetchReadings — POST current_endexes for the WHOLE
// Istanbul-local calendar month(s) the in-window end_of_month_endexes
// readings fall in, independent of req.From/req.To, and set max_demand_kw
// on each to the maximum MaxDemand among current_endexes rows in that same
// month (06 §4 "Max demand").
//
// R49: a cursor-driven resume fetch's window starts AT (inclusive) the
// stored end_of_month_endexes reading's own Ts — the SAME reading is
// requested again every run until the next one exists. Bounding the second
// call by req.From/req.To (round 1's bug) then excludes every
// current_endexes row EARLIER in that reading's month, so the computed max
// silently drops (to a partial-month value, or to no value at all), and
// store/postgres's upsert (`max_demand_kw = excluded.max_demand_kw`)
// replaces the previously-stored maximum with that smaller/NULL value on
// every single resumed run. Widening the second call to the WHOLE month
// makes it see the SAME current_endexes rows every time, regardless of
// where req.From happens to fall — the fix generalises to the first-ever
// 30-day-lookback fetch and to backfill windows (normalize.Chunk, not
// month-aligned) the same way, since both are also bounded by an arbitrary
// req.From that need not align with a calendar month start.
func (s *Source) fetchBilling(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	if err := fetchConfigError(creds, req, []string{"authentication", "end_of_month_endexes", "current_endexes"}); err != nil {
		return integration.FetchResult{}, err
	}
	token, err := s.authenticate(ctx, creds)
	if err != nil {
		return integration.FetchResult{}, err
	}

	eomRows, warnings, err := s.callEndexes(ctx, token, creds, req, "end_of_month_endexes")
	if err != nil {
		return integration.FetchResult{}, err
	}

	// Determine the Istanbul month(s) covered by the IN-WINDOW
	// end_of_month_endexes readings only — a row outside [req.From, req.To)
	// is dropped below anyway and must not widen the current_endexes call
	// for a month nothing in this result will ever report.
	var monthFrom, monthTo time.Time
	haveMonth := false
	for _, row := range eomRows {
		ts, tsErr := parseProfileDate(row.wire.ProfileDate)
		if tsErr != nil || ts.Before(req.From) || !ts.Before(req.To) {
			continue
		}
		start, end := istanbulMonthBounds(ts)
		if !haveMonth {
			monthFrom, monthTo, haveMonth = start, end, true
			continue
		}
		if start.Before(monthFrom) {
			monthFrom = start
		}
		if end.After(monthTo) {
			monthTo = end
		}
	}

	var (
		mdRows  []endexRowWithRaw
		mdWarns []integration.Warning
	)
	if haveMonth {
		mdReq := req
		mdReq.From, mdReq.To = monthFrom, monthTo
		var callErr error
		mdRows, mdWarns, callErr = s.callEndexes(ctx, token, creds, mdReq, "current_endexes")
		if callErr != nil {
			return integration.FetchResult{}, callErr
		}
		warnings = append(warnings, mdWarns...)
	}

	maxByMonth := map[monthKey]decimal.Decimal{}
	for i, row := range mdRows {
		ts, tsErr := maxDemandDate(row.wire)
		if tsErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("MaxDemandDate/ProfileDate at current_endexes row %d", i)})
			continue
		}
		if ts.Before(monthFrom) || !ts.Before(monthTo) {
			continue
		}
		md, mdErr := mapMaxDemand(row.wire.MaxDemand, req.Multiplier)
		if mdErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("MaxDemand at current_endexes row %d", i)})
			continue
		}
		if md == nil {
			continue
		}
		local := ts.In(normalize.Istanbul)
		key := monthKey{local.Year(), local.Month()}
		if cur, ok := maxByMonth[key]; !ok || md.GreaterThan(cur) {
			maxByMonth[key] = *md
		}
	}

	var readings []model.MeterReading
	for i, row := range eomRows {
		ts, tsErr := parseProfileDate(row.wire.ProfileDate)
		if tsErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("ProfileDate at end_of_month_endexes row %d", i)})
			continue
		}
		if ts.Before(req.From) || !ts.Before(req.To) {
			continue
		}
		reading, mapErr := mapRegisters(row.wire.registers, req.Multiplier)
		if mapErr != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%v at row %d", mapErr, i)})
			continue
		}
		local := ts.In(normalize.Istanbul)
		md, ok := maxByMonth[monthKey{local.Year(), local.Month()}]
		if !ok {
			// R49: never emit MaxDemandKw=nil over what may be a stored
			// value — store/postgres's upsert has no way to tell "no new
			// information" from "the max demand is now unknown". Skip the
			// whole reading rather than let the pipeline persist a NULL.
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("no current_endexes max demand for %04d-%02d at end_of_month_endexes row %d — reading skipped", local.Year(), local.Month(), i)})
			continue
		}
		mdCopy := md
		reading.MaxDemandKw = &mdCopy
		readings = append(readings, s.stamp(reading, req, ts, row.raw))
	}

	readings, dupWarnings := dedupeByTimestamp(readings)
	warnings = append(warnings, dupWarnings...)

	return integration.FetchResult{Readings: readings, Warnings: warnings}, nil
}

// stamp fills every field mapRegisters does not (its doc): AnalyzerID, Ts,
// Kind, MultiplierApplied, SourceProvider, IngestedAt, MeterSerial and Raw.
// MeterSerial is carried forward from req.Point.MeterNumber (the discovered
// subscription's MeterSerial, 06 §4 "Subscription mapping") since none of
// ARIL's data endpoints report a per-row meter serial.
func (s *Source) stamp(reading model.MeterReading, req integration.FetchRequest, ts time.Time, raw json.RawMessage) model.MeterReading {
	reading.AnalyzerID = req.AnalyzerID
	reading.Ts = ts
	reading.Kind = req.Kind
	reading.MultiplierApplied = req.Multiplier
	reading.SourceProvider = model.IntegrationProviderARIL
	reading.IngestedAt = s.clock.Now()
	reading.MeterSerial = req.Point.MeterNumber
	reading.Raw = json.RawMessage(append([]byte(nil), raw...))
	return reading
}

// dedupeByTimestamp sorts readings by Ts and collapses a repeated Ts to its
// first occurrence: a value-identical repeat is dropped silently (no new
// information), while a repeat whose registers differ from the first
// occurrence's produces a WarnUnparseableRow noting the conflict (adapter
// review pattern 13) rather than being silently dropped or overwriting the
// first.
func dedupeByTimestamp(readings []model.MeterReading) ([]model.MeterReading, []integration.Warning) {
	sort.SliceStable(readings, func(i, j int) bool { return readings[i].Ts.Before(readings[j].Ts) })

	var warnings []integration.Warning
	out := make([]model.MeterReading, 0, len(readings))
	for i, r := range readings {
		if i > 0 && r.Ts.Equal(readings[i-1].Ts) {
			if !readingRegistersEqual(readings[i-1], r) {
				warnings = append(warnings, integration.Warning{
					Code:   integration.WarnUnparseableRow,
					Detail: fmt.Sprintf("duplicate timestamp %s with conflicting register values; kept the first", r.Ts.Format(time.RFC3339)),
				})
			}
			continue
		}
		out = append(out, r)
	}
	return out, warnings
}

// formatIstanbulDateTime renders t as ARIL's StartDate/EndDate placeholder,
// "YYYY-MM-DD HH:mm:ss" Europe/Istanbul local (task-8-brief.md item 3).
func formatIstanbulDateTime(t time.Time) string {
	return t.In(normalize.Istanbul).Format(istanbulDateTimeLayout)
}

// decodeToken, decodeSubscriptions, decodeConsumptions and decodeEndexes
// each decode into one concrete, fully-typed local variable — never into
// `any`/`interface{}` — per TestIntegrationTreesDoNotParseFloats.
// decodeToken's own two-shape logic (06 §4: "the token comes back as a
// bare string or as {access_token}") first tries the bare-string shape,
// falling back to the object shape only when that fails; both targets are
// concretely typed (string, tokenObject), never `any`.

func decodeToken(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)

	var bare string
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&bare); err == nil {
		return bare, nil
	}

	var obj tokenObject
	dec2 := json.NewDecoder(bytes.NewReader(trimmed))
	dec2.UseNumber()
	if err := dec2.Decode(&obj); err != nil {
		return "", err
	}
	return obj.AccessToken, nil
}

func decodeSubscriptions(body []byte) (subscriptionsResponse, error) {
	var v subscriptionsResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeConsumptions(body []byte) (consumptionsResponse, error) {
	var v consumptionsResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}

func decodeEndexes(body []byte) (endexesResponse, error) {
	var v endexesResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	err := dec.Decode(&v)
	return v, err
}
