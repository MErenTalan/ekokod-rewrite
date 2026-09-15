package osos_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	lock "github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
)

// testToken is the fixture access_token every successful auth route in
// this file returns. It is not a real credential — see credentials_test.go
// / fake.TestFixturesAreSanitised's sanitisation rules, which this string
// (a Go source literal, not a testdata fixture file) is not subject to,
// but it is deliberately shaped like a fixture value anyway.
const testToken = "FIXTURE-ACCESS-TOKEN-0001"

// fixtureNow is the clock every Source in this file is built with.
var fixtureNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

// recordingSleep is a fake httpx.PoolOptions.Sleep: it records every
// requested delay and returns immediately, so the rate-limit fixture case
// never actually waits in real time.
type recordingSleep struct {
	delays []time.Duration
}

func (r *recordingSleep) fn(_ context.Context, d time.Duration) error {
	r.delays = append(r.delays, d)
	return nil
}

// ososTestPool builds an httpx.Pool pinned to srv's own certificate, with a
// recording Sleep and an in-memory Locker so SerializeKey exercises real
// (in-process) serialisation without ever blocking a test in real time.
func ososTestPool(t *testing.T, srv *fake.Server) (*httpx.Pool, *recordingSleep) {
	t.Helper()
	rs := &recordingSleep{}
	pool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: srv.Pins,
		Sleep:       rs.fn,
		Locker:      lock.NewMemory(nil),
	})
	require.NoError(t, err)
	return pool, rs
}

// ososTestCreds builds Credentials whose Endpoints point at srv, using the
// exact placeholder names task-6-brief.md / 06 §2 name for each endpoint.
func ososTestCreds(srv *fake.Server) integration.Credentials {
	return integration.Credentials{
		CredentialID: uuid.New(),
		CompanyID:    uuid.New(),
		Provider:     integration.ProviderOSOS,
		Endpoints: map[string]string{
			"authentication": srv.URL + "/osos/auth?u={username_or_email}&p={secret_password}",
			"analyzers_list": srv.URL + "/osos/analyzers?t={secret_token}",
			"energy_values":  srv.URL + "/osos/energy?t={secret_token}&start={start_date}&end={end_date}&inst={installationNumber}&type={dataType}",
			"hourly_values":  srv.URL + "/osos/hourly?t={secret_token}&month={meter_month}&from={from_date}&inst={installationNumber}",
		},
		Username: "fixture-user",
		Secret:   integration.NewSecret([]byte("fixture-password-0001")),
	}
}

// ososTestRequest builds a FetchRequest for installation FX0001 over
// [from, to), multiplier 1.
func ososTestRequest(kind model.ReadingKind, from, to time.Time) integration.FetchRequest {
	return integration.FetchRequest{
		Point:      integration.MeteringPoint{InstallationNumber: "FX0001"},
		AnalyzerID: uuid.New(),
		Multiplier: decimal.NewFromInt(1),
		Kind:       kind,
		From:       from,
		To:         to,
	}
}

func authRoute(token string) fake.Route {
	return fake.Route{
		Method:  http.MethodPost,
		Path:    "/osos/auth",
		Respond: fake.JSON(http.StatusOK, []byte(`{"access_token":"`+token+`"}`)),
	}
}

func energyRoute(body []byte) fake.Route {
	return fake.Route{Method: http.MethodGet, Path: "/osos/energy", Respond: fake.JSON(http.StatusOK, body)}
}

func hourlyRoute(body []byte) fake.Route {
	return fake.Route{Method: http.MethodGet, Path: "/osos/hourly", Respond: fake.JSON(http.StatusOK, body)}
}

// Test<P>FixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination". The subtest names are exactly fake.RequiredCases; Task 17's
// guard checks them.
func TestOSOSFixtureMatrix(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	paginationTo := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC) // 45-day window: 30 + 15 days

	for _, tc := range []struct {
		name    string
		routes  func(t *testing.T) []fake.Route
		wantErr error
		check   func(t *testing.T, res integration.FetchResult)
	}{
		{
			name: "success",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_success.json"))}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 2)
				require.Nil(t, res.NextCursor)
				require.Empty(t, res.Warnings)
				require.Empty(t, res.HourlyValues)
			},
		},
		{
			name: "empty",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_empty.json"))}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Empty(t, res.Readings)
				require.Nil(t, res.NextCursor)
			},
		},
		{
			name: "partial",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_partial.json"))}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 1)
				var warnCount int
				for _, w := range res.Warnings {
					if w.Code == integration.WarnUnparseableRow {
						warnCount++
					}
				}
				require.Equal(t, 2, warnCount)
			},
		},
		{
			name: "auth_failure",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					{Method: http.MethodPost, Path: "/osos/auth", Respond: fake.JSON(http.StatusUnauthorized, fake.Fixture(t, "osos", "osos_auth_failure.json"))},
				}
			},
			wantErr: integration.ErrAuth,
		},
		{
			name: "malformed",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_malformed.json"))}
			},
			wantErr: integration.ErrMalformedPayload,
		},
		{
			name: "rate_limited",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{
					authRoute(testToken),
					{
						Method: http.MethodGet, Path: "/osos/energy",
						Respond: fake.Sequence(fake.RateLimited("1"), fake.JSON(http.StatusOK, fake.Fixture(t, "osos", "osos_rate_limited.json"))),
					},
				}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 1)
			},
		},
		{
			name: "pagination",
			routes: func(t *testing.T) []fake.Route {
				var pages struct {
					Page1 json.RawMessage `json:"page1"`
					Page2 json.RawMessage `json:"page2"`
				}
				require.NoError(t, json.Unmarshal(fake.Fixture(t, "osos", "osos_pagination.json"), &pages))
				return []fake.Route{
					authRoute(testToken),
					{
						Method: http.MethodGet, Path: "/osos/energy",
						Respond: fake.Sequence(fake.JSON(http.StatusOK, pages.Page1), fake.JSON(http.StatusOK, pages.Page2)),
					},
				}
			},
			check: func(t *testing.T, res integration.FetchResult) {
				require.Len(t, res.Readings, 2)
				require.True(t, res.Readings[0].Ts.Before(res.Readings[1].Ts))
				require.Nil(t, res.NextCursor)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			pool, rs := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})
			creds := ososTestCreds(srv)

			reqTo := to
			if tc.name == "pagination" {
				reqTo = paginationTo
			}
			req := ososTestRequest(model.ReadingKindDaily, from, reqTo)

			res, err := src.FetchReadings(context.Background(), creds, req)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.name == "rate_limited" {
				require.Contains(t, rs.delays, time.Second)
			}
			tc.check(t, res)
		})
	}
}

// TestIdempotentOSOSRefetchYieldsIdenticalReadings: the same fixture served
// twice gives require.Equal result slices.
func TestIdempotentOSOSRefetchYieldsIdenticalReadings(t *testing.T) {
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_success.json")))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})
	creds := ososTestCreds(srv)
	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

	res1, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)
	res2, err := src.FetchReadings(context.Background(), creds, req)
	require.NoError(t, err)

	require.Equal(t, len(res1.Readings), len(res2.Readings))
	require.Equal(t, res1.Readings, res2.Readings)
}

// TestOSOSVerifyMapsAuthFailure: Verify surfaces ErrAuth for a rejected
// credential.
func TestOSOSVerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/osos/auth",
		Respond: fake.JSON(http.StatusUnauthorized, fake.Fixture(t, "osos", "osos_auth_failure.json")),
	})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	err := src.Verify(context.Background(), ososTestCreds(srv))
	require.ErrorIs(t, err, integration.ErrAuth)
}

// TestOSOSVerifyMapsEmptyAccessTokenToAuthFailure: 200 with an empty
// access_token is also ErrAuth (06 §2 "Authenticate").
func TestOSOSVerifyMapsEmptyAccessTokenToAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/osos/auth",
		Respond: fake.JSON(http.StatusOK, []byte(`{"access_token":""}`)),
	})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	err := src.Verify(context.Background(), ososTestCreds(srv))
	require.ErrorIs(t, err, integration.ErrAuth)
}

// TestOSOSDiscoverMapsEveryField asserts every row of 06 §2's mapping table
// by name, from osos_discover.json.
func TestOSOSDiscoverMapsEveryField(t *testing.T) {
	srv := fake.NewTLSServer(t, authRoute(testToken), fake.Route{
		Method: http.MethodGet, Path: "/osos/analyzers",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "osos", "osos_discover.json")),
	})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	points, err := src.DiscoverMeteringPoints(context.Background(), ososTestCreds(srv))
	require.NoError(t, err)
	require.Len(t, points, 1)
	pt := points[0]

	require.Equal(t, "FX0001", pt.InstallationNumber, "instalationNumber -> InstallationNumber")
	require.Equal(t, "Fixture Customer One", *pt.CustomerName, "customerName -> CustomerName")
	require.Equal(t, "Fixture Address One", *pt.Address, "customerAdress -> Address")
	require.Equal(t, "Fixture Il", *pt.Province, "il -> Province")
	require.Equal(t, "Fixture Ilce", *pt.District, "ilce -> District")
	require.Equal(t, "Fixture Koy Mahallesi", *pt.Neighbourhood, "koyMahallesi -> Neighbourhood")
	require.Equal(t, "Fixture Cadde Sokagi", *pt.Street, "caddesiSokagi -> Street")
	require.Equal(t, "Single-time", *pt.TariffType, "tarifeTipi -> TariffType")
	require.Equal(t, "Mesken", *pt.TariffKind, "tarifeTuru -> TariffKind")
	require.Equal(t, "Residential", *pt.InstallationKind, "tesisatTurTanim -> InstallationKind")
	require.True(t, decimal.RequireFromString("150.5").Equal(*pt.InstalledPowerKw), "kuruluGucu -> InstalledPowerKw")
	require.True(t, decimal.RequireFromString("39.925018").Equal(*pt.Latitude), "koordinatX -> Latitude")
	require.True(t, decimal.RequireFromString("32.836956").Equal(*pt.Longitude), "koordinatY -> Longitude")
	require.Equal(t, "FX0002", *pt.MeterNumber, "meterNumber -> MeterNumber")
	require.Equal(t, "Fixture Meter Model", *pt.MeterModel, "meterModel -> MeterModel")
	require.True(t, decimal.RequireFromString("1").Equal(*pt.MeterMultiplier), "meterMultiplier -> MeterMultiplier")
	require.Equal(t, "FX0003", *pt.CounterpartyNo, "muhatapNo -> CounterpartyNo")
	require.Equal(t, "Fixture Metering Point Name", *pt.MeteringPointName, "sayimNokTanim -> MeteringPointName")
}

// TestOSOSNeverReturnsReadingsOutsideWindow: a row inside [From, To) and a
// row outside it, only the inside one comes back, silently (no warning).
func TestOSOSNeverReturnsReadingsOutsideWindow(t *testing.T) {
	body := []byte(`{"energy":[
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"},
		{"meter_date":"01/08/2026 20:00:00","meter_serial_no":"SN0001","t_top_kWh":"2,000"}
	]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.Empty(t, res.Warnings, "a row outside the window is dropped silently, not warned")
	require.True(t, decimal.RequireFromString("1").Equal(*res.Readings[0].ActiveImport))
}

// TestOSOSDateIsIstanbulLocal pins the exact task-6-brief.md example:
// meter_date "01/09/2026 00:15:00" -> 2026-08-31T21:15:00Z.
func TestOSOSDateIsIstanbulLocal(t *testing.T) {
	body := []byte(`{"energy":[{"meter_date":"01/09/2026 00:15:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"}]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.Equal(t, time.Date(2026, 8, 31, 21, 15, 0, 0, time.UTC), res.Readings[0].Ts.UTC())
}

// TestOSOSThousandsSeparatorsParseExactly pins the exact task-6-brief.md
// example: "12.345,678" -> 12345.678, x multiplier "40" -> 493827.12.
func TestOSOSThousandsSeparatorsParseExactly(t *testing.T) {
	body := []byte(`{"energy":[{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"12.345,678"}]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	req.Multiplier = decimal.RequireFromString("40")

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.True(t, decimal.RequireFromString("493827.12").Equal(*res.Readings[0].ActiveImport),
		"got %s", res.Readings[0].ActiveImport.String())
}

// TestOSOSRequestsDataTypePerKind: the recorded energy_values query carries
// dataType=1|2|3 for load_profile|daily|reset respectively.
func TestOSOSRequestsDataTypePerKind(t *testing.T) {
	for _, tc := range []struct {
		kind model.ReadingKind
		want string
	}{
		{model.ReadingKindLoadProfile, "type=1"},
		{model.ReadingKindDaily, "type=2"},
		{model.ReadingKindReset, "type=3"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			routes := []fake.Route{authRoute(testToken), energyRoute(fake.Fixture(t, "osos", "osos_empty.json"))}
			if tc.kind == model.ReadingKindLoadProfile {
				routes = append(routes, hourlyRoute([]byte(`{"items":{}}`)))
			}
			srv := fake.NewTLSServer(t, routes...)
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

			req := ososTestRequest(tc.kind,
				time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

			_, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
			require.NoError(t, err)

			var (
				found    bool
				rawQuery string
			)
			for _, rr := range srv.Requests() {
				if rr.Method == http.MethodGet && rr.Path == "/osos/energy" {
					found, rawQuery = true, rr.RawQuery
				}
			}
			require.True(t, found, "no energy_values request recorded")
			require.Contains(t, rawQuery, tc.want)
		})
	}
}

// TestOSOSHourlyValuesNeverMixIntoReadings: a fixture with both series —
// Readings contains only energy rows, HourlyValues only hourly rows.
func TestOSOSHourlyValuesNeverMixIntoReadings(t *testing.T) {
	srv := fake.NewTLSServer(t,
		authRoute(testToken),
		energyRoute(fake.Fixture(t, "osos", "osos_success.json")),
		hourlyRoute(fake.Fixture(t, "osos", "osos_hourly_values.json")),
	)
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindLoadProfile,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)

	require.Len(t, res.Readings, 2, "Readings must contain only the energy rows")
	require.Len(t, res.HourlyValues, 2, "HourlyValues must contain only the hourly rows")
	for _, hv := range res.HourlyValues {
		require.NotNil(t, hv.ActiveConsumption)
	}
}

// TestOSOSErrorsCarryNoCredential: the auth-failure fixture's server
// echoes the submitted password; err.Error() must not contain it.
func TestOSOSErrorsCarryNoCredential(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/osos/auth",
		Respond: func(w http.ResponseWriter, r *http.Request) {
			pw := r.URL.Query().Get("p")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"access_token":"","error":"invalid credentials: ` + pw + `"}`))
		},
	})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	creds := ososTestCreds(srv)
	err := src.Verify(context.Background(), creds)
	require.Error(t, err)
	require.NotContains(t, err.Error(), creds.Secret.Reveal())
	require.NotContains(t, err.Error(), "fixture-password-0001")
}
