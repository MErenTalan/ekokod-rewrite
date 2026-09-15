package isolar_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// --- shared test plumbing ---------------------------------------------------

// recordingSleep is a fake PoolOptions.Sleep: it records every requested
// delay and returns immediately, so no test in this package ever waits in
// real time. Mirrors httpx_test's own copy (internal/integration/httpx's
// unexported helper is not visible from here) — the adapter template rule
// "no test constructs an *http.Client" still holds; only a Pool/Sleep func
// is built here.
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

// isolarTestPool builds a Pool pinned to srv's own certificate.
func isolarTestPool(t *testing.T, srv *fake.Server) *httpx.Pool {
	t.Helper()
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: (&recordingSleep{}).fn})
	require.NoError(t, err)
	return pool
}

// The fixture credential material below is all FIXTURE-/FX-placeholder
// shaped (fake.TestFixturesAreSanitised's rules): none of it is real.
const (
	fixtureAppKey      = "FIXTURE-APPKEY-a1"
	fixtureSecretKey   = "FIXTURE-SECRETKEY-b2"
	fixtureAppID       = "FIXTURE-APPID-9"
	fixtureAccessToken = "FIXTURE-ACCESSTOKEN-c3"
	fixtureRefreshTok  = "FIXTURE-REFRESHTOKEN-d4"

	// isolarOpToken etc. duplicate isolar's own unexported op* constants:
	// this is an external test package (isolar_test), so it cannot
	// reference them directly, and creds.Endpoints keys must match them
	// byte-for-byte for the Client to find its own templates.
	isolarOpAuthorize                          = "authorize"
	isolarOpToken                              = "token"
	isolarOpRefreshToken                       = "refreshToken"
	isolarOpQueryPowerStationList              = "queryPowerStationList"
	isolarOpGetDeviceListByPsId                = "getDeviceListByPsId"
	isolarOpGetDevicePointMinuteDataList       = "getDevicePointMinuteDataList"
	isolarOpGetPowerStationPointMinuteDataList = "getPowerStationPointMinuteDataList"
	isolarOpGetFaultAlarmInfo                  = "getFaultAlarmInfo"
)

// isolarTestCreds builds Credentials whose Endpoints all point at srv, for
// region EU (cloud id 3, per 06 §6's Regions table).
func isolarTestCreds(srv *fake.Server) integration.Credentials {
	base := srv.URL + "/openapi/apiManage/"
	return integration.Credentials{
		CredentialID: uuid.New(),
		CompanyID:    uuid.New(),
		Provider:     integration.ProviderISolar,
		Subtype:      "EU",
		Region:       "EU",
		Endpoints: map[string]string{
			isolarOpAuthorize:                          "https://web3.isolarcloud.eu",
			isolarOpToken:                              base + "token",
			isolarOpRefreshToken:                       base + "refreshToken",
			isolarOpQueryPowerStationList:              base + "queryPowerStationList",
			isolarOpGetDeviceListByPsId:                base + "getDeviceListByPsId",
			isolarOpGetDevicePointMinuteDataList:       base + "getDevicePointMinuteDataList",
			isolarOpGetPowerStationPointMinuteDataList: base + "getPowerStationPointMinuteDataList",
			isolarOpGetFaultAlarmInfo:                  base + "getFaultAlarmInfo",
		},
		Extra: map[string]integration.Secret{
			"app_key":       integration.NewSecret([]byte(fixtureAppKey)),
			"secret_key":    integration.NewSecret([]byte(fixtureSecretKey)),
			"app_id":        integration.NewSecret([]byte(fixtureAppID)),
			"access_token":  integration.NewSecret([]byte(fixtureAccessToken)),
			"refresh_token": integration.NewSecret([]byte(fixtureRefreshTok)),
		},
	}
}

var (
	fixtureFrom = time.Date(2026, time.January, 1, 0, 0, 0, 0, normalize.Istanbul)
	fixtureTo   = time.Date(2026, time.January, 1, 1, 0, 0, 0, normalize.Istanbul)
)

// --- TestISolarFixtureMatrix ------------------------------------------------

// TestISolarFixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination" (fake.RequiredCases). It is exercised on DeviceMinuteSeries
// for every case except "pagination", which iSolar's minute-series calls
// do not themselves paginate (each call covers exactly the window it is
// given — see series.go); "pagination" instead exercises Devices, whose
// getDeviceListByPsId call follows rowCount across pages (task brief:
// "(plus pagination on Devices)").
func TestISolarFixtureMatrix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes func(t *testing.T) []fake.Route
		call   func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error)
	}{
		{
			name: "success",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_success.json")))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001", "FX1002"}, fixtureFrom, fixtureTo)
				return len(samples), err
			},
		},
		{
			name: "empty",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_empty.json")))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
				require.NoError(t, err)
				require.Empty(t, samples)
				return len(samples), err
			},
		},
		{
			name: "partial",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_partial.json")))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
				require.NoError(t, err)
				require.Len(t, samples, 1, "one bad row must be skipped, not fail the whole call")
				return len(samples), err
			},
		},
		{
			name: "auth_failure",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_auth_failure.json")))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
				return 0, err
			},
		},
		{
			name: "malformed",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_malformed.json")))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
				return 0, err
			},
		},
		{
			name: "rate_limited",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{deviceMinuteRoute(fake.Sequence(
					fake.RateLimited("1"),
					fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_rate_limited.json")),
				))}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
				require.NoError(t, err, "a 429 with Retry-After must be retried in-client, not surfaced")
				require.Len(t, samples, 1)
				return len(samples), err
			},
		},
		{
			name: "pagination",
			routes: func(t *testing.T) []fake.Route {
				return []fake.Route{devicesPagedRoute(t)}
			},
			call: func(t *testing.T, c *isolar.Client, creds integration.Credentials) (int, error) {
				devices, err := c.Devices(context.Background(), creds, "FX3001")
				require.NoError(t, err)
				require.Len(t, devices, 2, "both pages must be followed and merged")
				return len(devices), err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fake.NewTLSServer(t, tc.routes(t)...)
			c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
			creds := isolarTestCreds(srv)

			switch tc.name {
			case "auth_failure":
				_, err := tc.call(t, c, creds)
				require.ErrorIs(t, err, integration.ErrAuth)
			case "malformed":
				_, err := tc.call(t, c, creds)
				require.ErrorIs(t, err, integration.ErrMalformedPayload)
			default:
				_, err := tc.call(t, c, creds)
				require.NoError(t, err)
			}
		})
	}
}

func deviceMinuteRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/getDevicePointMinuteDataList", Respond: respond}
}

// devicesPagedRoute serves isolar_devices_page1.json on the first call to
// getDeviceListByPsId and isolar_devices_page2.json on every call after,
// via fake.Sequence — the two-page fixture the task brief names.
func devicesPagedRoute(t *testing.T) fake.Route {
	return fake.Route{
		Method: http.MethodPost,
		Path:   "/openapi/apiManage/getDeviceListByPsId",
		Respond: fake.Sequence(
			fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_devices_page1.json")),
			fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_devices_page2.json")),
		),
	}
}

// --- named behaviour tests ---------------------------------------------------

// TestIdempotentISolarRefetchYieldsIdenticalSamples: the same fixture served
// twice gives equal results (sorted, decimals compared with Equal).
func TestIdempotentISolarRefetchYieldsIdenticalSamples(t *testing.T) {
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_device_minute.json"))))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	first, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	second, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)

	require.Len(t, first, 2)
	require.Len(t, second, 2)
	sortSamples(first)
	sortSamples(second)
	for i := range first {
		require.True(t, first[i].Ts.Equal(second[i].Ts))
		requireDecimalPtrEqual(t, first[i].ProductionKwh, second[i].ProductionKwh)
		requireDecimalPtrEqual(t, first[i].ActivePowerKw, second[i].ActivePowerKw)
	}
}

func sortSamples(s []isolar.ProductionSample) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Ts.Before(s[j-1].Ts); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func requireDecimalPtrEqual(t *testing.T, a, b *decimal.Decimal) {
	t.Helper()
	require.Equal(t, a == nil, b == nil)
	if a != nil {
		require.True(t, a.Equal(*b))
	}
}

// TestISolarUnitsAreNormalised: 1500 Wh -> 1.5 kWh; 2400 W -> 2.4 kW (task
// brief, verbatim).
func TestISolarUnitsAreNormalised(t *testing.T) {
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_device_minute.json"))))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	var checked bool
	for _, s := range samples {
		if s.Ts.Equal(time.Date(2025, time.December, 31, 21, 5, 0, 0, time.UTC)) {
			require.NotNil(t, s.ProductionKwh)
			require.True(t, decimal.RequireFromString("1.5").Equal(*s.ProductionKwh))
			require.NotNil(t, s.ActivePowerKw)
			require.True(t, decimal.RequireFromString("2.4").Equal(*s.ActivePowerKw))
			// Every template-required field, per adapter-patterns.md item 1.
			require.NotNil(t, s.PSKey)
			require.Equal(t, "FX1001", *s.PSKey)
			require.NotNil(t, s.IrradianceWm2)
			require.NotNil(t, s.AmbientTempC)
			require.NotNil(t, s.ModuleTempC)
			checked = true
		}
	}
	require.True(t, checked, "the 1500Wh/2400W row must be present")
}

// TestISolarHeadersCarrySecretsOnlyInHeaders: the recorded request has
// x-access-key and the bearer header; on failure, err.Error() contains
// neither secret.
func TestISolarHeadersCarrySecretsOnlyInHeaders(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/getDevicePointMinuteDataList",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_auth_failure.json")),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.ErrorIs(t, err, integration.ErrAuth)
	require.NotContains(t, err.Error(), fixtureSecretKey)
	require.NotContains(t, err.Error(), fixtureAccessToken)

	reqs := srv.Requests()
	require.Len(t, reqs, 1)
	require.Equal(t, fixtureSecretKey, reqs[0].Header.Get("x-access-key"))
	require.Equal(t, "Bearer "+fixtureAccessToken, reqs[0].Header.Get("Authorization"))
	// The body carries appkey but never the secret key or bearer token.
	require.Contains(t, string(reqs[0].Body), fixtureAppKey)
	require.NotContains(t, string(reqs[0].Body), fixtureSecretKey)
	require.NotContains(t, string(reqs[0].Body), fixtureAccessToken)
}

// TestISolarConfigErrorsAreErrAuth: a missing endpoint template or a missing
// credential-shaped Extra value is refused before any request is sent, with
// the chosen config-error Kind (ErrAuth — see client.go's configError doc).
func TestISolarConfigErrorsAreErrAuth(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	t.Run("missing endpoint template", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Endpoints, isolarOpGetDevicePointMinuteDataList)
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrAuth)
	})

	t.Run("missing app_key", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Extra, "app_key")
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrAuth)
	})

	t.Run("missing access_token", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Extra, "access_token")
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrAuth)
	})

	require.Empty(t, srv.Requests(), "a config error must never reach the network")
}

// TestISolarPlantsAndFaultsMapEveryField exercises Plants and Faults, whose
// task-brief tests are not individually named (unlike DeviceMinuteSeries's
// fixture matrix). Field mapping is checked here so both calls are still
// proven correct end to end.
func TestISolarPlantsAndFaultsMapEveryField(t *testing.T) {
	plantsBody := []byte(`{"result_code":"1","result_msg":"success","result_data":{"rowCount":1,"pageList":[{"ps_id":"FX3001","ps_name":"Fixture Plant","total_capacity":"220.500"}]}}`)
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/queryPowerStationList", Respond: fake.JSON(http.StatusOK, plantsBody)},
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/getFaultAlarmInfo", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_faults.json"))},
	)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	plants, err := c.Plants(context.Background(), creds)
	require.NoError(t, err)
	require.Len(t, plants, 1)
	require.Equal(t, "FX3001", plants[0].PSID)
	require.Equal(t, "Fixture Plant", plants[0].Name)
	require.NotNil(t, plants[0].InstalledKw)
	require.True(t, decimal.RequireFromString("220.500").Equal(*plants[0].InstalledKw))

	faults, err := c.Faults(context.Background(), creds, fixtureFrom, fixtureTo.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, faults, 1)
	require.Equal(t, "FX_ALARM_0001", faults[0].Ref)
	require.Equal(t, "FX3001", faults[0].PSID)
	require.NotNil(t, faults[0].PSKey)
	require.Equal(t, "FX2001", *faults[0].PSKey)
	require.Equal(t, "W-01", faults[0].Code)
	require.Equal(t, "Fixture Fault", faults[0].Message)
	require.False(t, faults[0].OccurredAt.IsZero())
}

// TestISolarVerifyMapsAuthFailure mirrors the meter adapters'
// Test<P>VerifyMapsAuthFailure.
func TestISolarVerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/queryPowerStationList",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_auth_failure.json")),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	require.ErrorIs(t, c.Verify(context.Background(), isolarTestCreds(srv)), integration.ErrAuth)
}

// TestISolarErrorsCarryNoCredential: the auth-failure fixture's server
// never echoes app_key/secret_key/access_token, and neither does the error
// text — err.Error() is Provider/Op/Kind/HTTPStatus only (integration.Error
// itself proves this generically; this test proves iSolar's own calls
// route through that same type).
func TestISolarErrorsCarryNoCredential(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/queryPowerStationList",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_auth_failure.json")),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	err := c.Verify(context.Background(), isolarTestCreds(srv))
	require.Error(t, err)
	for _, secret := range []string{fixtureAppKey, fixtureSecretKey, fixtureAccessToken, fixtureRefreshTok} {
		require.False(t, strings.Contains(err.Error(), secret), "error text must not contain %q", secret)
	}
}
