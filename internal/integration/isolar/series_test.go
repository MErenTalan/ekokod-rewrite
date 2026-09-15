package isolar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

const isolarWireTimeLayout = "20060102150405" // yyyyMMddHHmmss — mirrors series.go's isolarTimestampLayout

func plantMinuteRoute(respond fake.Responder) fake.Route {
	return fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getPowerStationPointMinuteDataList", Respond: respond}
}

// TestISolarPlantSeriesHasNoDeviceKey pins the BLOCKER at the adapter
// boundary: PlantMinuteSeries's samples always carry PSKey == nil and
// DeviceSN == nil — internal/ingest/production.Store relies on exactly
// this to decide what gets quarantined.
func TestISolarPlantSeriesHasNoDeviceKey(t *testing.T) {
	srv := fake.NewTLSServer(t, plantMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_plant_minute.json"))))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.PlantMinuteSeries(context.Background(), creds, "FX3001", fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Nil(t, samples[0].PSKey, "a plant-level sample must never carry a device key")
	require.Nil(t, samples[0].DeviceSN)
	require.Equal(t, "FX3001", samples[0].PSID)
	require.NotNil(t, samples[0].ProductionKwh)
	require.True(t, decimal.RequireFromString("5").Equal(*samples[0].ProductionKwh))

	// R41 isolarClient.ts:610: ps_id_list is an ARRAY even for one plant.
	reqs := srv.Requests()
	require.Len(t, reqs, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.Equal(t, []any{"FX3001"}, body["ps_id_list"])
	require.NotContains(t, body, "ps_id", "the singular ps_id field does not exist on this call")
}

// TestISolarPlantMinuteSeriesRejectsEmptyPSID: adapter-patterns.md item 12
// — an empty psID means no call at all (unlike DeviceMinuteSeries's
// psKeys, a single required id cannot mean "fetch nothing").
func TestISolarPlantMinuteSeriesRejectsEmptyPSID(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	_, err := c.PlantMinuteSeries(context.Background(), creds, "", fixtureFrom, fixtureTo)
	require.ErrorIs(t, err, integration.ErrConfig) // R48/I5
	require.NotErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests())
}

// TestISolarDeviceMinuteSeriesWindowIsHalfOpen: adapter-patterns.md item 4 —
// From-1s dropped, From kept, To dropped.
func TestISolarDeviceMinuteSeriesWindowIsHalfOpen(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"FX1001":[
		{"time_stamp":"20251231235959","p1":"1","p24":"1","p2001":"1","p2009":"1","p2010":"1"},
		{"time_stamp":"20260101000000","p1":"2","p24":"2","p2001":"2","p2009":"2","p2010":"2"},
		{"time_stamp":"20260101010000","p1":"3","p24":"3","p2001":"3","p2009":"3","p2010":"3"}
	]}}`)
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, body)))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1, "only the exactly-From row is kept; From-1s and To are both dropped")
	require.True(t, samples[0].Ts.Equal(fixtureFrom.UTC()))
}

// TestISolarDeviceMinuteSeriesRejectsEmptyPSKeys: a defensive request check
// (adapter-patterns.md item 12) — no ps_keys means no call at all. An empty
// LIST (unlike PlantMinuteSeries's single required psID) is treated as "no
// series requested", an empty result, not an error — consistent with the
// general adapter convention that an empty request yields an empty result.
func TestISolarDeviceMinuteSeriesRejectsEmptyPSKeys(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, nil, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Empty(t, samples)
	require.Empty(t, srv.Requests(), "an empty ps_key list must never reach the network")
}

// --- fix round 1: R42 MaxWindow + 3h sub-window splitting (I8) -------------

// TestISolarDeviceMinuteSeriesRefusesWindowLargerThanMaxWindow: R42 — a
// [from, to) wider than isolar.MaxWindowMinute (1 day) is refused with a
// non-retryable error BEFORE any call. MUTATION PROOF (task's fix-round-1
// requirement): removing series.go's checkMaxWindow call makes this test
// fail (err becomes nil and the request reaches the fake server, which is
// not configured with any route and would fail the test via
// "fake: unmatched request") — see task-13-report.md's Fix round 1 section
// for the observed failure output.
func TestISolarDeviceMinuteSeriesRefusesWindowLargerThanMaxWindow(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	from := fixtureFrom
	to := from.Add(isolar.MaxWindowMinute + time.Hour)
	_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, from, to)
	require.ErrorIs(t, err, integration.ErrConfig) // R48/I5
	require.NotErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests(), "a window larger than MaxWindowMinute must never reach the network")
}

// TestISolarPlantMinuteSeriesRefusesWindowLargerThanMaxWindow: same R42
// refusal, PlantMinuteSeries side.
func TestISolarPlantMinuteSeriesRefusesWindowLargerThanMaxWindow(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	from := fixtureFrom
	to := from.Add(isolar.MaxWindowMinute + time.Minute)
	_, err := c.PlantMinuteSeries(context.Background(), creds, "FX3001", from, to)
	require.ErrorIs(t, err, integration.ErrConfig) // R48/I5
	require.NotErrorIs(t, err, integration.ErrAuth)
	require.Empty(t, srv.Requests())
}

// TestISolarDeviceMinuteSeriesSplitsIntoThreeHourSubWindows: R42 — an
// accepted window (<=MaxWindowMinute) is internally split into contiguous,
// half-open sub-windows of at most 3h (isolarClient.ts's own documented
// per-request cap), issued as one call per sub-window and merged. A 7h
// window must issue exactly 3 calls: [0,3h), [3h,6h), [6h,7h) — the last
// one clipped, proving the split is exact even when the window is not a
// whole multiple of 3h. MUTATION PROOF (task's fix-round-1 requirement):
// replacing series.go's subWindows(from, to, legacyMaxSubWindow) with a
// single [from, to) window (round 1's original behaviour) makes this test
// fail with len(reqs) == 1, not 3 — see task-13-report.md's Fix round 1
// section for the observed failure output.
func TestISolarDeviceMinuteSeriesSplitsIntoThreeHourSubWindows(t *testing.T) {
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, []byte(`{"result_code":"1","result_msg":"success","result_data":{}}`))))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	from := fixtureFrom
	to := fixtureFrom.Add(7 * time.Hour)
	_, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, from, to)
	require.NoError(t, err)

	reqs := srv.Requests()
	require.Len(t, reqs, 3, "a 7h window split at <=3h sub-windows must issue exactly 3 calls")

	wantBounds := [][2]time.Time{
		{from, from.Add(3 * time.Hour)},
		{from.Add(3 * time.Hour), from.Add(6 * time.Hour)},
		{from.Add(6 * time.Hour), to},
	}
	for i, want := range wantBounds {
		var body map[string]any
		require.NoError(t, json.Unmarshal(reqs[i].Body, &body))
		require.Equal(t, want[0].In(normalize.Istanbul).Format(isolarWireTimeLayout), body["start_time_stamp"], "sub-window %d start", i)
		require.Equal(t, want[1].In(normalize.Istanbul).Format(isolarWireTimeLayout), body["end_time_stamp"], "sub-window %d end", i)
	}
}
