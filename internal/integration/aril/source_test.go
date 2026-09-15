package aril_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/aril"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// --- test harness -----------------------------------------------------

// recordingSleep is a fake httpx.PoolOptions.Sleep: it records every
// requested delay and returns immediately, so no test in this file ever
// waits in real time.
type recordingSleep struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (r *recordingSleep) fn(_ context.Context, d time.Duration) error {
	r.mu.Lock()
	r.delays = append(r.delays, d)
	r.mu.Unlock()
	return nil
}

func (r *recordingSleep) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.delays))
	copy(out, r.delays)
	return out
}

func arilTestPool(t *testing.T, srv *fake.Server) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: rs.fn})
	require.NoError(t, err)
	return pool, rs
}

// fixtureNow is the fixed instant every test's clock reports, so
// MeterReading.IngestedAt is deterministic.
var fixtureNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

const arilOwnerSerno = "1000123456"

var arilAnalyzerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func arilInt16Ptr(v int16) *int16    { return &v }
func arilStringPtr(s string) *string { return &s }

func arilEndpoints(srv *fake.Server) map[string]string {
	base := srv.URL + "/aril"
	return map[string]string{
		"authentication":       base + "/authentication",
		"analyzers_list":       base + "/analyzers-list",
		"owner_consumptions":   base + "/owner-consumptions",
		"current_endexes":      base + "/current-endexes",
		"end_of_month_endexes": base + "/end-of-month-endexes",
	}
}

func arilTestCreds(srv *fake.Server) integration.Credentials {
	return integration.Credentials{
		CredentialID: uuid.MustParse("00000000-0000-0000-0000-0000000000c1"),
		CompanyID:    uuid.MustParse("00000000-0000-0000-0000-0000000000c0"),
		Provider:     integration.ProviderARIL,
		Endpoints:    arilEndpoints(srv),
		Username:     "FIXTURE-user",
		Secret:       integration.NewSecret([]byte("FIXTURE-pass")),
	}
}

func arilTestPoint() integration.MeteringPoint {
	return integration.MeteringPoint{
		InstallationNumber: arilOwnerSerno,
		DefinitionType:     arilInt16Ptr(15),
		MeterNumber:        arilStringPtr("SN-ARIL-1"),
	}
}

func arilTestRequest(kind model.ReadingKind, from, to time.Time, multiplier decimal.Decimal) integration.FetchRequest {
	return integration.FetchRequest{
		Point:      arilTestPoint(),
		AnalyzerID: arilAnalyzerID,
		Multiplier: multiplier,
		Kind:       kind,
		From:       from,
		To:         to,
	}
}

func arilNewSource(pool *httpx.Pool, pageBudget int) *aril.Source {
	return aril.New(pool, aril.Options{Clock: clock.NewFake(fixtureNow), PageBudget: pageBudget})
}

// arilAuthRoute is the authentication call every Verify/Discover/Fetch
// makes first.
func arilAuthRoute(t *testing.T) fake.Route {
	t.Helper()
	return fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_token_bare_string.json"))}
}

// arilAuthFailureResponder answers 401 and echoes the submitted body back
// in the response — the only channel through which the plaintext password
// could leak, since httpx never returns a failed response's body to the
// caller. TestARILErrorsCarryNoCredential asserts it does not.
func arilAuthFailureResponder(t *testing.T) fake.Responder {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"echo": string(raw)})
	}
}

func istanbulDay(year int, month time.Month, day int) (from, to time.Time) {
	from = time.Date(year, month, day, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to = time.Date(year, month, day+1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	return from, to
}

func sortReadings(rs []model.MeterReading) {
	sort.Slice(rs, func(i, j int) bool {
		if !rs[i].Ts.Equal(rs[j].Ts) {
			return rs[i].Ts.Before(rs[j].Ts)
		}
		return rs[i].Kind < rs[j].Kind
	})
}

// --- Step 1: fixture matrix --------------------------------------------

// TestARILFixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination". The subtest names are exactly fake.RequiredCases; Task 17's
// guard checks them.
//
// Deviation from the shared template's literal pseudocode: the
// "pagination" subtest calls DiscoverMeteringPoints, not FetchReadings.
// task-8-brief.md item 2 names analyzers_list's own PageNumber paging as
// "the pagination case" for ARIL, and FetchReadings never internally
// paginates (source.go's package doc: 06 §4 documents no per-call window
// limit for ARIL's data endpoints, so one call always fully covers
// [From, To)).
func TestARILFixtureMatrix(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	for _, tc := range []struct {
		name        string
		routes      func(t *testing.T) []fake.Route
		wantErr     error
		useDiscover bool
		check       func(t *testing.T, res integration.FetchResult, points []integration.MeteringPoint, rs *recordingSleep, srv *fake.Server)
	}{
		{
			name: "success",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_success.json"))},
				}
			},
			check: func(t *testing.T, res integration.FetchResult, _ []integration.MeteringPoint, _ *recordingSleep, _ *fake.Server) {
				require.Len(t, res.Readings, 2)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "empty",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_empty.json"))},
				}
			},
			check: func(t *testing.T, res integration.FetchResult, _ []integration.MeteringPoint, _ *recordingSleep, _ *fake.Server) {
				require.Empty(t, res.Readings)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "partial",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_partial.json"))},
				}
			},
			check: func(t *testing.T, res integration.FetchResult, _ []integration.MeteringPoint, _ *recordingSleep, _ *fake.Server) {
				require.Len(t, res.Readings, 1)
				var found bool
				for _, w := range res.Warnings {
					if w.Code == integration.WarnUnparseableRow {
						found = true
					}
				}
				require.True(t, found, "expected a WarnUnparseableRow, got %+v", res.Warnings)
			},
		},
		{
			name: "auth_failure",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{Method: http.MethodPost, Path: "/aril/authentication", Respond: arilAuthFailureResponder(t)}}
			},
			wantErr: integration.ErrAuth,
		},
		{
			name: "malformed",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_malformed.json"))},
				}
			},
			wantErr: integration.ErrMalformedPayload,
		},
		{
			name: "rate_limited",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.Sequence(
						fake.RateLimited("1"),
						fake.JSON(200, fake.Fixture(t, "aril", "aril_rate_limited.json")),
					)},
				}
			},
			check: func(t *testing.T, res integration.FetchResult, _ []integration.MeteringPoint, rs *recordingSleep, _ *fake.Server) {
				require.Len(t, res.Readings, 1)
				require.Contains(t, rs.recorded(), time.Second, "expected the Retry-After: 1 delay to be recorded")
			},
		},
		{
			name:        "pagination",
			useDiscover: true,
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					arilAuthRoute(t),
					{Method: http.MethodPost, Path: "/aril/analyzers-list", Respond: fake.Sequence(
						fake.JSON(200, fake.Fixture(t, "aril", "aril_subscriptions_page1.json")),
						fake.JSON(200, fake.Fixture(t, "aril", "aril_subscriptions_page2.json")),
					)},
				}
			},
			check: func(t *testing.T, _ integration.FetchResult, points []integration.MeteringPoint, _ *recordingSleep, srv *fake.Server) {
				require.Len(t, points, 1002, "1000 rows from page 1 + 2 rows from page 2, merged")

				var calls []fake.RecordedRequest
				for _, r := range srv.Requests() {
					if r.Path == "/aril/analyzers-list" {
						calls = append(calls, r)
					}
				}
				require.Len(t, calls, 2, "expected exactly one call per page")
				var body1, body2 struct {
					PageNumber int `json:"PageNumber"`
				}
				require.NoError(t, json.Unmarshal(calls[0].Body, &body1))
				require.NoError(t, json.Unmarshal(calls[1].Body, &body2))
				require.Equal(t, 1, body1.PageNumber)
				require.Equal(t, 2, body2.PageNumber)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			pool, rs := arilTestPool(t, srv)
			src := arilNewSource(pool, 0)
			creds := arilTestCreds(srv)

			if tc.useDiscover {
				points, err := src.DiscoverMeteringPoints(context.Background(), creds)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					return
				}
				require.NoError(t, err)
				tc.check(t, integration.FetchResult{}, points, rs, srv)
				return
			}

			req := arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1))
			res, err := src.FetchReadings(context.Background(), creds, req)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, res, nil, rs, srv)
		})
	}
}

// --- Named acceptance tests (task-8-brief.md) ---------------------------

func TestARILAcceptsBothTokenShapes(t *testing.T) {
	t.Run("bare_string", func(t *testing.T) {
		srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_token_bare_string.json"))})
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		require.NoError(t, src.Verify(context.Background(), arilTestCreds(srv)))
	})

	t.Run("object", func(t *testing.T) {
		srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_token_object.json"))})
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		require.NoError(t, src.Verify(context.Background(), arilTestCreds(srv)))
	})
}

// TestARILEmptyTokenIsAuthFailure exercises aril_auth_failure.json (a bare
// empty string): task-8-brief.md item 1, "A 401 or an empty token is
// ErrAuth" — the empty-token half, distinct from the 401-status half
// TestARILFixtureMatrix's "auth_failure" case covers.
func TestARILEmptyTokenIsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_auth_failure.json"))})
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	err := src.Verify(context.Background(), arilTestCreds(srv))
	require.ErrorIs(t, err, integration.ErrAuth)
}

func TestARILRequestsWithoutMultiplierAndMultipliesOnce(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_success.json"))},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(40)))
	require.NoError(t, err)
	require.NotEmpty(t, res.Readings)

	var found bool
	for _, r := range srv.Requests() {
		if r.Path != "/aril/owner-consumptions" {
			continue
		}
		require.Contains(t, string(r.Body), `"WithoutMultiplier":true`)
		found = true
	}
	require.True(t, found)

	sortReadings(res.Readings)
	requireDecimalEqual(t, "500", res.Readings[0].ActiveImport) // TSum "12.5" (aril_success.json row 0) x 40
}

func TestARILMaxDemandIsMonthlyMaximum(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	// end_of_month_endexes' snapshot itself falls at the end of the day
	// window aril_end_of_month.json uses, current_endexes' rows span the
	// same September day with three different MaxDemand values.
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/end-of-month-endexes", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_end_of_month.json"))},
		fake.Route{Method: http.MethodPost, Path: "/aril/current-endexes", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_current_endexes.json"))},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindBilling, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	requireDecimalEqual(t, "75", res.Readings[0].MaxDemandKw) // max(50,75,60) from aril_current_endexes.json
}

func TestARILProfileDateIsIstanbulLocal(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"LoadProfiles":[{"ProfileDate":20260901100000,"TSum":"1"}]}`)
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.True(t, res.Readings[0].Ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)), "got %s", res.Readings[0].Ts)
}

func TestARILErrorCodeIsNotSwallowed(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"ErrorCode":5,"LoadProfiles":[]}`)
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable, "removed-behaviour 27: the legacy swallowed a nonzero ErrorCode as []")
	require.Empty(t, res.Readings)
}

// --- shared adapter template's "also required in every adapter" tests ---

func TestIdempotentARILRefetchYieldsIdenticalReadings(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, fake.Fixture(t, "aril", "aril_success.json"))},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)
	req := arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1))

	res1, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)
	res2, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)

	sortReadings(res1.Readings)
	sortReadings(res2.Readings)
	require.Equal(t, res1.Readings, res2.Readings)
}

func TestARILVerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: arilAuthFailureResponder(t)})
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	err := src.Verify(context.Background(), arilTestCreds(srv))
	require.ErrorIs(t, err, integration.ErrAuth)
}

func TestARILNeverReturnsReadingsOutsideWindow(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"LoadProfiles":[
		{"ProfileDate":20260831100000,"TSum":"1"},
		{"ProfileDate":20260901100000,"TSum":"2"},
		{"ProfileDate":20260902100000,"TSum":"3"}
	]}`)
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.True(t, res.Readings[0].Ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)))
}

// TestARILNeverReturnsReadingsOutsideWindowPreciseBoundary is adapter
// review pattern 4: the half-open window [From, To) tested at its exact
// edges — From-1s dropped, From itself kept, To itself dropped.
func TestARILNeverReturnsReadingsOutsideWindowPreciseBoundary(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	beforeFrom := from.Add(-time.Second)

	body := []byte(fmt.Sprintf(`{"LoadProfiles":[
		{"ProfileDate":%s,"TSum":"1"},
		{"ProfileDate":%s,"TSum":"2"},
		{"ProfileDate":%s,"TSum":"3"}
	]}`, arilProfileDateLiteral(beforeFrom), arilProfileDateLiteral(from), arilProfileDateLiteral(to)))

	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "only the row at exactly From must survive")
	require.True(t, res.Readings[0].Ts.Equal(from))
}

// arilProfileDateLiteral renders t as ARIL's 14-digit yyyyMMddHHmmss
// Istanbul-local literal, unquoted, for building an inline JSON body.
func arilProfileDateLiteral(t time.Time) string {
	return t.In(normalize.Istanbul).Format("20060102150405")
}

func TestARILErrorsCarryNoCredential(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: arilAuthFailureResponder(t)})
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	_, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "FIXTURE-pass")
	require.NotContains(t, fmt.Sprintf("%+v", err), "FIXTURE-pass")
}

// --- adapter review patterns (Task 6 OSOS opus review, applied here) ----

// TestARILStampsEveryTemplateRequiredField is adapter review pattern 1:
// every field the template requires an adapter to stamp — Ts, Kind,
// AnalyzerID, SourceProvider, MultiplierApplied, meter serial, and
// (pattern 14) the row's own original raw bytes — asserted together on one
// reading.
func TestARILStampsEveryTemplateRequiredField(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	rowBody := []byte(`{"ProfileDate":20260901100000,"TSum":"100"}`)
	body := []byte(`{"LoadProfiles":[` + string(rowBody) + `]}`)

	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(3)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	r := res.Readings[0]

	require.True(t, r.Ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)), "Ts")
	require.Equal(t, model.ReadingKindLoadProfile, r.Kind, "Kind")
	require.Equal(t, arilAnalyzerID, r.AnalyzerID, "AnalyzerID")
	require.Equal(t, model.IntegrationProviderARIL, r.SourceProvider, "SourceProvider")
	require.True(t, decimal.NewFromInt(3).Equal(r.MultiplierApplied), "MultiplierApplied")
	require.NotNil(t, r.MeterSerial, "meter serial")
	require.Equal(t, "SN-ARIL-1", *r.MeterSerial)
	require.JSONEq(t, string(rowBody), string(r.Raw), "Raw must be the row's own original bytes (pattern 14), not a re-marshalled struct")
}

// TestARILMalformedEnvelopeShapes is adapter review pattern 6: a 200
// response whose top-level key is missing or explicitly null is
// ErrMalformedPayload — never an empty success, and never confused with a
// legitimate ErrorCode!=0 business failure (TestARILErrorCodeIsNotSwallowed).
func TestARILMalformedEnvelopeShapes(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty_object", `{}`},
		{"bare_null", `null`},
		{"explicit_null_load_profiles", `{"LoadProfiles":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t,
				arilAuthRoute(t),
				fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, []byte(tc.body))},
			)
			pool, _ := arilTestPool(t, srv)
			src := arilNewSource(pool, 0)
			creds := arilTestCreds(srv)

			res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
			require.ErrorIs(t, err, integration.ErrMalformedPayload)
			require.NotErrorIs(t, err, integration.ErrUpstreamUnavailable)
			require.Empty(t, res.Readings)
		})
	}
}

// TestARILPerRowDecodeFailureIsolated is adapter review pattern 7: one
// structurally-malformed row (TSum given as a JSON object, not a string)
// produces a WarnUnparseableRow for that row alone, never
// ErrMalformedPayload for the whole page.
func TestARILPerRowDecodeFailureIsolated(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"LoadProfiles":[
		{"ProfileDate":20260901100000,"TSum":"100"},
		{"ProfileDate":20260901101500,"TSum":{}},
		{"ProfileDate":20260901103000,"TSum":"300"}
	]}`)

	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err, "one bad row must not fail the whole page")
	require.Len(t, res.Readings, 2, "the two structurally-valid rows must still be returned")

	var found bool
	for _, w := range res.Warnings {
		if w.Code == integration.WarnUnparseableRow {
			found = true
		}
	}
	require.True(t, found, "the malformed row must still produce a warning, not be silently dropped")
}

// TestARILDuplicateTimestampConflictWarns is adapter review pattern 13:
// two rows sharing one Ts whose mapped registers DIFFER are not silently
// merged or arbitrarily dropped — the first is kept and a warning notes
// the conflict.
func TestARILDuplicateTimestampConflictWarns(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"LoadProfiles":[
		{"ProfileDate":20260901100000,"TSum":"100"},
		{"ProfileDate":20260901100000,"TSum":"999"}
	]}`)

	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/owner-consumptions", Respond: fake.JSON(200, body)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)
	creds := arilTestCreds(srv)

	res, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "the conflicting duplicate must not produce a second reading")
	requireDecimalEqual(t, "100", res.Readings[0].ActiveImport)

	var found bool
	for _, w := range res.Warnings {
		if w.Code == integration.WarnUnparseableRow && strings.Contains(w.Detail, "duplicate timestamp") {
			found = true
		}
	}
	require.True(t, found, "expected a duplicate-timestamp warning, got %+v", res.Warnings)
}

// TestARILConfigErrorsAreDeliberate is adapter review patterns 9 and 12: a
// missing endpoint template, a blank OwnerSerno or a zero multiplier is
// reported as a deliberately-classified *integration.Error, before ANY
// HTTP call is attempted.
func TestARILConfigErrorsAreDeliberate(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	t.Run("missing_endpoint_key", func(t *testing.T) {
		srv := fake.NewTLSServer(t) // no routes: any request fails the test
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		creds := arilTestCreds(srv)
		delete(creds.Endpoints, "owner_consumptions")

		_, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1)))
		require.ErrorIs(t, err, integration.ErrAuth)
		var ierr *integration.Error
		require.ErrorAs(t, err, &ierr)
		require.Equal(t, "config:owner_consumptions", ierr.Op)
		require.Empty(t, srv.Requests(), "no HTTP call must be attempted for an incomplete configuration")
	})

	t.Run("blank_owner_serno", func(t *testing.T) {
		srv := fake.NewTLSServer(t)
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		creds := arilTestCreds(srv)
		req := arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1))
		req.Point.InstallationNumber = ""

		_, err := src.FetchReadings(context.Background(), creds, req)
		require.ErrorIs(t, err, integration.ErrAuth)
		require.Empty(t, srv.Requests())
	})

	t.Run("missing_definition_type", func(t *testing.T) {
		srv := fake.NewTLSServer(t)
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		creds := arilTestCreds(srv)
		req := arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.NewFromInt(1))
		req.Point.DefinitionType = nil

		_, err := src.FetchReadings(context.Background(), creds, req)
		require.ErrorIs(t, err, integration.ErrAuth)
		require.Empty(t, srv.Requests())
	})

	t.Run("zero_multiplier", func(t *testing.T) {
		srv := fake.NewTLSServer(t)
		pool, _ := arilTestPool(t, srv)
		src := arilNewSource(pool, 0)
		creds := arilTestCreds(srv)

		_, err := src.FetchReadings(context.Background(), creds, arilTestRequest(model.ReadingKindLoadProfile, from, to, decimal.Zero))
		require.ErrorIs(t, err, integration.ErrAuth)
		require.Empty(t, srv.Requests())
	})
}

// TestARILAuthenticationNeverRetries is adapter review pattern 10 (Ruling
// R32): the login POST is marked NoRetry, so a retryable-classified
// failure (here, 500 -> ErrUpstreamUnavailable, which IS retryable in
// general) still produces exactly one HTTP attempt.
func TestARILAuthenticationNeverRetries(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/aril/authentication", Respond: func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}})
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 0)

	err := src.Verify(context.Background(), arilTestCreds(srv))
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls, "R32: a non-idempotent login POST must never be retried in-client")
}

// --- pagination / page budget (task-8-brief.md item 2, adapter review
// pattern 5/8) -------------------------------------------------------------

// TestARILDiscoverPageBudgetLimitsPages proves Options.PageBudget stops
// analyzers_list paging after that many pages, without erroring, and that
// a smaller-than-default budget genuinely truncates (request-count
// checked, not just a result-count check, per adapter review pattern 5).
func TestARILDiscoverPageBudgetLimitsPages(t *testing.T) {
	srv := fake.NewTLSServer(t,
		arilAuthRoute(t),
		fake.Route{Method: http.MethodPost, Path: "/aril/analyzers-list", Respond: fake.Sequence(
			fake.JSON(200, fake.Fixture(t, "aril", "aril_subscriptions_page1.json")),
			fake.JSON(200, fake.Fixture(t, "aril", "aril_subscriptions_page2.json")),
		)},
	)
	pool, _ := arilTestPool(t, srv)
	src := arilNewSource(pool, 1) // budget of exactly one page
	creds := arilTestCreds(srv)

	points, err := src.DiscoverMeteringPoints(context.Background(), creds)
	require.NoError(t, err)
	require.Len(t, points, 1000, "page 1's own 1000 rows, page 2 never fetched")

	var calls int
	for _, r := range srv.Requests() {
		if r.Path == "/aril/analyzers-list" {
			calls++
		}
	}
	require.Equal(t, 1, calls, "PageBudget=1 must stop after exactly one page")
}

// --- helpers ---------------------------------------------------------------

func requireDecimalEqual(t *testing.T, want string, d *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, d)
	require.True(t, decimal.RequireFromString(want).Equal(*d), "got %s, want %s", d.String(), want)
}
