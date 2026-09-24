package osos_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
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

				// I2: assert every template-required field on one reading, not
				// just how many came back.
				r := res.Readings[0]
				require.Equal(t, model.ReadingKindDaily, r.Kind)
				require.NotEqual(t, uuid.UUID{}, r.AnalyzerID)
				require.Equal(t, model.IntegrationProviderOSOS, r.SourceProvider)
				require.True(t, decimal.NewFromInt(1).Equal(r.MultiplierApplied))
				require.NotNil(t, r.MeterSerial)
				require.Equal(t, "SN0001", *r.MeterSerial)
				require.False(t, r.Ts.IsZero())
				require.NotNil(t, r.ActiveImport)
				require.True(t, decimal.RequireFromString("1000.5").Equal(*r.ActiveImport))
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

// ---------------------------------------------------------------------
// Fix round 1 (task-6-fix1-findings.md): I1-I8 and folded minors.
// ---------------------------------------------------------------------

// TestOSOSPerRowTypeMismatchWarnsWithoutFailingTheWholePage proves I7
// end-to-end through FetchReadings (not just mapEnergyRow in isolation): a
// row whose register is a JSON number (not the string the wire type
// expects) produces one WarnUnparseableRow naming that field and row
// index — the surrounding, well-shaped rows still come back, and the whole
// energy_values page is never failed as ErrMalformedPayload just because
// one row doesn't decode.
func TestOSOSPerRowTypeMismatchWarnsWithoutFailingTheWholePage(t *testing.T) {
	body := []byte(`{"energy":[
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"},
		{"meter_date":"01/08/2026 10:15:00","meter_serial_no":"SN0001","t_top_kWh":123},
		{"meter_date":"01/08/2026 10:30:00","meter_serial_no":"SN0001","t_top_kWh":"3,000"}
	]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 2, "the two well-shaped rows must still come back")

	var found bool
	for _, w := range res.Warnings {
		if w.Code == integration.WarnUnparseableRow && strings.Contains(w.Detail, "t_top_kWh") && strings.Contains(w.Detail, "row 1") {
			found = true
		}
	}
	require.True(t, found, "expected a WarnUnparseableRow naming t_top_kWh at row 1 (window included), got %+v", res.Warnings)
}

// TestOSOSMalformedTopLevelEnergyShapes: I6, energy_values. {} (no "energy"
// key), an explicit top-level null, {"energy":null} and an error envelope
// like {"message":"invalid token"} are all ErrMalformedPayload — none is a
// silent empty success. TestOSOSFixtureMatrix/empty already covers the
// "energy":[] genuine-empty-result case.
func TestOSOSMalformedTopLevelEnergyShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing_key", `{}`},
		{"top_level_null", `null`},
		{"key_null", `{"energy":null}`},
		{"error_envelope", `{"message":"invalid token"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute([]byte(tc.body)))
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})
			req := ososTestRequest(model.ReadingKindDaily,
				time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
			_, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
			require.ErrorIs(t, err, integration.ErrMalformedPayload)
		})
	}
}

// TestOSOSMalformedTopLevelDiscoverShapes is
// TestOSOSMalformedTopLevelEnergyShapes' analyzers_list/instalation_list
// counterpart.
//
//nolint:misspell // OSOS's own field spelling ("instalation_list"), not an English typo
func TestOSOSMalformedTopLevelDiscoverShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing_key", `{}`},
		{"top_level_null", `null`},
		{"key_null", `{"instalation_list":null}`}, //nolint:misspell // OSOS's own field spelling
		{"error_envelope", `{"message":"invalid token"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, authRoute(testToken), fake.Route{
				Method: http.MethodGet, Path: "/osos/analyzers",
				Respond: fake.JSON(http.StatusOK, []byte(tc.body)),
			})
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})
			_, err := src.DiscoverMeteringPoints(context.Background(), ososTestCreds(srv))
			require.ErrorIs(t, err, integration.ErrMalformedPayload)
		})
	}
}

// TestOSOSMalformedTopLevelHourlyShapes is the hourly_values/items
// counterpart. {"items":{}} (a genuine empty result) is proven elsewhere
// (TestOSOSRequestsDataTypePerKind's load_profile subtest uses it and
// expects success).
func TestOSOSMalformedTopLevelHourlyShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"missing_key", `{}`},
		{"top_level_null", `null`},
		{"key_null", `{"items":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t,
				authRoute(testToken),
				energyRoute(fake.Fixture(t, "osos", "osos_empty.json")),
				hourlyRoute([]byte(tc.body)),
			)
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})
			req := ososTestRequest(model.ReadingKindLoadProfile,
				time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
			_, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
			require.ErrorIs(t, err, integration.ErrMalformedPayload)
		})
	}
}

// TestOSOSBoundaryRowsAtFromMinusOneSecondFromAndTo proves I4: the
// half-open window [From, To) keeps a row at exactly From, and drops rows
// at From-1s and at exactly To.
func TestOSOSBoundaryRowsAtFromMinusOneSecondFromAndTo(t *testing.T) {
	from := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)

	row := func(ts time.Time, value string) string {
		return `{"meter_date":"` + normalize.FormatOSOSDate(ts) + `","meter_serial_no":"SN0001","t_top_kWh":"` + value + `"}`
	}
	body := []byte(`{"energy":[` +
		row(from.Add(-time.Second), "1,000") + `,` +
		row(from, "2,000") + `,` +
		row(to, "3,000") +
		`]}`)

	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily, from, to)
	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "only the row at exactly From must survive the half-open [From, To) window")
	require.True(t, res.Readings[0].Ts.Equal(from), "got %s", res.Readings[0].Ts)
	require.True(t, decimal.RequireFromString("2").Equal(*res.Readings[0].ActiveImport))
}

// TestOSOSFortyFiveDayWindowMakesExactlyTwoRequestsWithExpectedDates proves
// I5: a 45-day window makes exactly two energy_values requests, with the
// exact start/end params windowChunks computes (30 + 15 days). Mutation d
// (task-6-fix1-findings.md): MaxWindow mutated to 15 days would make three
// requests, which this test's exact count and per-request date assertions
// catch — unlike the pre-fix pagination check, which only asserted the
// merged reading count and NextCursor.
func TestOSOSFortyFiveDayWindowMakesExactlyTwoRequestsWithExpectedDates(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC) // 45 days
	mid := from.Add(30 * 24 * time.Hour)               // Aug 31: the chunk boundary

	page1 := []byte(`{"energy":[{"meter_date":"` + normalize.FormatOSOSDate(from) + `","meter_serial_no":"SN0001","t_top_kWh":"1,000"}]}`)
	page2 := []byte(`{"energy":[{"meter_date":"` + normalize.FormatOSOSDate(mid) + `","meter_serial_no":"SN0001","t_top_kWh":"2,000"}]}`)

	srv := fake.NewTLSServer(t,
		authRoute(testToken),
		fake.Route{
			Method: http.MethodGet, Path: "/osos/energy",
			Respond: fake.Sequence(fake.JSON(http.StatusOK, page1), fake.JSON(http.StatusOK, page2)),
		},
	)
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily, from, to)
	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Nil(t, res.NextCursor)

	var energyReqs []fake.RecordedRequest
	for _, rr := range srv.Requests() {
		if rr.Method == http.MethodGet && rr.Path == "/osos/energy" {
			energyReqs = append(energyReqs, rr)
		}
	}
	require.Len(t, energyReqs, 2, "a 45-day window with MaxWindow=30d must make exactly two energy_values requests")

	q0, err := url.ParseQuery(energyReqs[0].RawQuery)
	require.NoError(t, err)
	require.Equal(t, normalize.FormatOSOSDate(from), q0.Get("start"))
	require.Equal(t, normalize.FormatOSOSDate(mid), q0.Get("end"))

	q1, err := url.ParseQuery(energyReqs[1].RawQuery)
	require.NoError(t, err)
	require.Equal(t, normalize.FormatOSOSDate(mid), q1.Get("start"))
	require.Equal(t, normalize.FormatOSOSDate(to), q1.Get("end"))
}

// TestOSOSPageBudgetTruncationStillFetchesHourlyForCoveredRange proves I8 /
// Ruling R34: PageBudget:1 over a 45-day load_profile window covers only
// the first (30-day, Aug 1-31) energy_values chunk, NextCursor is the end
// of that chunk, and hourly_values is still fetched — for August only, not
// skipped outright and not reaching into September.
func TestOSOSPageBudgetTruncationStillFetchesHourlyForCoveredRange(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC) // 45 days
	mid := from.Add(30 * 24 * time.Hour)               // Aug 31

	page1 := []byte(`{"energy":[{"meter_date":"` + normalize.FormatOSOSDate(from) + `","meter_serial_no":"SN0001","t_top_kWh":"1,000"}]}`)
	hourlyAug := []byte(`{"items":{"FX0001":[{"valueList":[
		{"meter_date":"` + normalize.FormatOSOSDate(from.Add(time.Hour)) + `","activeConsumption":"1,000","activeGeneration":"0"}
	]}]}}`)

	srv := fake.NewTLSServer(t,
		authRoute(testToken),
		energyRoute(page1), // PageBudget:1 -> exactly one energy_values call
		hourlyRoute(hourlyAug),
	)
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow), PageBudget: 1})

	req := ososTestRequest(model.ReadingKindLoadProfile, from, to)
	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.NotNil(t, res.NextCursor)
	require.True(t, res.NextCursor.Equal(mid), "NextCursor must be the end of the last fully covered chunk (Aug 31), got %s", res.NextCursor)

	require.Len(t, res.HourlyValues, 1, "hourly must be fetched for the covered [From, NextCursor) range, not skipped because the call was budget-truncated")

	var hourlyReqs []fake.RecordedRequest
	for _, rr := range srv.Requests() {
		if rr.Method == http.MethodGet && rr.Path == "/osos/hourly" {
			hourlyReqs = append(hourlyReqs, rr)
		}
	}
	require.Len(t, hourlyReqs, 1, "only August intersects the covered [From, NextCursor) range — September must not be touched this call")
	q, err := url.ParseQuery(hourlyReqs[0].RawQuery)
	require.NoError(t, err)
	require.Equal(t, "2026-08", q.Get("month"))
}

// TestOSOSConflictingDuplicateTimestampsWarn: adapter-patterns.md item 13.
// Two rows sharing a Ts but disagreeing on a register value must warn, not
// silently drop one.
func TestOSOSConflictingDuplicateTimestampsWarn(t *testing.T) {
	body := []byte(`{"energy":[
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"},
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"2,000"}
	]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1, "only one reading survives per timestamp")

	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w.Detail, "conflicting duplicate") {
			found = true
		}
	}
	require.True(t, found, "conflicting duplicate readings at the same Ts must warn, not silently drop; got %+v", res.Warnings)
}

// TestOSOSIdenticalDuplicateTimestampsDoNotWarn is
// TestOSOSConflictingDuplicateTimestampsWarn's negative: two rows with the
// SAME values at the same Ts (routine chunk-boundary overlap, R20) must not
// warn.
func TestOSOSIdenticalDuplicateTimestampsDoNotWarn(t *testing.T) {
	body := []byte(`{"energy":[
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"},
		{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":"1,000"}
	]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), energyRoute(body))
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	req := ososTestRequest(model.ReadingKindDaily,
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))

	res, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.Empty(t, res.Warnings)
}

// TestOSOSMissingEndpointKeyIsErrConfig: task-6-fix1-findings.md folded
// minor, reclassified by R48/I5 — a missing endpoint template key is
// *integration.Error{Kind: ErrConfig} (see missingEndpointErr's doc
// comment in source.go), never ErrMalformedPayload and never ErrAuth, for
// every one of OSOS's four endpoints.
func TestOSOSMissingEndpointKeyIsErrConfig(t *testing.T) {
	for _, key := range []string{"authentication", "analyzers_list", "energy_values", "hourly_values"} {
		t.Run(key, func(t *testing.T) {
			srv := fake.NewTLSServer(t,
				authRoute(testToken),
				energyRoute(fake.Fixture(t, "osos", "osos_empty.json")),
				hourlyRoute([]byte(`{"items":{}}`)),
			)
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

			creds := ososTestCreds(srv)
			delete(creds.Endpoints, key)

			var err error
			switch key {
			case "authentication":
				err = src.Verify(context.Background(), creds)
			case "analyzers_list":
				_, err = src.DiscoverMeteringPoints(context.Background(), creds)
			default:
				req := ososTestRequest(model.ReadingKindLoadProfile,
					time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
				_, err = src.FetchReadings(context.Background(), creds, req)
			}
			require.ErrorIs(t, err, integration.ErrConfig)
			require.NotErrorIs(t, err, integration.ErrAuth)
		})
	}
}

// TestOSOSZeroMultiplierOrEmptyInstallationErrorsBeforeAnyCall:
// adapter-patterns.md item 12.
func TestOSOSZeroMultiplierOrEmptyInstallationErrorsBeforeAnyCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  func(integration.FetchRequest) integration.FetchRequest
	}{
		{"zero_multiplier", func(r integration.FetchRequest) integration.FetchRequest {
			r.Multiplier = decimal.Zero
			return r
		}},
		{"empty_installation_number", func(r integration.FetchRequest) integration.FetchRequest {
			r.Point.InstallationNumber = ""
			return r
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t) // must never be called
			pool, _ := ososTestPool(t, srv)
			src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

			req := tc.req(ososTestRequest(model.ReadingKindDaily,
				time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
				time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)))

			_, err := src.FetchReadings(context.Background(), ososTestCreds(srv), req)
			require.Error(t, err)
			require.Empty(t, srv.Requests(), "must fail before any network call")
		})
	}
}

// TestOSOSDiscoverSkipsRowsWithEmptyInstallationNumber: task-6-fix1-findings.md
// folded minor — a row with no instalationNumber can never become an
// identifiable MeteringPoint and is skipped, not turned into a
// malformed-payload failure for every other installation in the response.
func TestOSOSDiscoverSkipsRowsWithEmptyInstallationNumber(t *testing.T) {
	//nolint:misspell // OSOS's own field spelling ("instalation_list"), not an English typo
	body := []byte(`{"instalation_list":[
		{"instalationNumber":"","customerName":"Fixture Skip Me"},
		{"instalationNumber":"FX0001","customerName":"Fixture Keep Me"}
	]}`)
	srv := fake.NewTLSServer(t, authRoute(testToken), fake.Route{
		Method: http.MethodGet, Path: "/osos/analyzers",
		Respond: fake.JSON(http.StatusOK, body),
	})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	points, err := src.DiscoverMeteringPoints(context.Background(), ososTestCreds(srv))
	require.NoError(t, err)
	require.Len(t, points, 1)
	require.Equal(t, "FX0001", points[0].InstallationNumber)
}

// TestOSOSAuthenticationNeverRetries is adapter review pattern 10 (ruling
// R32): the login POST is marked NoRetry, so a retryable-classified failure
// (here, 500 -> ErrUpstreamUnavailable, which IS retryable in general)
// still produces exactly one HTTP attempt. Removing NoRetry: true from
// authenticate's httpx.Request makes this test FAIL (calls == 3, the
// Client's default MaxAttempts).
func TestOSOSAuthenticationNeverRetries(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/osos/auth", Respond: func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}})
	pool, _ := ososTestPool(t, srv)
	src := osos.New(pool, osos.Options{Clock: clock.NewFake(fixtureNow)})

	err := src.Verify(context.Background(), ososTestCreds(srv))
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, calls, "R32: a non-idempotent login POST must never be retried in-client")
}
