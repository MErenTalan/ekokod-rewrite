package gridbox_test

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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/gridbox"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

// --- test harness ---------------------------------------------------------

// recordingSleep is a fake httpx.PoolOptions.Sleep: it records every
// requested delay and returns immediately, so no test in this file ever
// waits in real time (mirrors httpx/client_test.go's own helper — that one
// is unexported to its package, so gridbox_test needs its own copy).
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

// gridboxTestPool builds a Pool pinned to srv's own certificate, with a
// no-op recording Sleep.
func gridboxTestPool(t *testing.T, srv *fake.Server) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: rs.fn})
	require.NoError(t, err)
	return pool, rs
}

// fakeLocker is a Locker that records Acquire/Release pairs in order, for
// TestGridBoxDataCallsSerializePerCompany (I1: fix round 1 finding). Mirrors
// httpx/client_test.go's own copy (unexported to that package).
type fakeLocker struct {
	mu     sync.Mutex
	events []string
}

type fakeLease struct {
	l   *fakeLocker
	key string
}

func (l *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	l.mu.Lock()
	l.events = append(l.events, "acquire:"+key)
	l.mu.Unlock()
	return &fakeLease{l: l, key: key}, nil
}

func (l *fakeLease) Release(_ context.Context) error {
	l.l.mu.Lock()
	l.l.events = append(l.l.events, "release:"+l.key)
	l.l.mu.Unlock()
	return nil
}

func (l *fakeLocker) recorded() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.events))
	copy(out, l.events)
	return out
}

// gridboxTestPoolWithLocker is gridboxTestPool plus a recording Locker — the
// only harness variant that needs one (SerializeKey observation).
func gridboxTestPoolWithLocker(t *testing.T, srv *fake.Server, locker httpx.Locker) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: rs.fn, Locker: locker})
	require.NoError(t, err)
	return pool, rs
}

// fixtureNow is the fixed instant every test's clock reports, so
// MeterReading.IngestedAt is deterministic — required for
// TestGridBoxIdempotentRefetchYieldsIdenticalReadings.
var fixtureNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

const gridboxWiring = "FX_WIRING_1"

var gridboxAnalyzerID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// gridboxEndpoints builds the Endpoints template map source.go's endpoint
// keys expect, with every placeholder as a query parameter (so every
// endpoint has a fixed, easy-to-route Path — fake.Server matches Method and
// Path only, never the query string, unless a Route supplies its own
// Match).
func gridboxEndpoints(srv *fake.Server) map[string]string {
	base := srv.URL + "/gridbox"
	return map[string]string{
		"token":             base + "/token",
		"last_success_date": base + "/last-success-date?wiringNo={wiringNo}",
		"last_endex":        base + "/last-endex?wiringNo={wiringNo}",
		"load_profiles":     base + "/load-profiles?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}",
		"endexes":           base + "/endexes?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}&isBilling={isBilling}",
		"energy_values":     base + "/energy-values?wiringNo={wiringNo}&startDate={startDate}&endDate={endDate}",
	}
}

func gridboxTestCreds(srv *fake.Server, billing bool) integration.Credentials {
	return integration.Credentials{
		CredentialID: uuid.MustParse("00000000-0000-0000-0000-0000000000c1"),
		CompanyID:    uuid.MustParse("00000000-0000-0000-0000-0000000000c0"),
		Provider:     integration.ProviderGridBox,
		Endpoints:    gridboxEndpoints(srv),
		Username:     "FIXTURE-user",
		Secret:       integration.NewSecret([]byte("FIXTURE-pass")),
		Settings:     integration.Settings{UseBillingIndexes: billing},
	}
}

func gridboxTestRequest(kind model.ReadingKind, from, to time.Time) integration.FetchRequest {
	return integration.FetchRequest{
		Point:      integration.MeteringPoint{InstallationNumber: gridboxWiring},
		AnalyzerID: gridboxAnalyzerID,
		Kind:       kind,
		From:       from,
		To:         to,
	}
}

func gridboxNewSource(pool *httpx.Pool, pageBudget int) *gridbox.Source {
	return gridbox.New(pool, gridbox.Options{Clock: clock.NewFake(fixtureNow), PageBudget: pageBudget})
}

// gridboxBaseRoutes are the two calls every FetchReadings/Verify/Discover
// makes regardless of kind: the token exchange and last_success_date.
func gridboxBaseRoutes(t *testing.T) []fake.Route {
	t.Helper()
	return []fake.Route{
		{Method: http.MethodPost, Path: "/gridbox/token", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_token.json"))},
		{Method: http.MethodGet, Path: "/gridbox/last-success-date", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_success_date.json"))},
	}
}

// gridboxAuthFailureResponder answers 400 invalid_grant and echoes the
// submitted form body back in the response — the only way
// TestGridBoxErrorsCarryNoCredential can prove the credential does not
// survive into err.Error(): httpx never returns a failed response's body to
// the caller, so this route's ECHO is the sole channel through which the
// plaintext password could leak, and the test asserts it does not.
func gridboxAuthFailureResponder(t *testing.T) fake.Responder {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "FIXTURE-invalid-credentials",
			"echo":              string(raw),
		})
	}
}

// istanbulDay returns the [from, to) half-open UTC instants bounding one
// Europe/Istanbul local calendar day — well under GridBox's 30-day
// MaxWindow, so tests that do not care about pagination never accidentally
// trigger splitWindow into more than one chunk.
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

// --- Step 1: fixture matrix -----------------------------------------------

// TestGridBoxFixtureMatrix is the F2 acceptance criterion "every adapter
// has recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination". The subtest names are exactly fake.RequiredCases; Task 17's
// guard checks them.
func TestGridBoxFixtureMatrix(t *testing.T) {
	dayFrom, dayTo := istanbulDay(2026, 9, 1)
	// pagFrom/pagTo span 45 days (Jan1 - Feb15 Istanbul), which splitWindow
	// (R36 — never normalize.Chunk for this) breaks into exactly two
	// chunks under GridBox's 30-day MaxWindow: [Jan1,Jan31) and
	// [Jan31,Feb15). Each chunk gets its own recorded HTTP call and its own
	// fixture body, so the "pagination" case actually proves two provider
	// pages were merged, in order, without duplicates — not just that the
	// window happened to span more than one calendar day.
	pagFrom, _ := istanbulDay(2026, 1, 1)
	_, pagTo := istanbulDay(2026, 2, 14)
	pagChunk2Body := []byte(`{"ResultStatus":1,"ResultObject":[{"ProfileDateTime":"2026-02-01T10:00:00+03:00","ActiveEndex":200}]}`)

	for _, tc := range []struct {
		name    string
		from    time.Time
		to      time.Time
		routes  func(t *testing.T) []fake.Route
		wantErr error
		check   func(t *testing.T, res integration.FetchResult, rs *recordingSleep, srv *fake.Server)
	}{
		{
			name: "success",
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
				)
			},
			check: func(t *testing.T, res integration.FetchResult, _ *recordingSleep, _ *fake.Server) {
				require.Len(t, res.Readings, 2)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "empty",
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_empty.json"))},
				)
			},
			check: func(t *testing.T, res integration.FetchResult, _ *recordingSleep, _ *fake.Server) {
				require.Empty(t, res.Readings)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "partial",
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_partial.json"))},
				)
			},
			check: func(t *testing.T, res integration.FetchResult, _ *recordingSleep, _ *fake.Server) {
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
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{Method: http.MethodPost, Path: "/gridbox/token", Respond: gridboxAuthFailureResponder(t)}}
			},
			wantErr: integration.ErrAuth,
		},
		{
			name: "malformed",
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_malformed.json"))},
				)
			},
			wantErr: integration.ErrMalformedPayload,
		},
		{
			name: "rate_limited",
			from: dayFrom, to: dayTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.Sequence(
						fake.RateLimited("1"),
						fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_rate_limited.json")),
					)},
				)
			},
			check: func(t *testing.T, res integration.FetchResult, rs *recordingSleep, _ *fake.Server) {
				require.Len(t, res.Readings, 1)
				require.Contains(t, rs.recorded(), time.Second, "expected the Retry-After: 1 delay to be recorded")
			},
		},
		{
			name: "pagination",
			from: pagFrom, to: pagTo,
			routes: func(t *testing.T) []fake.Route {
				return append(gridboxBaseRoutes(t),
					fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
					fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.Sequence(
						fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_pagination.json")),
						fake.JSON(200, pagChunk2Body),
					)},
				)
			},
			check: func(t *testing.T, res integration.FetchResult, _ *recordingSleep, srv *fake.Server) {
				require.Nil(t, res.NextCursor, "default PageBudget covers both 30-day-max chunks in one call")
				require.Len(t, res.Readings, 2)
				sortReadings(res.Readings)
				require.True(t, res.Readings[0].Ts.Before(res.Readings[1].Ts), "merged in order")
				require.False(t, res.Readings[0].Ts.Equal(res.Readings[1].Ts), "no duplicates")

				// Adapter review pattern 5: COUNT recorded requests and
				// assert their date params — fake.Sequence repeats its last
				// page on any call beyond what it was given, which would
				// silently hide a "chunk 2 never got its own call, chunk 1
				// was just served twice" bug that a readings-count-only
				// assertion cannot catch (2 reads either way).
				var loadProfileCalls []fake.RecordedRequest
				for _, r := range srv.Requests() {
					if r.Path == "/gridbox/load-profiles" {
						loadProfileCalls = append(loadProfileCalls, r)
					}
				}
				require.Len(t, loadProfileCalls, 2, "expected exactly one call per chunk")
				require.Contains(t, loadProfileCalls[0].RawQuery, "startDate=2026-01-01")
				require.Contains(t, loadProfileCalls[0].RawQuery, "endDate=2026-01-30")
				require.Contains(t, loadProfileCalls[1].RawQuery, "startDate=2026-01-31")
				require.Contains(t, loadProfileCalls[1].RawQuery, "endDate=2026-02-14")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			pool, rs := gridboxTestPool(t, srv)
			src := gridboxNewSource(pool, 0)
			creds := gridboxTestCreds(srv, false)
			req := gridboxTestRequest(model.ReadingKindLoadProfile, tc.from, tc.to)

			res, err := src.FetchReadings(context.Background(), creds, req)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, res, rs, srv)
		})
	}
}

// --- Named acceptance tests (Task 7 brief) ---------------------------------

// TestGridBoxMultiplierResolution is the F2 acceptance criterion "GridBox
// multiplier resolution is tested for all three paths, including the
// fallback-to-1 case which must emit a warning". Unlike
// mapping_test.go's TestGridBoxResolveMultiplierPureCases (ResolveMultiplier
// called directly), this drives every path through FetchReadings so
// "readings stamped 80" and "res.Warnings" — both of which the brief
// mentions explicitly — are proven end to end.
func TestGridBoxMultiplierResolution(t *testing.T) {
	t.Run("last_endex", func(t *testing.T) {
		from, to := istanbulDay(2026, 9, 14)
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex_multiplier.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindCurrentIndex, from, to))
		require.NoError(t, err)
		require.NotNil(t, res.ResolvedMultiplier)
		require.Equal(t, integration.MultiplierFromLastEndex, res.ResolvedMultiplier.Source)
		require.True(t, decimal.NewFromInt(80).Equal(res.ResolvedMultiplier.Value))
		for _, w := range res.Warnings {
			require.NotEqual(t, integration.WarnMultiplierFallback, w.Code, "no fallback warning expected")
		}
		require.Len(t, res.Readings, 1)
		require.True(t, decimal.NewFromInt(80).Equal(res.Readings[0].MultiplierApplied), "readings stamped 80")
	})

	t.Run("derived_from_load_profile", func(t *testing.T) {
		from, to := istanbulDay(2026, 9, 14)
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_no_multiplier.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_load_profile_ratio.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
		require.NoError(t, err)
		require.Equal(t, integration.MultiplierFromLoadProfile, res.ResolvedMultiplier.Source)
		require.True(t, decimal.NewFromInt(40).Equal(res.ResolvedMultiplier.Value), "500/12.5 = 40, got %s", res.ResolvedMultiplier.Value)
	})

	t.Run("zero_denominator_skips_to_next_row", func(t *testing.T) {
		// gridbox_load_profile_ratio.json's FIRST row has ActiveEndex 0
		// (ignored); only the second row's 500/12.5 is usable. A resolved
		// value other than 40 here means row 1 was not skipped.
		from, to := istanbulDay(2026, 9, 14)
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_no_multiplier.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_load_profile_ratio.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
		require.NoError(t, err)
		require.Equal(t, integration.MultiplierFromLoadProfile, res.ResolvedMultiplier.Source)
		require.True(t, decimal.NewFromInt(40).Equal(res.ResolvedMultiplier.Value), "the zero-denominator first row must be skipped, not used")
	})

	t.Run("fallback_to_one_warns", func(t *testing.T) {
		from, to := istanbulDay(2026, 9, 1)
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_no_multiplier.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/energy-values", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindReset, from, to))
		require.NoError(t, err)
		require.Equal(t, integration.MultiplierFallbackOne, res.ResolvedMultiplier.Source)
		require.True(t, decimal.NewFromInt(1).Equal(res.ResolvedMultiplier.Value))

		var fallback []integration.Warning
		for _, w := range res.Warnings {
			if w.Code == integration.WarnMultiplierFallback {
				fallback = append(fallback, w)
			}
		}
		require.Len(t, fallback, 1, "exactly one WarnMultiplierFallback, got %+v", res.Warnings)
		require.Contains(t, fallback[0].Detail, gridboxWiring, "warning must name the wiring number")
	})

	// stored_multiplier_used_but_not_provider_resolved is R51/I2's core
	// regression: a "daily"-kind fetch (no last_endex.Multiplier, no
	// load_profiles at all — daily's own endpoint carries neither) with
	// req.Multiplier=40 (the analyzer's stored value, as the pipeline
	// always sets it — R3) must resolve to 40, not fall all the way back
	// to 1, and must report ProviderResolved=false so the pipeline never
	// treats this as new information to persist over the stored value.
	t.Run("stored_multiplier_used_but_not_provider_resolved", func(t *testing.T) {
		from, to := istanbulDay(2026, 9, 1)
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_no_multiplier.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/endexes", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		req := gridboxTestRequest(model.ReadingKindDaily, from, to)
		req.Multiplier = decimal.NewFromInt(40)

		res, err := src.FetchReadings(context.Background(), creds, req)
		require.NoError(t, err)
		require.NotNil(t, res.ResolvedMultiplier)
		require.Equal(t, integration.MultiplierFromRequest, res.ResolvedMultiplier.Source)
		require.False(t, res.ResolvedMultiplier.ProviderResolved, "a stored-multiplier fallback must not be reported as provider-resolved")
		require.True(t, decimal.NewFromInt(40).Equal(res.ResolvedMultiplier.Value), "got %s", res.ResolvedMultiplier.Value)

		var fallback []integration.Warning
		for _, w := range res.Warnings {
			if w.Code == integration.WarnMultiplierFallback {
				fallback = append(fallback, w)
			}
		}
		require.Len(t, fallback, 1)
		require.Contains(t, fallback[0].Detail, "40", "warning should name the reused stored value")
	})
}

// TestGridBoxOffsetlessTimestampIsIstanbul: a ProfileDate carrying no
// offset ("2026-09-14T10:15:00") is Europe/Istanbul local time (UTC+3),
// never UTC — removed-behaviour 20.
func TestGridBoxOffsetlessTimestampIsIstanbul(t *testing.T) {
	from, to := istanbulDay(2026, 9, 14)
	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_offsetless.json"))},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.True(t, res.Readings[0].Ts.Equal(time.Date(2026, 9, 14, 7, 15, 0, 0, time.UTC)),
		"got %s, want 2026-09-14T07:15:00Z", res.Readings[0].Ts)
}

// TestGridBoxResultStatusErrorIsSurfaced is R25: ResultStatus != 1 is
// ErrUpstreamUnavailable, never an empty result.
func TestGridBoxResultStatusErrorIsSurfaced(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_result_status_error.json"))},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	require.Empty(t, res.Readings)
}

// TestGridBoxBillingOnlyWhenEnabled: recorded requests contain
// isBilling=true iff creds.Settings.UseBillingIndexes is on.
func TestGridBoxBillingOnlyWhenEnabled(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	t.Run("disabled", func(t *testing.T) {
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/endexes", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)

		kinds := src.Kinds(creds)
		require.NotContains(t, kinds, model.ReadingKindBilling)
		require.Contains(t, kinds, model.ReadingKindDaily)

		_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindDaily, from, to))
		require.NoError(t, err)

		require.True(t, requestQueryContains(t, srv, "/gridbox/endexes", "isBilling=false"))
	})

	t.Run("enabled", func(t *testing.T) {
		srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
			fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
			fake.Route{Method: http.MethodGet, Path: "/gridbox/endexes", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
		)...)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, true)

		kinds := src.Kinds(creds)
		require.Contains(t, kinds, model.ReadingKindBilling)

		_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindBilling, from, to))
		require.NoError(t, err)

		require.True(t, requestQueryContains(t, srv, "/gridbox/endexes", "isBilling=true"))
	})
}

func requestQueryContains(t *testing.T, srv *fake.Server, path, needle string) bool {
	t.Helper()
	for _, r := range srv.Requests() {
		if r.Path == path && strings.Contains(r.RawQuery, needle) {
			return true
		}
	}
	return false
}

// --- shared adapter template's "also required in every adapter" tests -----

func TestIdempotentGridBoxRefetchYieldsIdenticalReadings(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)
	req := gridboxTestRequest(model.ReadingKindLoadProfile, from, to)

	res1, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)
	res2, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)

	sortReadings(res1.Readings)
	sortReadings(res2.Readings)
	require.Equal(t, res1.Readings, res2.Readings)
}

func TestGridBoxVerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: gridboxAuthFailureResponder(t)})
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)

	err := src.Verify(context.Background(), gridboxTestCreds(srv, false))
	require.ErrorIs(t, err, integration.ErrAuth)
}

// TestGridBoxDataCallsSerializePerCompany is fix round 1 finding I1: a
// recording Locker must see the per-company key on every DATA-endpoint call
// a real FetchReadings makes (last_success_date, last_endex, load_profiles),
// not only on the token exchange — provider-defaults.md's `gridbox` row
// marks "Serialise per company: yes" for the whole provider. The token
// exchange acquires its own, distinct key ("gridbox:token:<company>" —
// tokenClient's doc in source.go explains why the two are kept separate).
//
// Mutation proof (fix round 1): removing dataClientConfig's SerializeKey
// (source.go) makes the "data key" assertion below FAIL with 0 acquires of
// "gridbox:<company>" — recorded in task-7-report.md's "Fix round 1"
// section.
func TestGridBoxDataCallsSerializePerCompany(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
	)...)
	locker := &fakeLocker{}
	pool, _ := gridboxTestPoolWithLocker(t, srv, locker)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)

	dataKey := "gridbox:" + creds.CompanyID.String()
	tokenKey := "gridbox:token:" + creds.CompanyID.String()

	var dataAcquires, tokenAcquires int
	for _, e := range locker.recorded() {
		switch e {
		case "acquire:" + dataKey:
			dataAcquires++
		case "acquire:" + tokenKey:
			tokenAcquires++
		}
	}
	// Three data-endpoint calls this fetch makes: last_success_date,
	// last_endex, load_profiles.
	require.Equal(t, 3, dataAcquires, "every data-endpoint call must acquire the per-company data serialise key")
	require.Equal(t, 1, tokenAcquires, "the token exchange must acquire its own, distinct serialise key")
}

// TestGridBoxDiscoverHasNoListingEndpoint documents source.go's
// DiscoverMeteringPoints doc: 06 §3's Endpoints table has no
// discovery/listing endpoint for GridBox, so Discover always returns zero
// points (after still proving the credential authenticates).
func TestGridBoxDiscoverHasNoListingEndpoint(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_token.json"))})
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)

	points, err := src.DiscoverMeteringPoints(context.Background(), gridboxTestCreds(srv, false))
	require.NoError(t, err)
	require.Empty(t, points)
}

// TestGridBoxLastSuccessDateCapsToNeverRaisesFrom is R50/I1's regression
// test for the SEMANTIC half of the ruling (the shape half — decoding
// ResultObject as a bare string, not an object — is proven by every other
// test in this file now that gridbox_last_success_date.json carries the
// legacy bare-string shape; reverting decodeLastSuccessDate to the old
// object type makes every FetchReadings-driven test in this file fail with
// "integration: malformed payload").
//
// The request window is [Sep 1, Sep 10) Istanbul; last_success_date reports
// Sep 5 (a provider high-water mark INSIDE the window). Round 1 used this
// to RAISE From to Sep 5, producing a request for [Sep 5, Sep 10) —
// startDate=2026-09-05. R18/R37's re-ruling (R50) says last_success_date
// must never raise From (only the pipeline's own cursor does) and may only
// CAP To, so the fixed adapter must request [Sep 1, Sep 5) instead —
// startDate=2026-09-01 (unraised), endDate=2026-09-04 (capped to the day
// before the provider's last success).
func TestGridBoxLastSuccessDateCapsToNeverRaisesFrom(t *testing.T) {
	from, _ := istanbulDay(2026, 9, 1)
	_, to := istanbulDay(2026, 9, 9) // [Sep 1, Sep 10) Istanbul

	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_token.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-success-date", Respond: fake.JSON(200, []byte(`{"ResultStatus":1,"ResultObject":"2026-09-05T00:00:00+03:00"}`))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_success.json"))},
	)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)

	var calls []fake.RecordedRequest
	for _, r := range srv.Requests() {
		if r.Path == "/gridbox/load-profiles" {
			calls = append(calls, r)
		}
	}
	require.Len(t, calls, 1)
	require.Contains(t, calls[0].RawQuery, "startDate=2026-09-01", "last_success_date must never raise From")
	require.Contains(t, calls[0].RawQuery, "endDate=2026-09-04", "last_success_date must cap To to the day before the provider's last success")
}

func TestGridBoxNeverReturnsReadingsOutsideWindow(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"ResultStatus":1,"ResultObject":[
		{"ProfileDateTime":"2026-08-31T10:00:00+03:00","ActiveEndex":1},
		{"ProfileDateTime":"2026-09-01T10:00:00+03:00","ActiveEndex":2},
		{"ProfileDateTime":"2026-09-02T10:00:00+03:00","ActiveEndex":3}
	]}`)
	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, body)},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.True(t, res.Readings[0].Ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)))
}

func TestGridBoxErrorsCarryNoCredential(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: gridboxAuthFailureResponder(t)})
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)
	from, to := istanbulDay(2026, 9, 1)

	_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "FIXTURE-pass")
	require.NotContains(t, fmt.Sprintf("%+v", err), "FIXTURE-pass")
}

// --- bonus: PageBudget / NextCursor (R4/R20) --------------------------------

// TestGridBoxPageBudgetReturnsNextCursor is not one of the brief's named
// tests, but it is the direct proof of R4/R20/R34 pagination the
// FixtureMatrix's "pagination" subtest (default PageBudget, both chunks in
// one call) does not exercise on its own: a PageBudget of 1 against a
// 45-day (two 30-day-max chunk) window must return a NextCursor at the end
// of the first, fully-covered chunk (Ruling R34), and a follow-up call from
// that cursor must fetch exactly the second chunk — never re-fetching the
// first (adapter review pattern 5: request-count and date-param checks,
// not just a result-count check, which a repeated first page would still
// satisfy) — with the two calls' readings merged (by the caller here) in
// order and without duplicates, and the resumed call fetching no less data
// than a single unbudgeted call would have (matches the "pagination"
// subtest's 2-reading total).
func TestGridBoxPageBudgetReturnsNextCursor(t *testing.T) {
	from, _ := istanbulDay(2026, 1, 1)
	chunk2Start, _ := istanbulDay(2026, 1, 31)
	_, to := istanbulDay(2026, 2, 14)
	chunk2Body := []byte(`{"ResultStatus":1,"ResultObject":[{"ProfileDateTime":"2026-02-01T10:00:00+03:00","ActiveEndex":200}]}`)

	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.Sequence(
			fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_pagination.json")),
			fake.JSON(200, chunk2Body),
		)},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 1)
	creds := gridboxTestCreds(srv, false)

	res1, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res1.Readings, 1)
	require.NotNil(t, res1.NextCursor)
	require.True(t, res1.NextCursor.Equal(chunk2Start), "got %s, want %s", res1.NextCursor, chunk2Start)

	res2, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, *res1.NextCursor, to))
	require.NoError(t, err)
	require.Len(t, res2.Readings, 1)
	require.Nil(t, res2.NextCursor)

	require.False(t, res1.Readings[0].Ts.Equal(res2.Readings[0].Ts), "no duplicates across the two calls")
	require.True(t, res1.Readings[0].Ts.Before(res2.Readings[0].Ts), "merged in order")

	var loadProfileCalls []fake.RecordedRequest
	for _, r := range srv.Requests() {
		if r.Path == "/gridbox/load-profiles" {
			loadProfileCalls = append(loadProfileCalls, r)
		}
	}
	require.Len(t, loadProfileCalls, 2, "one call per chunk across the two FetchReadings invocations")
	require.Contains(t, loadProfileCalls[0].RawQuery, "startDate=2026-01-01")
	require.Contains(t, loadProfileCalls[0].RawQuery, "endDate=2026-01-30")
	require.Contains(t, loadProfileCalls[1].RawQuery, "startDate=2026-01-31")
	require.Contains(t, loadProfileCalls[1].RawQuery, "endDate=2026-02-14")
}

// --- adapter review patterns (Task 6 OSOS opus review, applied here) ------

// TestGridBoxStampsEveryTemplateRequiredField is adapter review pattern 1:
// every field the template requires an adapter to stamp — Ts, Kind,
// AnalyzerID, SourceProvider, MultiplierApplied, meter serial, and (pattern
// 14) the row's original raw bytes — is asserted together on one reading.
func TestGridBoxStampsEveryTemplateRequiredField(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	rowBody := []byte(`{"ProfileDateTime":"2026-09-01T10:00:00+03:00","ActiveEndex":100,"MeterSerialNumber":"SN-FIELD-TEST"}`)
	body := []byte(`{"ResultStatus":1,"ResultObject":[` + string(rowBody) + `]}`)

	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex_multiplier.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, body)},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	r := res.Readings[0]

	require.True(t, r.Ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)), "Ts")
	require.Equal(t, model.ReadingKindLoadProfile, r.Kind, "Kind")
	require.Equal(t, gridboxAnalyzerID, r.AnalyzerID, "AnalyzerID")
	require.Equal(t, model.IntegrationProviderGridbox, r.SourceProvider, "SourceProvider")
	require.True(t, decimal.NewFromInt(80).Equal(r.MultiplierApplied), "MultiplierApplied")
	require.NotNil(t, r.MeterSerial, "meter serial")
	require.Equal(t, "SN-FIELD-TEST", *r.MeterSerial)
	require.JSONEq(t, string(rowBody), string(r.Raw), "Raw must be the row's own original bytes (pattern 14), not a re-marshalled struct")
}

// TestGridBoxNeverReturnsReadingsOutsideWindowPreciseBoundary is adapter
// review pattern 4: the half-open window [From, To) is tested at its exact
// edges, not just "somewhere outside, somewhere inside" — From-1s dropped,
// From itself kept, To itself dropped.
func TestGridBoxNeverReturnsReadingsOutsideWindowPreciseBoundary(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	beforeFrom := from.Add(-time.Second)

	body := []byte(fmt.Sprintf(`{"ResultStatus":1,"ResultObject":[
		{"ProfileDateTime":"%s","ActiveEndex":1},
		{"ProfileDateTime":"%s","ActiveEndex":2},
		{"ProfileDateTime":"%s","ActiveEndex":3}
	]}`, beforeFrom.Format(time.RFC3339), from.Format(time.RFC3339), to.Format(time.RFC3339)))

	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, body)},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "only the row at exactly From must survive")
	require.True(t, res.Readings[0].Ts.Equal(from))
}

// TestGridBoxMalformedEnvelopeShapes is adapter review pattern 6: a 200
// response whose top-level key is missing, null, or explicitly null
// ({} / a bare JSON null body / {"ResultStatus":null}) is ErrMalformedPayload
// — never an empty success, and never confused with a legitimate
// "ResultStatus: 0" business failure (ErrUpstreamUnavailable, covered by
// TestGridBoxResultStatusErrorIsSurfaced).
func TestGridBoxMalformedEnvelopeShapes(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty_object", `{}`},
		{"bare_null", `null`},
		{"explicit_null_result_status", `{"ResultStatus":null,"ResultObject":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
				fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
				fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, []byte(tc.body))},
			)...)
			pool, _ := gridboxTestPool(t, srv)
			src := gridboxNewSource(pool, 0)
			creds := gridboxTestCreds(srv, false)

			res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
			require.ErrorIs(t, err, integration.ErrMalformedPayload)
			require.NotErrorIs(t, err, integration.ErrUpstreamUnavailable)
			require.Empty(t, res.Readings)
		})
	}
}

// TestGridBoxPerRowDecodeFailureIsolated is adapter review pattern 7: rows
// are decoded one at a time, so ONE structurally-malformed row (here,
// ActiveEndex given as a JSON object instead of a number — not just a bad
// timestamp string, which the existing "partial" fixture already covers,
// but a genuine shape mismatch on a *json.Number field) produces a
// WarnUnparseableRow for that row alone, never an ErrMalformedPayload for
// the whole page.
// M4 (fix round 1 finding): every register field decodes into *json.Number
// (wire.go), so normalize.OptionalNumber's own "no value" sentinel strings
// — "" and "-" — are NOT valid JSON for that type (unlike a genuine bare
// number or null): encoding/json refuses to unmarshal them into a
// json.Number at all. A row sending one is therefore a per-row decode
// FAILURE exactly like a structurally wrong-shaped value ({}), isolated to
// a WarnUnparseableRow (pattern 7) — never silently treated as a zero
// register (pattern 3, nil-never-zero): the row is dropped entirely, not
// mapped with a zero-valued ActiveEndex.
func TestGridBoxPerRowDecodeFailureIsolated(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  string // raw JSON for the malformed row's ActiveEndex field
	}{
		{"wrong_shaped_value", `{}`},
		{"empty_string_sentinel", `""`},
		{"dash_sentinel", `"-"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, to := istanbulDay(2026, 9, 1)
			body := []byte(fmt.Sprintf(`{"ResultStatus":1,"ResultObject":[
				{"ProfileDateTime":"2026-09-01T10:00:00+03:00","ActiveEndex":100},
				{"ProfileDateTime":"2026-09-01T10:15:00+03:00","ActiveEndex":%s},
				{"ProfileDateTime":"2026-09-01T10:30:00+03:00","ActiveEndex":300}
			]}`, tc.bad))

			srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
				fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
				fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, body)},
			)...)
			pool, _ := gridboxTestPool(t, srv)
			src := gridboxNewSource(pool, 0)
			creds := gridboxTestCreds(srv, false)

			res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
			require.NoError(t, err, "one bad row must not fail the whole page")
			require.Len(t, res.Readings, 2, "the two structurally-valid rows must still be returned")
			for _, r := range res.Readings {
				require.NotNil(t, r.ActiveImport)
				require.False(t, r.ActiveImport.IsZero(), "the malformed row (never a genuine 0) must not surface as a zero-valued reading — it must be dropped entirely")
			}

			var found bool
			for _, w := range res.Warnings {
				if w.Code == integration.WarnUnparseableRow {
					found = true
				}
			}
			require.True(t, found, "the malformed row must still produce a warning, not be silently dropped")
		})
	}
}

// TestGridBoxDuplicateTimestampConflictWarns is adapter review pattern 13:
// two rows sharing one Ts whose mapped registers DIFFER are not silently
// merged or arbitrarily dropped — the first is kept and a warning notes the
// conflict.
func TestGridBoxDuplicateTimestampConflictWarns(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)
	body := []byte(`{"ResultStatus":1,"ResultObject":[
		{"ProfileDateTime":"2026-09-01T10:00:00+03:00","ActiveEndex":100},
		{"ProfileDateTime":"2026-09-01T10:00:00+03:00","ActiveEndex":999}
	]}`)

	srv := fake.NewTLSServer(t, append(gridboxBaseRoutes(t),
		fake.Route{Method: http.MethodGet, Path: "/gridbox/last-endex", Respond: fake.JSON(200, fake.Fixture(t, "gridbox", "gridbox_last_endex.json"))},
		fake.Route{Method: http.MethodGet, Path: "/gridbox/load-profiles", Respond: fake.JSON(200, body)},
	)...)
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)
	creds := gridboxTestCreds(srv, false)

	res, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "the conflicting duplicate must not produce a second reading")
	require.True(t, decimal.NewFromInt(100).Equal(*res.Readings[0].ActiveImport), "the FIRST occurrence is kept")

	var found bool
	for _, w := range res.Warnings {
		if w.Code == integration.WarnUnparseableRow && strings.Contains(w.Detail, "duplicate timestamp") {
			found = true
		}
	}
	require.True(t, found, "expected a duplicate-timestamp warning, got %+v", res.Warnings)
}

// TestGridBoxConfigErrorsAreDeliberate is adapter review patterns 9 and 12:
// a missing endpoint template or a blank installation number is reported as
// a deliberately-classified *integration.Error (ErrConfig — R48/I5;
// see configError's doc in source.go), before ANY HTTP call is attempted,
// and MUST NOT be ErrAuth (F3's credential-health logic must never treat a
// misconfiguration as a rejected credential).
func TestGridBoxConfigErrorsAreDeliberate(t *testing.T) {
	from, to := istanbulDay(2026, 9, 1)

	t.Run("missing_endpoint_key", func(t *testing.T) {
		srv := fake.NewTLSServer(t) // no routes: any request fails the test
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)
		delete(creds.Endpoints, "load_profiles")

		_, err := src.FetchReadings(context.Background(), creds, gridboxTestRequest(model.ReadingKindLoadProfile, from, to))
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
		var ierr *integration.Error
		require.ErrorAs(t, err, &ierr)
		require.Equal(t, "config:load_profiles", ierr.Op)
		require.Empty(t, srv.Requests(), "no HTTP call must be attempted for an incomplete configuration")
	})

	t.Run("blank_wiring_number", func(t *testing.T) {
		srv := fake.NewTLSServer(t)
		pool, _ := gridboxTestPool(t, srv)
		src := gridboxNewSource(pool, 0)
		creds := gridboxTestCreds(srv, false)
		req := gridboxTestRequest(model.ReadingKindLoadProfile, from, to)
		req.Point.InstallationNumber = ""

		_, err := src.FetchReadings(context.Background(), creds, req)
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
		require.Empty(t, srv.Requests())
	})
}

// TestGridBoxTokenExchangeNeverRetries is adapter review pattern 10 (ruling
// R32): the token exchange POST is marked NoRetry, so a retryable-classified
// failure (here, 500 -> ErrUpstreamUnavailable, which IS retryable in
// general) still produces exactly one HTTP attempt. Removing NoRetry: true
// from token()'s httpx.Request makes this test FAIL (calls == 3, the
// Client's default MaxAttempts).
func TestGridBoxTokenExchangeNeverRetries(t *testing.T) {
	var mu sync.Mutex
	var calls int
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/gridbox/token", Respond: func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}})
	pool, _ := gridboxTestPool(t, srv)
	src := gridboxNewSource(pool, 0)

	err := src.Verify(context.Background(), gridboxTestCreds(srv, false))
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls, "R32: a non-idempotent token exchange POST must never be retried in-client")
}
