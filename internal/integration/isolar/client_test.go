package isolar_test

import (
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
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

// isolarTestPoolWithLocker is isolarTestPool plus a Locker — used only by
// TestISolarSerializeKeyCoversAuthAndData (I6).
func isolarTestPoolWithLocker(t *testing.T, srv *fake.Server, locker lock.Locker) *httpx.Pool {
	t.Helper()
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: (&recordingSleep{}).fn, Locker: locker})
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

	// isolarEndpointGateway etc. and isolarOpToken etc. duplicate isolar's
	// own unexported endpoint*/op* constants: this is an external test
	// package (isolar_test), so it cannot reference them directly, and
	// creds.Endpoints keys must match them byte-for-byte — R40: these are
	// EXACTLY internal/seed/data/integration_definitions.json's isolar row
	// keys — for the Client to find its own templates.
	isolarEndpointGateway         = "gateway"
	isolarEndpointAuthorizeOrigin = "authorize_origin"
	isolarEndpointCloudID         = "cloud_id"

	isolarOpToken                              = "token"
	isolarOpRefreshToken                       = "refresh_token"
	isolarOpQueryPowerStationList              = "query_power_station_list"
	isolarOpGetDeviceListByPsID                = "get_device_list_by_ps_id"
	isolarOpGetDevicePointMinuteDataList       = "get_device_point_minute_data_list"
	isolarOpGetPowerStationPointMinuteDataList = "get_power_station_point_minute_data_list"
	isolarOpGetFaultAlarmInfo                  = "get_fault_alarm_info"
)

// isolarTestCreds builds Credentials whose Endpoints mirror
// internal/seed/data/integration_definitions.json's isolar/EU row exactly
// (R40): gateway is srv's own URL, every op key names its REAL relative
// path (token/refreshToken under /openapi/apiManage/, everything else
// under /openapi/platform/), and cloud_id/authorize_origin are the seed's
// own EU values.
func isolarTestCreds(srv *fake.Server) integration.Credentials {
	return integration.Credentials{
		CredentialID: uuid.New(),
		CompanyID:    uuid.New(),
		Provider:     integration.ProviderISolar,
		Subtype:      "EU",
		Region:       "EU",
		Endpoints: map[string]string{
			isolarEndpointGateway:                      srv.URL,
			isolarEndpointAuthorizeOrigin:              "https://web3.isolarcloud.eu",
			isolarEndpointCloudID:                      "3",
			isolarOpToken:                              "/openapi/apiManage/token",
			isolarOpRefreshToken:                       "/openapi/apiManage/refreshToken",
			isolarOpQueryPowerStationList:              "/openapi/platform/queryPowerStationList",
			isolarOpGetDeviceListByPsID:                "/openapi/platform/getDeviceListByPsId",
			isolarOpGetDevicePointMinuteDataList:       "/openapi/platform/getDevicePointMinuteDataList",
			isolarOpGetPowerStationPointMinuteDataList: "/openapi/platform/getPowerStationPointMinuteDataList",
			isolarOpGetFaultAlarmInfo:                  "/openapi/platform/getFaultAlarmInfo",
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

// TestISolarEndpointKeysMatchSeedDefinition is R40's anti-drift proof (I1):
// it reads internal/seed/data/integration_definitions.json's isolar/EU row
// directly (the actual embedded seed data, not a copy) and asserts every
// key this package's op/endpoint constants use (duplicated above as
// isolarOp*/isolarEndpoint* — see their doc comment) is present in it. A
// key renamed in the seed WITHOUT this test package's constants being
// updated to match fails here, not silently in production. MUTATION PROOF
// (task's fix-round-1 requirement): renaming one seed-derived fixture key
// (e.g. isolarOpGetFaultAlarmInfo above to "get_fault_alarm_infoX") makes
// this test fail with "seed isolar/EU endpoints must carry key
// \"get_fault_alarm_infoX\"" — see task-13-report.md's Fix round 1 section
// for the observed failure output.
func TestISolarEndpointKeysMatchSeedDefinition(t *testing.T) {
	defs, err := seed.IntegrationDefinitions()
	require.NoError(t, err)

	var euEndpoints json.RawMessage
	for _, d := range defs {
		if d.Provider == model.IntegrationProviderISolar && d.Subtype == "EU" {
			euEndpoints = d.Endpoints
		}
	}
	require.NotNil(t, euEndpoints, "seed must carry an isolar/EU integration_definitions row")

	var endpoints map[string]string
	require.NoError(t, json.Unmarshal(euEndpoints, &endpoints))

	for _, key := range []string{
		isolarEndpointGateway, isolarEndpointAuthorizeOrigin, isolarEndpointCloudID,
		isolarOpToken, isolarOpRefreshToken, isolarOpQueryPowerStationList,
		isolarOpGetDeviceListByPsID, isolarOpGetDevicePointMinuteDataList,
		isolarOpGetPowerStationPointMinuteDataList, isolarOpGetFaultAlarmInfo,
	} {
		_, ok := endpoints[key]
		require.True(t, ok, "seed isolar/EU endpoints must carry key %q — this package's op constants must match the seed verbatim (R40)", key)
	}
}

// --- TestISolarFixtureMatrix ------------------------------------------------

// TestISolarFixtureMatrix is the F2 acceptance criterion "every adapter has
// recorded fixtures covering success, empty result, partial data,
// authentication failure, malformed payload, rate limiting, and
// pagination" (fake.RequiredCases). It is exercised on DeviceMinuteSeries
// for every case except "pagination" (iSolar's minute-series calls do not
// themselves paginate — R42 splits by TIME, not by page — see series.go);
// "pagination" instead exercises Devices, whose getDeviceListByPsId call
// follows rowCount across pages (task brief: "(plus pagination on
// Devices)").
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
				// I1: the fixture-matrix guard requires this case's own
				// table entry to reference isolar_pagination.json directly
				// (not merely through a helper's own body), so the file's
				// name is read here and passed in.
				return []fake.Route{devicesPagedRoute(t, fake.Fixture(t, "isolar", "isolar_pagination.json"))}
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

			if tc.name == "pagination" {
				// I7: pagination must be proven by counting recorded
				// requests and asserting their OWN page params, not merely
				// that the merged result has the right length (which a
				// hard-coded page=1 sent twice could also produce if the
				// fake server ignored the param — fake.Sequence instead
				// serves page1 then page2 unconditionally, so this only
				// proves real progression if the CLIENT actually sent
				// page=1 then page=2).
				reqs := srv.Requests()
				require.Len(t, reqs, 2)
				var body1, body2 map[string]any
				require.NoError(t, json.Unmarshal(reqs[0].Body, &body1))
				require.NoError(t, json.Unmarshal(reqs[1].Body, &body2))
				require.EqualValues(t, 1, body1["page"], "first request must carry page=1")
				require.EqualValues(t, 2, body2["page"], "second request must carry page=2, not another page=1")
			}
		})
	}
}

func deviceMinuteRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getDevicePointMinuteDataList", Respond: respond}
}

// devicesPagedRoute serves fixture's "page1" object on the first call to
// getDeviceListByPsId and its "page2" object on every call after, via
// fake.Sequence — fixture is isolar_pagination.json, read by the caller so
// the fixture-matrix "pagination" case's own table entry names it directly.
func devicesPagedRoute(t *testing.T, fixture []byte) fake.Route {
	var pages struct {
		Page1 json.RawMessage `json:"page1"`
		Page2 json.RawMessage `json:"page2"`
	}
	require.NoError(t, json.Unmarshal(fixture, &pages))
	return fake.Route{
		Method: http.MethodPost,
		Path:   "/openapi/platform/getDeviceListByPsId",
		Respond: fake.Sequence(
			fake.JSON(http.StatusOK, pages.Page1),
			fake.JSON(http.StatusOK, pages.Page2),
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

// TestISolarPointsNilNeverZero: adapter-patterns.md item 3 / fix-round-1 I5
// — null, "" and "-" all parse to nil, never a fabricated zero; a genuine
// "0" stays a real zero decimal, distinguishable from nil. MUTATION PROOF
// (task's fix-round-1 requirement): changing normalize.OptionalNumber's
// null-handling to return &zero instead of nil makes this test's nil
// assertions fail — see task-13-report.md's Fix round 1 section for the
// observed failure output.
func TestISolarPointsNilNeverZero(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"FX1001":[
		{"time_stamp":"20260101000500","p1":null,"p24":"","p2001":"-","p2009":"0","p2010":"1.5"}
	]}}`)
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, body)))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	s := samples[0]
	require.Nil(t, s.ProductionKwh, "null p1 must be nil, never a fabricated zero")
	require.Nil(t, s.ActivePowerKw, "\"\" p24 must be nil")
	require.Nil(t, s.IrradianceWm2, "\"-\" p2001 must be nil")
	require.NotNil(t, s.AmbientTempC, "a genuine \"0\" p2009 must stay a real zero, not nil")
	require.True(t, decimal.NewFromInt(0).Equal(*s.AmbientTempC))
	require.NotNil(t, s.ModuleTempC)
	require.True(t, decimal.RequireFromString("1.5").Equal(*s.ModuleTempC))
}

// TestISolarHeadersCarrySecretsOnlyInHeaders: the recorded request has
// x-access-key and the bearer header; on failure, err.Error() contains
// neither secret.
func TestISolarHeadersCarrySecretsOnlyInHeaders(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/platform/getDevicePointMinuteDataList",
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

// TestISolarConfigErrorsAreErrConfig: a missing endpoint template or a
// missing credential-shaped Extra value is refused before any request is
// sent, with the chosen config-error Kind (ErrConfig — R48/I5; see
// client.go's configError doc) and MUST NOT be ErrAuth, so F3's
// credential-health logic never treats a misconfiguration as a rejected
// credential.
func TestISolarConfigErrorsAreErrConfig(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	t.Run("missing endpoint template", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Endpoints, isolarOpGetDevicePointMinuteDataList)
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
	})

	t.Run("missing gateway", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Endpoints, isolarEndpointGateway)
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
	})

	t.Run("missing app_key", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Extra, "app_key")
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
	})

	t.Run("missing access_token", func(t *testing.T) {
		creds := isolarTestCreds(srv)
		delete(creds.Extra, "access_token")
		_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
		require.ErrorIs(t, err, integration.ErrConfig)
		require.NotErrorIs(t, err, integration.ErrAuth)
	})

	require.Empty(t, srv.Requests(), "a config error must never reach the network")
}

// TestISolarPlantsAndFaultsMapEveryField exercises Plants and Faults, whose
// task-brief tests are not individually named (unlike DeviceMinuteSeries's
// fixture matrix). Field mapping is checked here so both calls are still
// proven correct end to end.
func TestISolarPlantsAndFaultsMapEveryField(t *testing.T) {
	plantsBody := []byte(`{"result_code":"1","result_msg":"success","result_data":{"rowCount":1,"pageList":[{"ps_id":"FX3001","ps_name":"Fixture Plant","installed_power":"220.500"}]}}`)
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/openapi/platform/queryPowerStationList", Respond: fake.JSON(http.StatusOK, plantsBody)},
		fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getFaultAlarmInfo", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_faults.json"))},
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

	// isolar_faults.json's create_time sits inside [fixtureFrom, fixtureFrom+48h).
	faults, err := c.Faults(context.Background(), creds, fixtureFrom, fixtureFrom.Add(48*time.Hour))
	require.NoError(t, err)
	require.Len(t, faults, 1)
	require.Equal(t, "W-01", faults[0].Ref)
	require.Equal(t, "FX3001", faults[0].PSID)
	require.NotNil(t, faults[0].PSKey)
	require.Equal(t, "FX2001", *faults[0].PSKey)
	require.Equal(t, "W-01", faults[0].Code)
	require.Equal(t, "Fixture Fault", faults[0].Message)
	require.False(t, faults[0].OccurredAt.IsZero())
}

// TestISolarFaultsAcceptsRowCountAndPageListFallback: wire.go's
// wirePagedFaults doc — a real getFaultAlarmInfo response has been observed
// spelling this call's paging keys BOTH page_list/row_count (snake_case)
// and pageList/rowCount (camelCase); this package must decode either.
func TestISolarFaultsAcceptsRowCountAndPageListFallback(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"row_count":1,"page_list":[
		{"ps_id":"FX3002","ps_key":"FX2002","fault_code":"W-02","fault_name":"Snake Case Fault","create_time":"20260102090000"}
	]}}`)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getFaultAlarmInfo", Respond: fake.JSON(http.StatusOK, body)})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	faults, err := c.Faults(context.Background(), creds, fixtureFrom, fixtureFrom.Add(48*time.Hour))
	require.NoError(t, err)
	require.Len(t, faults, 1)
	require.Equal(t, "FX3002", faults[0].PSID)
	require.Equal(t, "Snake Case Fault", faults[0].Message)
}

// TestISolarDevicesAcceptsRowCountAndPageDataFallback: wire.go's
// wirePagedDevices doc — bcem-energy's OWN two call sites disagree on
// getDeviceListByPsId's response shape (plants/route.ts hedges pageList/
// page_data with a comment saying pageList is real; devices/route.ts reads
// ONLY page_data/row_count with no fallback at all). This package must
// decode either.
func TestISolarDevicesAcceptsRowCountAndPageDataFallback(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"row_count":1,"page_data":[
		{"ps_key":"FX2003","device_sn":"FX_0003","device_type":1,"device_name":"Snake Case Device"}
	]}}`)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getDeviceListByPsId", Respond: fake.JSON(http.StatusOK, body)})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	devices, err := c.Devices(context.Background(), creds, "FX3003")
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "FX2003", devices[0].PSKey)
	require.Equal(t, "FX_0003", devices[0].DeviceSN)
}

// TestISolarVerifyMapsAuthFailure mirrors the meter adapters'
// Test<P>VerifyMapsAuthFailure.
func TestISolarVerifyMapsAuthFailure(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/platform/queryPowerStationList",
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
		Method: http.MethodPost, Path: "/openapi/platform/queryPowerStationList",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_auth_failure.json")),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	err := c.Verify(context.Background(), isolarTestCreds(srv))
	require.Error(t, err)
	for _, secret := range []string{fixtureAppKey, fixtureSecretKey, fixtureAccessToken, fixtureRefreshTok} {
		require.False(t, strings.Contains(err.Error(), secret), "error text must not contain %q", secret)
	}
}

// --- fix round 1: recording-Locker SerializeKey coverage (I6) --------------

// recordingLocker is a minimal lock.Locker that records every key Acquire
// was called with and always succeeds immediately — enough to prove WHICH
// SerializeKey each httpx.Client call used, without a real distributed
// lock.
type recordingLocker struct {
	mu   sync.Mutex
	keys []string
}

func (r *recordingLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	r.mu.Lock()
	r.keys = append(r.keys, key)
	r.mu.Unlock()
	return recordingLease{}, nil
}

func (r *recordingLocker) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.keys))
	copy(out, r.keys)
	return out
}

type recordingLease struct{}

func (recordingLease) Release(context.Context) error { return nil }

// TestISolarSerializeKeyCoversAuthAndData proves adapter-patterns.md item
// 15 end to end: EVERY httpx.Client this package builds — an auth call
// (ExchangeCode, which goes through token.go/auth.go) AND a data call
// (DeviceMinuteSeries) — acquires the SAME SerializeKey,
// "isolar:<company id>". MUTATION PROOF (task's fix-round-1 requirement):
// removing SerializeKey from httpClient (or hard-coding it without the
// company id) makes this test fail with either zero recorded keys or a key
// that does not equal wantKey — see task-13-report.md's Fix round 1
// section for the observed failure output.
func TestISolarSerializeKeyCoversAuthAndData(t *testing.T) {
	srv := fake.NewTLSServer(t,
		fake.Route{Method: http.MethodPost, Path: "/openapi/apiManage/token", Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_token.json"))},
		deviceMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_device_minute.json"))),
	)
	rl := &recordingLocker{}
	c := isolar.New(isolarTestPoolWithLocker(t, srv, rl), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.ExchangeCode(context.Background(), creds, "FIXTURE-CODE-1", "https://app.example.invalid/callback")
	require.NoError(t, err)
	_, err = c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)

	wantKey := "isolar:" + creds.CompanyID.String()
	keys := rl.recorded()
	require.Len(t, keys, 2, "both the auth call and the data call must acquire the lock exactly once each")
	require.Equal(t, wantKey, keys[0], "the auth call (ExchangeCode) must carry the SerializeKey")
	require.Equal(t, wantKey, keys[1], "the data call (DeviceMinuteSeries) must carry the SerializeKey")
}
