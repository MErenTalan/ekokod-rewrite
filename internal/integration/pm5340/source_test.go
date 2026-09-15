package pm5340_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/pm5340"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

var fixtureNow = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
var windowFrom = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
var windowTo = time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)

// recordingSleep is a no-op PoolOptions.Sleep that records every requested
// duration, so a rate-limit/retry wait is asserted on without a test ever
// actually waiting in real time.
type recordingSleep struct {
	mu   sync.Mutex
	durs []time.Duration
}

func (r *recordingSleep) fn(_ context.Context, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durs = append(r.durs, d)
	return nil
}

func (r *recordingSleep) recorded() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.durs))
	copy(out, r.durs)
	return out
}

func pm5340TestPool(t *testing.T, srv *fake.Server) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: rs.fn})
	require.NoError(t, err)
	return pool, rs
}

func pm5340TestCreds(srv *fake.Server) integration.Credentials {
	return integration.Credentials{
		CredentialID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CompanyID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Provider:     integration.ProviderPM5340,
		BaseURL:      srv.URL,
	}
}

func pm5340TestRequest() integration.FetchRequest {
	return integration.FetchRequest{
		AnalyzerID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Multiplier: decimal.RequireFromString("1"),
		Kind:       model.ReadingKindLoadProfile,
		From:       windowFrom,
		To:         windowTo,
	}
}

// hasQueryParam reports whether raw contains a "key=" occurrence, matching
// how httpx.Expand emits a literal, deterministically-ordered query string
// (never a re-encoded url.Values map), so assertions can check for a
// param's presence without caring about escaping details.
func hasQueryParam(raw, key string) bool {
	return strings.Contains(raw, key+"=")
}

// ---- Test<P>FixtureMatrix (F2 acceptance criterion) ----

func TestPM5340FixtureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		routes  func(t *testing.T) []fake.Route
		wantErr error
		check   func(t *testing.T, res integration.FetchResult)
	}{
		{
			name: "success",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method:  http.MethodGet,
					Path:    "/api/v1/readings",
					Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
				}}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 2)
				require.Nil(t, res.NextCursor)
				require.Empty(t, res.Warnings)
			},
		},
		{
			name: "empty",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method:  http.MethodGet,
					Path:    "/api/v1/readings",
					Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_empty.json")),
				}}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Empty(t, res.Readings)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "partial",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method:  http.MethodGet,
					Path:    "/api/v1/readings",
					Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_partial.json")),
				}}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 2)
				var unparseable int
				for _, w := range res.Warnings {
					if w.Code == integration.WarnUnparseableRow {
						unparseable++
					}
				}
				require.GreaterOrEqual(t, unparseable, 1)
			},
		},
		{
			name: "auth_failure",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method:  http.MethodGet,
					Path:    "/api/v1/readings",
					Respond: fake.JSON(http.StatusUnauthorized, fake.Fixture(t, "pm5340", "pm5340_auth_failure.json")),
				}}
			},
			wantErr: integration.ErrAuth,
		},
		{
			name: "malformed",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method:  http.MethodGet,
					Path:    "/api/v1/readings",
					Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_malformed.json")),
				}}
			},
			wantErr: integration.ErrMalformedPayload,
		},
		{
			name: "rate_limited",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{{
					Method: http.MethodGet,
					Path:   "/api/v1/readings",
					Respond: fake.Sequence(
						fake.RateLimited("1"),
						fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
					),
				}}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 2)
			},
		},
		{
			name: "pagination",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					{
						Method:  http.MethodGet,
						Path:    "/api/v1/readings",
						Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "" },
						Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_page1.json")),
					},
					{
						Method:  http.MethodGet,
						Path:    "/api/v1/readings",
						Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "fixture-cursor-page-2" },
						Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_page2.json")),
					},
				}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 4)
				require.Nil(t, res.NextCursor)
				for i := 1; i < len(res.Readings); i++ {
					require.True(t, res.Readings[i-1].Ts.Before(res.Readings[i].Ts) || res.Readings[i-1].Ts.Equal(res.Readings[i].Ts))
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			pool, _ := pm5340TestPool(t, srv)
			src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

			res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, res)
		})
	}
}

// TestPM5340FixtureMatrixRateLimitedRecordsSleep is split out of the matrix
// loop above because it needs the recordingSleep, not just the result.
func TestPM5340FixtureMatrixRateLimitedRecordsSleep(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodGet,
		Path:   "/api/v1/readings",
		Respond: fake.Sequence(
			fake.RateLimited("1"),
			fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
		),
	})
	pool, sleeper := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.Len(t, res.Readings, 2)
	require.Contains(t, sleeper.recorded(), time.Second)
	require.Len(t, srv.Requests(), 2)
}

// ---- Named behaviour tests (task-9 brief) ----

// TestPM5340NeverDerivesCumulativeExport: every reading has ActiveExport ==
// nil, and IntervalGenerationKwh == currentGeneration x 0.25 (removed-
// behaviour 22). See mapping.go's mapRow: the mutation this test must catch
// is accumulating currentGeneration into ActiveExport in the adapter.
func TestPM5340NeverDerivesCumulativeExport(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.NotEmpty(t, res.Readings)

	quarter := decimal.RequireFromString("0.25")
	wantGeneration := []string{"3.600", "4.000"}
	for i, r := range res.Readings {
		require.Nil(t, r.ActiveExport, "reading %d: ActiveExport must never be derived by the adapter", i)
		require.NotNil(t, r.IntervalGenerationKwh)
		want := decimal.RequireFromString(wantGeneration[i]).Mul(quarter)
		require.Truef(t, want.Equal(*r.IntervalGenerationKwh), "reading %d: want %s, got %s", i, want, r.IntervalGenerationKwh)
	}
}

// TestPM5340NullsStayNull: an explicitly-null currentGeneration register
// decodes to nil (removed-behaviour 21) and carries WarnGenerationIntervalNil.
func TestPM5340NullsStayNull(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_null_generation.json")),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.Len(t, res.Readings, 2)
	require.Nil(t, res.Readings[0].IntervalGenerationKwh)
	require.NotNil(t, res.Readings[1].IntervalGenerationKwh)

	var sawGenerationNil bool
	for _, w := range res.Warnings {
		if w.Code == integration.WarnGenerationIntervalNil {
			sawGenerationNil = true
		}
	}
	require.True(t, sawGenerationNil)
}

// TestPM5340FollowsCursorPages: the pagination case, focused on request
// shape — the second request carries cursor=<cursorNext> (adapter-
// patterns.md item 5: count recorded requests and assert their params).
func TestPM5340FollowsCursorPages(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "" },
			Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_page1.json")),
		},
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "fixture-cursor-page-2" },
			Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_page2.json")),
		},
	)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.Len(t, res.Readings, 4)

	reqs := srv.Requests()
	require.Len(t, reqs, 2)
	require.False(t, hasQueryParam(reqs[0].RawQuery, "cursor"))
	require.True(t, hasQueryParam(reqs[1].RawQuery, "cursor"))
	require.Contains(t, reqs[1].RawQuery, "cursor=fixture-cursor-page-2")
}

// TestPM5340PageBudgetExhausted is adapter-patterns.md item 8: the page-
// budget / NextCursor path, where the provider keeps saying hasMore=true
// past the configured budget.
func TestPM5340PageBudgetExhausted(t *testing.T) {
	page1 := `{"items":[{"meterDate":"2026-01-05T08:00:00+03:00","activeImport_kWh":1,"inductive_kvarh":1,"capacitive_kvarh":1,"dmdKwPeak_kW":1,"currentGeneration":1}],"cursorNext":"cursor-1","hasMore":true}`
	page2 := `{"items":[{"meterDate":"2026-01-05T08:15:00+03:00","activeImport_kWh":2,"inductive_kvarh":2,"capacitive_kvarh":2,"dmdKwPeak_kW":2,"currentGeneration":2}],"cursorNext":"cursor-2","hasMore":true}`

	srv := fake.NewTLSServer(t,
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "" },
			Respond: fake.JSON(http.StatusOK, []byte(page1)),
		},
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.Query().Get("cursor") == "cursor-1" },
			Respond: fake.JSON(http.StatusOK, []byte(page2)),
		},
	)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow), PageBudget: 2})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.Len(t, res.Readings, 2)
	require.Len(t, srv.Requests(), 2, "the page budget must stop the loop, never the fake running out of routes")
	require.NotNil(t, res.NextCursor)
	require.True(t, res.NextCursor.Equal(res.Readings[len(res.Readings)-1].Ts), "R34: NextCursor is the last fully covered reading's Ts")
}

// TestPM5340AcceptsBothDateFormats (fixture-level): the full FetchReadings
// path, not just mapRow, accepts a fixture whose rows use DD/MM/YYYY
// HH:mm[:ss].
func TestPM5340AcceptsBothDateFormatsFixture(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_ddmmyyyy.json")),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
	require.NoError(t, err)
	require.Len(t, res.Readings, 2)
}

// TestPM5340PlainHTTPBaseURLIsAllowedAndHTTPSIsVerified: an http fake
// succeeds (PM5340 plain http is allowed — a customer-local device, per
// the plan ruling); an https fake without a pin fails.
func TestPM5340PlainHTTPBaseURLIsAllowedAndHTTPSIsVerified(t *testing.T) {
	t.Run("plain_http_allowed", func(t *testing.T) {
		body := fake.Fixture(t, "pm5340", "pm5340_success.json")
		plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		}))
		t.Cleanup(plain.Close)

		pool, err := httpx.NewPool(httpx.PoolOptions{})
		require.NoError(t, err)
		src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

		creds := integration.Credentials{
			CredentialID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			CompanyID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Provider:     integration.ProviderPM5340,
			BaseURL:      plain.URL,
		}
		res, err := src.FetchReadings(context.Background(), creds, pm5340TestRequest())
		require.NoError(t, err)
		require.Len(t, res.Readings, 2)
	})

	t.Run("https_without_pin_fails", func(t *testing.T) {
		srv := fake.NewTLSServer(t, fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
		})
		// Deliberately built WITHOUT srv.Pins: this https server's
		// certificate is never trusted.
		pool, err := httpx.NewPool(httpx.PoolOptions{})
		require.NoError(t, err)
		src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

		_, err = src.FetchReadings(context.Background(), pm5340TestCreds(srv), pm5340TestRequest())
		require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	})
}

// TestPM5340VerifyUsesItsOwnMinimalTemplate (S5): the recorded Verify
// request's path is /api/v1/readings?limit=1 with no sort/start/end/cursor
// params; a FetchReadings call against the same fake, by contrast, always
// carries all of sort/start/end.
func TestPM5340VerifyUsesItsOwnMinimalTemplate(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.RawQuery == "limit=1" },
			Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_empty.json")),
		},
		fake.Route{
			Method:  http.MethodGet,
			Path:    "/api/v1/readings",
			Match:   func(r *http.Request) bool { return r.URL.RawQuery != "limit=1" },
			Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
		},
	)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})
	creds := pm5340TestCreds(srv)

	require.NoError(t, src.Verify(context.Background(), creds))
	reqs := srv.Requests()
	require.Len(t, reqs, 1)
	require.Equal(t, "limit=1", reqs[0].RawQuery)

	_, err := src.FetchReadings(context.Background(), creds, pm5340TestRequest())
	require.NoError(t, err)
	reqs = srv.Requests()
	require.Len(t, reqs, 2)
	require.True(t, hasQueryParam(reqs[1].RawQuery, "sort"))
	require.True(t, hasQueryParam(reqs[1].RawQuery, "start"))
	require.True(t, hasQueryParam(reqs[1].RawQuery, "end"))
}

// ---- Shared-template-required tests ----

func TestIdempotentPM5340RefetchYieldsIdenticalReadings(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "pm5340", "pm5340_success.json")),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})
	creds := pm5340TestCreds(srv)
	req := pm5340TestRequest()

	res1, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)
	res2, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)

	sortReadings := func(rs []model.MeterReading) {
		sort.Slice(rs, func(i, j int) bool {
			if !rs[i].Ts.Equal(rs[j].Ts) {
				return rs[i].Ts.Before(rs[j].Ts)
			}
			return rs[i].Kind < rs[j].Kind
		})
	}
	sortReadings(res1.Readings)
	sortReadings(res2.Readings)
	require.Equal(t, len(res1.Readings), len(res2.Readings))
	for i := range res1.Readings {
		require.True(t, res1.Readings[i].Ts.Equal(res2.Readings[i].Ts))
		require.Equal(t, res1.Readings[i].Kind, res2.Readings[i].Kind)
		requireEqualDecimalPtr(t, res1.Readings[i].ActiveImport, res2.Readings[i].ActiveImport)
		requireEqualDecimalPtr(t, res1.Readings[i].IntervalGenerationKwh, res2.Readings[i].IntervalGenerationKwh)
	}
}

func requireEqualDecimalPtr(t *testing.T, a, b *decimal.Decimal) {
	t.Helper()
	if a == nil || b == nil {
		require.Equal(t, a == nil, b == nil)
		return
	}
	require.True(t, a.Equal(*b))
}

func TestPM5340VerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusUnauthorized, fake.Fixture(t, "pm5340", "pm5340_auth_failure.json")),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	err := src.Verify(context.Background(), pm5340TestCreds(srv))
	require.ErrorIs(t, err, integration.ErrAuth)
}

func TestPM5340NeverReturnsReadingsOutsideWindow(t *testing.T) {
	// From-1s (dropped), exactly From (kept), exactly To-1... interval end
	// (kept), exactly To (dropped) — half-open [From, To).
	from := time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 6, 0, 0, 0, time.UTC)

	body := `{"items":[
		{"meterDate":"2026-01-05T07:59:59+03:00","activeImport_kWh":1},
		{"meterDate":"2026-01-05T08:00:00+03:00","activeImport_kWh":2},
		{"meterDate":"2026-01-05T08:59:59+03:00","activeImport_kWh":3},
		{"meterDate":"2026-01-05T09:00:00+03:00","activeImport_kWh":4}
	],"cursorNext":null,"hasMore":false}`

	srv := fake.NewTLSServer(t, fake.Route{
		Method:  http.MethodGet,
		Path:    "/api/v1/readings",
		Respond: fake.JSON(http.StatusOK, []byte(body)),
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	req := pm5340TestRequest()
	req.From, req.To = from, to

	res, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 2)
	require.True(t, res.Readings[0].Ts.Equal(from))
	require.True(t, res.Readings[1].Ts.Equal(to.Add(-time.Second)))
}

// TestPM5340ErrorsCarryNoCredential: the auth-failure fixture's server
// echoes the password; err.Error() does not contain it.
func TestPM5340ErrorsCarryNoCredential(t *testing.T) {
	const password = "FIXTURE-never-leaked-pw"
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodGet,
		Path:   "/api/v1/readings",
		Respond: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized for password ` + password + `"}`))
		},
	})
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	creds := pm5340TestCreds(srv)
	creds.Secret = integration.NewSecret([]byte(password))

	_, err := src.FetchReadings(context.Background(), creds, pm5340TestRequest())
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)
}

func TestPM5340DiscoverMeteringPointsReturnsNilNil(t *testing.T) {
	srv := fake.NewTLSServer(t)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	points, err := src.DiscoverMeteringPoints(context.Background(), pm5340TestCreds(srv))
	require.NoError(t, err)
	require.Nil(t, points)
}

func TestPM5340KindsAndMaxWindow(t *testing.T) {
	srv := fake.NewTLSServer(t)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	require.Equal(t, []model.ReadingKind{model.ReadingKindLoadProfile}, src.Kinds(pm5340TestCreds(srv)))
	require.Equal(t, 7*24*time.Hour, src.MaxWindow(model.ReadingKindLoadProfile))
}

// ---- Defensive/config-error tests (adapter-patterns.md items 9, 12) ----

func TestPM5340RefusesEmptyBaseURL(t *testing.T) {
	srv := fake.NewTLSServer(t)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	creds := pm5340TestCreds(srv)
	creds.BaseURL = ""

	_, err := src.FetchReadings(context.Background(), creds, pm5340TestRequest())
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests(), "a config error must be caught before any network call")

	err = src.Verify(context.Background(), creds)
	require.ErrorIs(t, err, integration.ErrAuth)
}

func TestPM5340RefusesZeroMultiplier(t *testing.T) {
	srv := fake.NewTLSServer(t)
	pool, _ := pm5340TestPool(t, srv)
	src := pm5340.New(pool, pm5340.Options{Clock: clock.NewFake(fixtureNow)})

	req := pm5340TestRequest()
	req.Multiplier = decimal.Zero

	_, err := src.FetchReadings(context.Background(), pm5340TestCreds(srv), req)
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests())
}
