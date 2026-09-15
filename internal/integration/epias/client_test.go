package epias_test

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/epias"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

const (
	casPath  = "/cas/v1/tickets"
	mcpPath  = "/electricity-service/v1/markets/dam/data/mcp"
	yekPath  = "/electricity-service/v1/renewables/data/unit-cost"
	testUser = "FIXTURE-epias-user"
	testPass = "FIXTURE-SECRET-pw91"
)

var fixtureNow = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)

// recordingSleep is a fake PoolOptions.Sleep, matching the shape
// internal/integration/httpx's own test file uses: it records every
// requested delay and returns immediately, so no test here ever waits in
// real time.
type recordingSleep struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (r *recordingSleep) fn(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.delays = append(r.delays, d)
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *recordingSleep) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.delays))
	copy(out, r.delays)
	return out
}

// epiasTestPool builds a Pool pinned to srv's own certificate, with a
// no-op recording Sleep and — when locker is non-nil — a Locker, so
// httpx's own SerializeKey mechanism (and, separately, epias.Options.Locker
// for the TGT cache) can be exercised deterministically.
func epiasTestPool(t *testing.T, srv *fake.Server, locker httpx.Locker) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: srv.Pins,
		Sleep:       rs.fn,
		Locker:      locker,
	})
	require.NoError(t, err)
	return pool, rs
}

func epiasTestOptions(srv *fake.Server, c clock.Clock, locker httpx.Locker) epias.Options {
	return epias.Options{
		CASURL:   srv.URL + casPath,
		BaseURL:  srv.URL + "/electricity-service",
		Username: testUser,
		Password: integration.NewSecret([]byte(testPass)),
		Clock:    c,
		Locker:   locker,
	}
}

// casSuccess responds 201 Created with the ticket in the Location header,
// the shape task-12's brief documents as primary.
func casSuccess(ticket string) fake.Responder {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://giris.epias.com.tr/cas/v1/tickets/"+ticket)
		w.WriteHeader(http.StatusCreated)
	}
}

// casSuccessInBody responds 201 Created with the ticket only in the body —
// the documented fallback location.
func casSuccessInBody(ticket string) fake.Responder {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(ticket))
	}
}

func casRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: casPath, Respond: respond}
}

func mcpRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: mcpPath, Respond: respond}
}

func yekRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: yekPath, Respond: respond}
}

// dataRequests filters srv's recorded requests to the MCP data endpoint,
// in the order they were received.
func dataRequests(srv *fake.Server) []fake.RecordedRequest {
	var out []fake.RecordedRequest
	for _, r := range srv.Requests() {
		if r.Path == mcpPath {
			out = append(out, r)
		}
	}
	return out
}

func casRequests(srv *fake.Server) []fake.RecordedRequest {
	var out []fake.RecordedRequest
	for _, r := range srv.Requests() {
		if r.Path == casPath {
			out = append(out, r)
		}
	}
	return out
}

// oneIstanbulDay returns [from, from+24h) for the given Istanbul-local
// calendar date, as the UTC instants epias.Client's methods take.
func oneIstanbulDay(y int, m time.Month, d int) (time.Time, time.Time) {
	from := time.Date(y, m, d, 0, 0, 0, 0, normalize.Istanbul)
	to := time.Date(y, m, d+1, 0, 0, 0, 0, normalize.Istanbul)
	return from.UTC(), to.UTC()
}

// TestEPIASFixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination" applied to EPİAŞ's HourlyPTF. The subtest names are exactly
// fake.RequiredCases.
func TestEPIASFixtureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		routes  func(t *testing.T) []fake.Route
		window  func() (time.Time, time.Time)
		wantErr error
		check   func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning)
	}{
		{
			name: "success",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-1-success-cas01")),
					mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
				}
			},
			check: func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning) {
				require.Len(t, prices, 24)
				require.Empty(t, warnings)
				require.Equal(t, "2345.6789", prices[0].PTF.String())
			},
		},
		{
			name: "empty",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-2-empty-cas01")),
					mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_empty.json"))),
				}
			},
			check: func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning) {
				require.Empty(t, prices)
				require.Empty(t, warnings)
			},
		},
		{
			name: "partial",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-3-partial-cas01")),
					mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_partial.json"))),
				}
			},
			check: func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning) {
				require.Len(t, prices, 23)
				require.Len(t, warnings, 1)
				require.Equal(t, integration.WarnUnparseableRow, warnings[0].Code)
			},
		},
		{
			name: "auth_failure",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(fake.Raw(http.StatusUnauthorized, "", nil)),
				}
			},
			wantErr: integration.ErrAuth,
		},
		{
			name: "malformed",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-4-malformed-cas01")),
					// A 200 whose top-level shape is neither known EPİAŞ
					// shape (no "items", no "body.dayAheadMCPList") — valid
					// JSON, wrong shape, per adapter-patterns.md #6.
					// Genuinely non-JSON bytes are covered separately by
					// TestEPIASRejectsNonJSONBody (an inline byte literal,
					// not a testdata fixture: fake.TestFixturesAreSanitised
					// requires every *.json fixture to itself be valid
					// JSON, so a deliberately-broken body cannot live in
					// testdata/epias_malformed.json).
					mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_malformed.json"))),
				}
			},
			wantErr: integration.ErrMalformedPayload,
		},
		{
			name: "rate_limited",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-5-ratelimited-cas01")),
					mcpRoute(fake.Sequence(
						fake.RateLimited(""), // no Retry-After: EPİAŞ's DefaultRateLimitWait (65s) applies
						fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_rate_limited.json")),
					)),
				}
			},
			check: func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning) {
				require.Len(t, prices, 24)
				require.Contains(t, rs.recorded(), 65*time.Second, "a 429 with no Retry-After must wait ClientConfig.DefaultRateLimitWait (65s, legacy)")
			},
		},
		{
			name: "pagination",
			window: func() (time.Time, time.Time) {
				from := time.Date(2026, 2, 1, 0, 0, 0, 0, normalize.Istanbul)
				to := time.Date(2026, 2, 1+45, 0, 0, 0, 0, normalize.Istanbul)
				return from.UTC(), to.UTC()
			},
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					casRoute(casSuccess("TGT-6-pagination-cas01")),
					mcpRoute(fake.Sequence(
						fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_pagination.json")),
						fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_pagination_chunk2.json")),
					)),
				}
			},
			check: func(t *testing.T, srv *fake.Server, rs *recordingSleep, prices []model.MarketPrice, warnings []integration.Warning) {
				reqs := dataRequests(srv)
				require.Len(t, reqs, 2, "a 45-day window must chunk to two 30-day-max requests (R20/R36)")

				from := time.Date(2026, 2, 1, 0, 0, 0, 0, normalize.Istanbul)
				to := time.Date(2026, 2, 1+45, 0, 0, 0, 0, normalize.Istanbul)
				windows := normalize.Chunk(from.UTC(), to.UTC(), 30*24*time.Hour)
				require.Len(t, windows, 2)
				for i, w := range windows {
					require.Contains(t, string(reqs[i].Body), w.From.In(normalize.Istanbul).Format("2006-01-02T15:04:05"), "chunk %d startDate", i)
					require.Contains(t, string(reqs[i].Body), w.To.In(normalize.Istanbul).Format("2006-01-02T15:04:05"), "chunk %d endDate", i)
				}
				require.Len(t, prices, 2)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			pool, rs := epiasTestPool(t, srv, nil)
			c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
			require.NoError(t, err)

			from, to := oneIstanbulDay(2026, 1, 5)
			if tc.window != nil {
				from, to = tc.window()
			}

			prices, warnings, err := c.HourlyPTF(context.Background(), from, to)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, srv, rs, prices, warnings)
		})
	}
}

// TestIdempotentEPIASRefetchYieldsIdenticalPrices: the same fixture served
// twice gives identical price slices.
func TestIdempotentEPIASRefetchYieldsIdenticalPrices(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-7-idempotent-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	ctx := context.Background()

	first, _, err := c.HourlyPTF(ctx, from, to)
	require.NoError(t, err)
	second, _, err := c.HourlyPTF(ctx, from, to)
	require.NoError(t, err)

	sortPrices(first)
	sortPrices(second)
	require.Equal(t, len(first), len(second))
	for i := range first {
		require.True(t, first[i].Ts.Equal(second[i].Ts))
		require.True(t, first[i].PTF.Equal(second[i].PTF), "row %d: %s != %s", i, first[i].PTF, second[i].PTF)
	}
}

func sortPrices(p []model.MarketPrice) {
	sort.Slice(p, func(i, j int) bool { return p[i].Ts.Before(p[j].Ts) })
}

// TestEPIASTicketIsCachedAndRefreshedOnce: two data calls cause one CAS
// call; a data 401 causes exactly one more CAS call, then success.
func TestEPIASTicketIsCachedAndRefreshedOnce(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(fake.Sequence(
			casSuccess("TGT-8-first-cas01"),
			casSuccess("TGT-8-second-cas02"),
		)),
		mcpRoute(fake.Sequence(
			fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json")), // call 1: cached ticket
			fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json")), // call 2: still cached
			fake.Raw(http.StatusUnauthorized, "", nil),                               // call 3, attempt 1: expired/rejected
			fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json")), // call 3, attempt 2: fresh ticket
		)),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	ctx := context.Background()

	_, _, err = c.HourlyPTF(ctx, from, to)
	require.NoError(t, err)
	_, _, err = c.HourlyPTF(ctx, from, to)
	require.NoError(t, err)
	require.Len(t, casRequests(srv), 1, "two data calls on an unexpired ticket must cause exactly one CAS call")

	prices, _, err := c.HourlyPTF(ctx, from, to)
	require.NoError(t, err)
	require.Len(t, prices, 24)
	require.Len(t, casRequests(srv), 2, "a data 401 must cause exactly one more CAS call")
	require.Len(t, dataRequests(srv), 4)

	// The two tickets obtained must actually differ, or this test would
	// not distinguish "refreshed" from "reused the same string twice".
	reqs := dataRequests(srv)
	require.NotEqual(t, reqs[0].Header.Get("Tgt"), reqs[3].Header.Get("Tgt"))
}

// TestEPIASTicketRefreshIsSingleFlightAcrossGoroutines proves the
// double-checked-locking design in ticket.go's getTicket: two goroutines
// racing on a cold cache, both needing a fresh ticket, cause exactly one
// CAS call when Options.Locker actually serialises them. This is the test
// Step 5(c) of the task brief re-runs after deleting the re-check under the
// lock, to prove it currently guards something real.
func TestEPIASTicketRefreshIsSingleFlightAcrossGoroutines(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-9-concurrent-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
	)
	locker := lock.NewMemory(func() time.Time { return fixtureNow })
	pool, _ := epiasTestPool(t, srv, locker)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), locker))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := c.HourlyPTF(ctx, from, to)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Len(t, casRequests(srv), 1, "two goroutines racing on a cold ticket cache must cause exactly one CAS call")
}

// TestEPIASErrorsCarryNoPasswordOrTicket: neither the password nor an
// obtained ticket ever appears in an error's text, whichever stage fails.
func TestEPIASErrorsCarryNoPasswordOrTicket(t *testing.T) {
	const ticket = "TGT-10-leaktest-cas01"

	t.Run("cas auth failure", func(t *testing.T) {
		srv := fake.NewTLSServer(t, casRoute(fake.Raw(http.StatusUnauthorized, "", nil)))
		pool, _ := epiasTestPool(t, srv, nil)
		c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
		require.NoError(t, err)

		from, to := oneIstanbulDay(2026, 1, 5)
		_, _, err = c.HourlyPTF(context.Background(), from, to)
		require.Error(t, err)
		require.NotContains(t, err.Error(), testPass)
		require.NotContains(t, err.Error(), testUser)
	})

	t.Run("data call persistently unauthorized", func(t *testing.T) {
		srv := fake.NewTLSServer(t,
			casRoute(casSuccess(ticket)),
			mcpRoute(fake.Raw(http.StatusUnauthorized, "", nil)),
		)
		pool, _ := epiasTestPool(t, srv, nil)
		c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
		require.NoError(t, err)

		from, to := oneIstanbulDay(2026, 1, 5)
		_, _, err = c.HourlyPTF(context.Background(), from, to)
		require.Error(t, err)
		require.NotContains(t, err.Error(), testPass)
		require.NotContains(t, err.Error(), ticket)
	})
}

// TestEPIASPricesAreExactDecimals: "2345.6789" survives byte-for-byte, not
// as an approximate float64 round trip.
func TestEPIASPricesAreExactDecimals(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-11-decimals-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	prices, _, err := c.HourlyPTF(context.Background(), from, to)
	require.NoError(t, err)
	require.NotEmpty(t, prices)
	require.Equal(t, "2345.6789", prices[0].PTF.String())
}

// TestEPIASParsesItemsShape exercises the "items[]" response shape — the
// primary shape per the brief, exercised via its own dedicated fixture
// (epias_items_shape.json) distinct from epias_success.json, which uses
// the "body.dayAheadMCPList[]" fallback shape — so both known shapes have
// real fixture+test coverage.
func TestEPIASParsesItemsShape(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-12-itemsshape-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_items_shape.json"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 8)
	prices, warnings, err := c.HourlyPTF(context.Background(), from, to)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, prices, 24)
}

// TestEPIASYekdemUnitCost exercises YekdemUnitCost end to end, including
// the period/date field fallback (epias_yekdem.json carries one row of
// each).
func TestEPIASYekdemUnitCost(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-13-yekdem-cas01")),
		yekRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_yekdem.json"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 3, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	values, err := c.YekdemUnitCost(context.Background(), from, to)
	require.NoError(t, err)
	require.Len(t, values, 2)

	byMonth := map[int16]string{}
	for _, v := range values {
		byMonth[v.Month] = v.Value.String()
	}
	require.Equal(t, "185.1234", byMonth[1])
	require.Equal(t, "190.5678", byMonth[2])
}

// TestEPIASRejectsNonJSONBody covers the genuinely-broken-syntax half of
// adapter-patterns.md #6 (the fixture matrix's "malformed" case now covers
// only the valid-JSON-wrong-shape half — see its route's comment for why).
// The body is an inline byte literal, not a testdata fixture, precisely so
// it can be actual invalid JSON without tripping
// fake.TestFixturesAreSanitised's "every *.json fixture must itself be
// valid JSON" rule.
func TestEPIASRejectsNonJSONBody(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-17-nonjson-cas01")),
		mcpRoute(fake.Raw(http.StatusOK, "text/html", []byte("<html><body>502 Bad Gateway</body></html>"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	_, _, err = c.HourlyPTF(context.Background(), from, to)
	require.ErrorIs(t, err, integration.ErrMalformedPayload)
}

// TestEPIASConfigErrorsNameTheEnvVarNotTheValue: New refuses an empty
// Username/Password before any network call, and the error names the
// environment variable EKOKOD_EPIAS_USERNAME/EKOKOD_EPIAS_PASSWORD reads
// from — never a value, since there is none to leak.
func TestEPIASConfigErrorsNameTheEnvVarNotTheValue(t *testing.T) {
	pool, err := httpx.NewPool(httpx.PoolOptions{})
	require.NoError(t, err)

	_, err = epias.New(pool, epias.Options{Password: integration.NewSecret([]byte(testPass))})
	require.ErrorIs(t, err, integration.ErrAuth)
	require.Contains(t, err.Error(), "EKOKOD_EPIAS_USERNAME")

	_, err = epias.New(pool, epias.Options{Username: testUser})
	require.ErrorIs(t, err, integration.ErrAuth)
	require.Contains(t, err.Error(), "EKOKOD_EPIAS_PASSWORD")
}

// TestEPIASAcceptsTicketFromBody: the CAS response's Location header is
// the documented primary location for the ticket, but a response that
// carries it only in the body (legacy epiasService.ts's documented
// fallback) still works.
func TestEPIASAcceptsTicketFromBody(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccessInBody("TGT-15-inbody-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
	)
	pool, _ := epiasTestPool(t, srv, nil)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	prices, _, err := c.HourlyPTF(context.Background(), from, to)
	require.NoError(t, err)
	require.Len(t, prices, 24)
}

// recordingLocker wraps a real lock.Memory (so Acquire genuinely
// serialises, not just records) and remembers every key Acquire was
// called with — adapter-patterns.md item 15's "a recording Locker test
// asserts the key on DATA calls".
type recordingLocker struct {
	inner httpx.Locker

	mu   sync.Mutex
	keys []string
}

func newRecordingLocker(now func() time.Time) *recordingLocker {
	return &recordingLocker{inner: lock.NewMemory(now)}
}

func (l *recordingLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (httpx.Lease, error) {
	l.mu.Lock()
	l.keys = append(l.keys, key)
	l.mu.Unlock()
	return l.inner.Acquire(ctx, key, ttl)
}

func (l *recordingLocker) recordedKeys() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.keys))
	copy(out, l.keys)
	return out
}

// TestEPIASDataCallsAreGloballySerialised proves adapter-patterns.md item
// 15 for this provider: EVERY httpx client this package builds — the CAS
// client AND the data client — carries ClientConfig.SerializeKey "epias"
// (the Provider defaults table's "global lock `epias`"), not just the
// auth call. The Locker here is the Pool's own (httpx's per-attempt
// SerializeKey mechanism), deliberately distinct from
// epias.Options.Locker (which guards only the in-process TGT cache under
// "epias:tgt" and is left nil here) — this test is about the lock every
// attempt takes, not the ticket cache.
func TestEPIASDataCallsAreGloballySerialised(t *testing.T) {
	srv := fake.NewTLSServer(t,
		casRoute(casSuccess("TGT-16-serialise-cas01")),
		mcpRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "epias", "epias_success.json"))),
	)
	rl := newRecordingLocker(func() time.Time { return fixtureNow })
	pool, _ := epiasTestPool(t, srv, rl)
	c, err := epias.New(pool, epiasTestOptions(srv, clock.NewFake(fixtureNow), nil))
	require.NoError(t, err)

	from, to := oneIstanbulDay(2026, 1, 5)
	_, _, err = c.HourlyPTF(context.Background(), from, to)
	require.NoError(t, err)

	keys := rl.recordedKeys()
	require.GreaterOrEqual(t, len(keys), 2, "expected a lock acquire for both the CAS attempt and the data attempt")
	for _, k := range keys {
		require.Equal(t, "epias", k, "every httpx attempt (auth and data) must serialise under the global epias key")
	}
}
