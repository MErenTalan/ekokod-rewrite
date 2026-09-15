package isolar_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

// TestISolarPlantSeriesHasNoDeviceKey pins the BLOCKER at the adapter
// boundary: PlantMinuteSeries's samples always carry PSKey == nil and
// DeviceSN == nil — internal/ingest/production.Store relies on exactly
// this to decide what gets quarantined.
func TestISolarPlantSeriesHasNoDeviceKey(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/getPowerStationPointMinuteDataList",
		Respond: fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_plant_minute.json")),
	})
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
}

// TestISolarDeviceMinuteSeriesWindowIsHalfOpen: adapter-patterns.md item 4 —
// From-1s dropped, From kept, To dropped.
func TestISolarDeviceMinuteSeriesWindowIsHalfOpen(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"FX1001":[
		{"time_stamp":"2025-12-31 23:59:59","point_id_1":"1","point_id_24":"1","point_id_2001":"1","point_id_2009":"1","point_id_2010":"1"},
		{"time_stamp":"2026-01-01 00:00:00","point_id_1":"2","point_id_24":"2","point_id_2001":"2","point_id_2009":"2","point_id_2010":"2"},
		{"time_stamp":"2026-01-01 01:00:00","point_id_1":"3","point_id_24":"3","point_id_2001":"3","point_id_2009":"3","point_id_2010":"3"}
	]}}`)
	srv := fake.NewTLSServer(t, fake.Route{
		Method: http.MethodPost, Path: "/openapi/apiManage/getDevicePointMinuteDataList",
		Respond: fake.JSON(http.StatusOK, body),
	})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1, "only the exactly-From row is kept; From-1s and To are both dropped")
	require.True(t, samples[0].Ts.Equal(fixtureFrom.UTC()))
}

// TestISolarDeviceMinuteSeriesRejectsEmptyPSKeys: a defensive request check
// (adapter-patterns.md item 12) — no ps_keys means no call at all.
func TestISolarDeviceMinuteSeriesRejectsEmptyPSKeys(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	creds := isolarTestCreds(srv)

	samples, err := c.DeviceMinuteSeries(context.Background(), creds, nil, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Empty(t, samples)
	require.Empty(t, srv.Requests(), "an empty ps_key list must never reach the network")
}
