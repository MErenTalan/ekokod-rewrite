// Package osos is the F2 provider adapter for OSOS (Otomatik Sayaç Okuma
// Sistemi), the shared protocol behind several Turkish distribution
// companies (Baskent, Aydem, Meramedas, ...). See docs/rewrite/06-
// integrations.md §2 for the authoritative field tables this package
// implements against.
//
// This package is pure (06 §1 rule 2): it makes no store, pgx or
// internal/ingest import, directly or transitively — TestAdaptersDoNotImportTheStore
// (internal/arch) enforces this. Every network call goes through the
// *httpx.Pool passed to New; nothing here builds an *http.Client.
package osos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// defaultPageBudget caps how many provider HTTP calls (energy_values window
// chunks, plus hourly_values calendar-month calls) one FetchReadings call
// will make before returning a NextCursor for the caller to resume from,
// per the adapter template's Options.PageBudget.
const defaultPageBudget = 10

// Options configures a Source.
type Options struct {
	// Clock is consulted nowhere in this adapter's own logic today (OSOS's
	// windows are entirely caller-supplied via FetchRequest.From/To), but is
	// part of every adapter's Options per the shared template so a future
	// clock-dependent behaviour (e.g. a "now" default) never needs a
	// constructor signature change. nil: clock.System().
	Clock clock.Clock
	// PageBudget caps provider HTTP calls per FetchReadings call. 0 (or
	// negative): defaultPageBudget.
	PageBudget int
}

// Source is the OSOS integration.Adapter.
type Source struct {
	pool       *httpx.Pool
	clock      clock.Clock
	pageBudget int
}

var _ integration.Adapter = (*Source)(nil)

// New builds a Source. pool is the only seam this adapter uses to reach the
// network (06 §1 rule 2 / global constraint: "adapters never accept an
// *http.Client").
func New(pool *httpx.Pool, o Options) *Source {
	c := o.Clock
	if c == nil {
		c = clock.System()
	}
	budget := o.PageBudget
	if budget <= 0 {
		budget = defaultPageBudget
	}
	return &Source{pool: pool, clock: c, pageBudget: budget}
}

// Provider identifies this adapter to the registry and to every
// integration.Error it returns.
func (s *Source) Provider() integration.Provider { return integration.ProviderOSOS }

// Kinds is 06 §2 / the Provider defaults table: OSOS is fetched for
// load_profile, daily and reset. Hourly values are a cross-check series
// fetched alongside load_profile calls (see FetchReadings), never a kind of
// their own.
func (s *Source) Kinds(_ integration.Credentials) []model.ReadingKind {
	return []model.ReadingKind{model.ReadingKindLoadProfile, model.ReadingKindDaily, model.ReadingKindReset}
}

// MaxWindow is the Provider defaults table: 30 days for every kind OSOS
// serves (hourly_values, fetched inside a load_profile FetchReadings call,
// is chunked separately by calendar month — see monthsIntersecting).
func (s *Source) MaxWindow(_ model.ReadingKind) time.Duration { return 30 * 24 * time.Hour }

// client builds this call's *httpx.Client per the Provider defaults table:
// 30s request timeout, 1 request/s with burst 1, serialised per company
// ("Serialise per company: yes (spec)").
func (s *Source) client(creds integration.Credentials) *httpx.Client {
	key := "osos:" + creds.CompanyID.String()
	return s.pool.Client(httpx.ClientConfig{
		Provider:       integration.ProviderOSOS,
		LimiterKey:     key,
		Every:          time.Second,
		Burst:          1,
		RequestTimeout: 30 * time.Second,
		SerializeKey:   key,
	})
}

// authenticate is OSOS's ONE login call (06 §2 "Authenticate"), routed
// through here so this is also the ONE spot that needs to change for R32.
// The token this returns is held by the caller for the lifetime of one
// FetchReadings/Discover/Verify call only — Source never caches it on
// itself.
func (s *Source) authenticate(ctx context.Context, cl *httpx.Client, creds integration.Credentials) (string, error) {
	tmpl, ok := creds.Endpoints["authentication"]
	if !ok {
		return "", &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "authentication"}
	}

	// R32 (controller ruling, Task 2 review): authentication is a
	// non-idempotent login POST and must not be retried in-client. Set
	// NoRetry: true here once httpx.Request.NoRetry lands (Task 2 fix
	// round 1) — it does not exist on this adapter's base yet.
	resp, err := cl.Do(ctx, httpx.Request{
		Op:       "authentication",
		Method:   http.MethodPost,
		Template: tmpl,
		Params: map[string]httpx.Param{
			"username_or_email": {Value: creds.Username},
			"secret_password":   {Value: creds.Secret.Reveal(), Secret: true},
		},
	})
	if err != nil {
		return "", err
	}

	var body authResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return "", &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "authentication", HTTPStatus: resp.Status}
	}
	if body.AccessToken == "" {
		return "", &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderOSOS, Op: "authentication", HTTPStatus: resp.Status}
	}
	return body.AccessToken, nil
}

// Verify authenticates and discards the token: 06's interface asks only
// whether the credential works.
func (s *Source) Verify(ctx context.Context, creds integration.Credentials) error {
	cl := s.client(creds)
	_, err := s.authenticate(ctx, cl, creds)
	return err
}

// DiscoverMeteringPoints authenticates, then GETs analyzers_list and maps
// every row (06 §2 "Discover").
func (s *Source) DiscoverMeteringPoints(ctx context.Context, creds integration.Credentials) ([]integration.MeteringPoint, error) {
	cl := s.client(creds)
	token, err := s.authenticate(ctx, cl, creds)
	if err != nil {
		return nil, err
	}

	tmpl, ok := creds.Endpoints["analyzers_list"]
	if !ok {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "analyzers_list"}
	}

	resp, err := cl.Do(ctx, httpx.Request{
		Op:       "analyzers_list",
		Method:   http.MethodGet,
		Template: tmpl,
		Params: map[string]httpx.Param{
			"secret_token": {Value: token, Secret: true},
		},
	})
	if err != nil {
		return nil, err
	}

	var body discoverResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "analyzers_list", HTTPStatus: resp.Status}
	}

	points := make([]integration.MeteringPoint, 0, len(body.InstalationList))
	for _, row := range body.InstalationList {
		pt, err := mapInstallation(row)
		if err != nil {
			return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "analyzers_list", HTTPStatus: resp.Status}
		}
		points = append(points, pt)
	}
	return points, nil
}

// dataTypeParam is 06 §2 / task-6-brief.md's energy_values dataType
// parameter: load_profile -> 1, daily -> 2, reset -> 3. FetchReadings is
// only ever called with a kind Kinds() advertises, so the default case is
// defensive, not a reachable path.
func dataTypeParam(kind model.ReadingKind) (string, error) {
	switch kind {
	case model.ReadingKindLoadProfile:
		return "1", nil
	case model.ReadingKindDaily:
		return "2", nil
	case model.ReadingKindReset:
		return "3", nil
	default:
		return "", fmt.Errorf("osos: FetchReadings called with unsupported kind %q", kind)
	}
}

// windowChunks splits [from, to) into consecutive half-open windows of at
// most max (R20 pagination: a 45-day request with max=30d makes two chunks,
// 30 + 15 days). Deliberately not normalize.Chunk, whose local-midnight
// alignment produces one chunk per calendar day regardless of max — the
// wrong shape for OSOS's 30-day provider window.
func windowChunks(from, to time.Time, max time.Duration) []normalize.Window {
	if !from.Before(to) || max <= 0 {
		return nil
	}
	var out []normalize.Window
	cur := from
	for cur.Before(to) {
		end := cur.Add(max)
		if end.After(to) {
			end = to
		}
		out = append(out, normalize.Window{From: cur, To: end})
		cur = end
	}
	return out
}

// ososMonth is one Europe/Istanbul calendar month intersecting a window,
// for hourly_values' once-per-calendar-month fetch.
type ososMonth struct {
	Label string    // "YYYY-MM"
	Start time.Time // UTC instant of the month's Istanbul-local midnight
}

// monthsIntersecting returns every Europe/Istanbul calendar month that
// [from, to) touches, in order.
func monthsIntersecting(from, to time.Time) []ososMonth {
	if !from.Before(to) {
		return nil
	}
	local := from.In(normalize.Istanbul)
	y, m, _ := local.Date()
	cur := time.Date(y, m, 1, 0, 0, 0, 0, normalize.Istanbul)

	var out []ososMonth
	for cur.Before(to) {
		out = append(out, ososMonth{Label: cur.Format("2006-01"), Start: cur})
		cur = cur.AddDate(0, 1, 0)
	}
	return out
}

// fetchEnergyValues GETs energy_values for one window and returns its rows,
// unmapped.
func (s *Source) fetchEnergyValues(ctx context.Context, cl *httpx.Client, token string, creds integration.Credentials, installationNumber, dataType string, w normalize.Window) ([]wireEnergyRow, error) {
	tmpl, ok := creds.Endpoints["energy_values"]
	if !ok {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "energy_values"}
	}

	resp, err := cl.Do(ctx, httpx.Request{
		Op:       "energy_values",
		Method:   http.MethodGet,
		Template: tmpl,
		Params: map[string]httpx.Param{
			"secret_token":       {Value: token, Secret: true},
			"start_date":         {Value: normalize.FormatOSOSDate(w.From)},
			"end_date":           {Value: normalize.FormatOSOSDate(w.To)},
			"installationNumber": {Value: installationNumber},
			"dataType":           {Value: dataType},
		},
	})
	if err != nil {
		return nil, err
	}

	var body energyResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "energy_values", HTTPStatus: resp.Status}
	}
	return body.Energy, nil
}

// fetchHourlyValues GETs hourly_values for one calendar month and returns
// installationNumber's valueList entries, unmapped.
func (s *Source) fetchHourlyValues(ctx context.Context, cl *httpx.Client, token string, creds integration.Credentials, installationNumber, meterMonth string, from time.Time) ([]wireHourlyValue, error) {
	tmpl, ok := creds.Endpoints["hourly_values"]
	if !ok {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "hourly_values"}
	}

	resp, err := cl.Do(ctx, httpx.Request{
		Op:       "hourly_values",
		Method:   http.MethodGet,
		Template: tmpl,
		Params: map[string]httpx.Param{
			"secret_token":       {Value: token, Secret: true},
			"meter_month":        {Value: meterMonth},
			"from_date":          {Value: normalize.FormatOSOSDate(from)},
			"installationNumber": {Value: installationNumber},
		},
	})
	if err != nil {
		return nil, err
	}

	var body hourlyResponse
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "hourly_values", HTTPStatus: resp.Status}
	}

	var out []wireHourlyValue
	for _, block := range body.Items[installationNumber] {
		out = append(out, block.ValueList...)
	}
	return out, nil
}

// FetchReadings implements 06 §2 "Fetch energy values" and "Fetch hourly
// values": one auth call, then energy_values chunked to MaxWindow (R20),
// then — for load_profile only — hourly_values once per calendar month
// intersecting the window, kept in FetchResult.HourlyValues and never
// mixed into Readings (removed-behaviour 23).
func (s *Source) FetchReadings(ctx context.Context, creds integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	dataType, err := dataTypeParam(req.Kind)
	if err != nil {
		return integration.FetchResult{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderOSOS, Op: "energy_values"}
	}

	cl := s.client(creds)
	token, err := s.authenticate(ctx, cl, creds)
	if err != nil {
		return integration.FetchResult{}, err
	}

	allChunks := windowChunks(req.From, req.To, s.MaxWindow(req.Kind))
	chunks := allChunks
	var nextCursor *time.Time
	if len(chunks) > s.pageBudget {
		chunks = chunks[:s.pageBudget]
		nc := allChunks[s.pageBudget].From
		nextCursor = &nc
	}

	var (
		readings []model.MeterReading
		warnings []integration.Warning
	)
	for _, w := range chunks {
		rows, err := s.fetchEnergyValues(ctx, cl, token, creds, req.Point.InstallationNumber, dataType, w)
		if err != nil {
			return integration.FetchResult{}, err
		}
		for i, row := range rows {
			reading, field, ok := mapEnergyRow(row, req)
			if !ok {
				warnings = append(warnings, integration.Warning{
					Code:   integration.WarnUnparseableRow,
					Detail: fmt.Sprintf("%s at row %d", field, i),
				})
				continue
			}
			if reading.Ts.Before(req.From) || !reading.Ts.Before(req.To) {
				continue // outside [From, To): dropped, not warned
			}
			readings = append(readings, reading)
		}
	}
	readings = sortAndDedupeReadings(readings)

	var hourly []integration.HourlyValue
	// Hourly is fetched only once the window is fully covered this call
	// (nextCursor == nil): a PageBudget-truncated call will pick it up on
	// the resumed call once the energy_values window is exhausted.
	if req.Kind == model.ReadingKindLoadProfile && nextCursor == nil {
		for _, m := range monthsIntersecting(req.From, req.To) {
			from := req.From
			if m.Start.After(from) {
				from = m.Start
			}
			items, err := s.fetchHourlyValues(ctx, cl, token, creds, req.Point.InstallationNumber, m.Label, from)
			if err != nil {
				return integration.FetchResult{}, err
			}
			for i, v := range items {
				hv, field, ok := mapHourlyValue(v)
				if !ok {
					warnings = append(warnings, integration.Warning{
						Code:   integration.WarnUnparseableRow,
						Detail: fmt.Sprintf("hourly %s at row %d", field, i),
					})
					continue
				}
				if hv.Ts.Before(req.From) || !hv.Ts.Before(req.To) {
					continue
				}
				hourly = append(hourly, hv)
			}
		}
		hourly = sortAndDedupeHourly(hourly)
	}

	return integration.FetchResult{
		Readings:     readings,
		NextCursor:   nextCursor,
		Warnings:     warnings,
		HourlyValues: hourly,
	}, nil
}

// sortAndDedupeReadings sorts by Ts and drops duplicate timestamps (R20:
// merged window chunks never produce duplicates), keeping the
// first-encountered row for a given Ts.
func sortAndDedupeReadings(in []model.MeterReading) []model.MeterReading {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Ts.Before(in[j].Ts) })
	out := make([]model.MeterReading, 0, len(in))
	for i, r := range in {
		if i > 0 && r.Ts.Equal(in[i-1].Ts) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// sortAndDedupeHourly is sortAndDedupeReadings' HourlyValue counterpart.
func sortAndDedupeHourly(in []integration.HourlyValue) []integration.HourlyValue {
	sort.SliceStable(in, func(i, j int) bool { return in[i].Ts.Before(in[j].Ts) })
	out := make([]integration.HourlyValue, 0, len(in))
	for i, v := range in {
		if i > 0 && v.Ts.Equal(in[i-1].Ts) {
			continue
		}
		out = append(out, v)
	}
	return out
}
