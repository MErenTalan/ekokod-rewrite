package isolar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
)

func requestBody(t *testing.T, srv *fake.Server, i int) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(srv.Requests()[i].Body, &body))
	return body
}

// R277: p1 is Yield Today (cumulative Wh since midnight) and reaches callers as kWh, unchanged in meaning.
func TestDeviceMinuteSeriesReturnsCumulativeYieldInKwh(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"FX1001":[
		{"time_stamp":"20260101000500","p1":"12500","p24":"3200"}]}}`)
	srv := fake.NewTLSServer(t, deviceMinuteRoute(fake.JSON(http.StatusOK, body)))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	samples, err := c.DeviceMinuteSeries(context.Background(), isolarTestCreds(srv), []string{"FX1001"}, fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.True(t, decimal.RequireFromString("12.5").Equal(*samples[0].YieldTodayKwh))
	require.True(t, decimal.RequireFromString("3.2").Equal(*samples[0].ActivePowerKw))
	require.Equal(t, "1,24", requestBody(t, srv, 0)["points"])
}

// F2 defect 2: a plant has its own points, 83022 (daily yield) and 83025 (active power).
func TestPlantMinuteSeriesAsksForPlantPoints(t *testing.T) {
	srv := fake.NewTLSServer(t, plantMinuteRoute(fake.JSON(http.StatusOK, fake.Fixture(t, "isolar", "isolar_plant_minute.json"))))
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	samples, err := c.PlantMinuteSeries(context.Background(), isolarTestCreds(srv), "FX3001", fixtureFrom, fixtureTo)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.True(t, decimal.RequireFromString("5").Equal(*samples[0].YieldTodayKwh))
	require.True(t, decimal.RequireFromString("8").Equal(*samples[0].ActivePowerKw))
	require.Equal(t, "83022,83025", requestBody(t, srv, 0)["points"])
}

func TestPlantDailySeriesOneRowPerDay(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"FX3001":{"p83022":[
		{"time_stamp":"20260101","2":"41000"},{"time_stamp":"20260102","2":null},{"time_stamp":"20260103","2":"39500"}]}}}`)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getPowerStationPointDayMonthYearDataList",
		Respond: fake.JSON(http.StatusOK, body)})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	istanbul, _ := time.LoadLocation("Europe/Istanbul")
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, istanbul)

	days, err := c.PlantDailySeries(context.Background(), isolarTestCreds(srv), "FX3001", from, from.AddDate(0, 0, 3))
	require.NoError(t, err)
	require.Len(t, days, 3)
	require.True(t, days[0].Day.Equal(from))
	require.True(t, decimal.RequireFromString("41").Equal(*days[0].Kwh))
	require.Nil(t, days[1].Kwh, "a null day is missing, never zero")
	require.True(t, days[2].Day.Equal(from.AddDate(0, 0, 2)))

	req := requestBody(t, srv, 0)
	require.Equal(t, "1", req["query_type"])
	require.Equal(t, "2", req["data_type"])
	require.Equal(t, "p83022", req["data_point"])
	require.Equal(t, "20260101", req["start_time"])
	require.Equal(t, "20260103", req["end_time"], "end_time is inclusive: the last day before `to`")
	require.Equal(t, []any{"FX3001"}, req["ps_id_list"])
}

func TestPlantDailySeriesRefusesMoreThanAMonth(t *testing.T) {
	srv := fake.NewTLSServer(t)
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})
	_, err := c.PlantDailySeries(context.Background(), isolarTestCreds(srv), "FX3001", fixtureFrom, fixtureFrom.AddDate(0, 0, 32))
	require.Error(t, err)
	require.Empty(t, srv.Requests())
}

func TestDeviceRealtimeNormalisesUnits(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"device_point_list":[
		{"device_point":{"ps_key":"FX1001","device_sn":"SN-1","device_time":"20260101120000","dev_fault_status":4,
		 "p24":"3200","p1":"12500","p87":"250000","p88":"3100000","p2":"45000000"}}]}}`)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getDeviceRealTimeData",
		Respond: fake.JSON(http.StatusOK, body)})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	snaps, err := c.DeviceRealtime(context.Background(), isolarTestCreds(srv), 1, []string{"FX1001"})
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	s := snaps[0]
	require.Equal(t, "FX1001", s.PSKey)
	require.Equal(t, "SN-1", s.DeviceSN)
	require.Equal(t, int32(4), *s.FaultStatus)
	require.True(t, decimal.RequireFromString("3.2").Equal(*s.ActivePowerKw))
	require.True(t, decimal.RequireFromString("12.5").Equal(*s.YieldTodayKwh))
	require.True(t, decimal.RequireFromString("250").Equal(*s.YieldMonthKwh))
	require.True(t, decimal.RequireFromString("3100").Equal(*s.YieldYearKwh))
	require.True(t, decimal.RequireFromString("45000").Equal(*s.YieldTotalKwh))
	require.True(t, s.At.Equal(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)), "device_time is Istanbul local")

	req := requestBody(t, srv, 0)
	require.EqualValues(t, 1, req["device_type"])
	require.Equal(t, []any{"24", "1", "87", "88", "2"}, req["point_id_list"])
	require.Equal(t, []any{"FX1001"}, req["ps_key_list"])
}

// F2 defect 3: fault_code is a type; the ref names one occurrence.
func TestFaultRefIsPerOccurrence(t *testing.T) {
	body := []byte(`{"result_code":"1","result_msg":"success","result_data":{"rowCount":2,"pageList":[
		{"ps_id":"FX3001","ps_key":"FX2001","fault_code":"10","fault_name":"电网掉电","create_time":"20260101100000","fault_level":2,"fault_type":1,"device_name":"INV-1","over_time":"20260101110000"},
		{"ps_id":"FX3001","ps_key":"FX2001","fault_code":"10","fault_name":"电网掉电","create_time":"20260101150000"}]}}`)
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodPost, Path: "/openapi/platform/getFaultAlarmInfo", Respond: fake.JSON(http.StatusOK, body)})
	c := isolar.New(isolarTestPool(t, srv), isolar.Options{Clock: clock.NewFake(fixtureFrom)})

	faults, err := c.Faults(context.Background(), isolarTestCreds(srv), fixtureFrom, fixtureFrom.Add(48*time.Hour))
	require.NoError(t, err)
	require.Len(t, faults, 2)
	require.NotEqual(t, faults[0].Ref, faults[1].Ref)
	require.Equal(t, int32(2), *faults[0].Level)
	require.Equal(t, int32(1), *faults[0].Type)
	require.Equal(t, "INV-1", *faults[0].DeviceName)
	require.NotNil(t, faults[0].ClosedAt)
	require.Nil(t, faults[1].ClosedAt)
}

// R299: no exported adapter type carries watts or watt-hours.
func TestNoWattsCrossTheAdapter(t *testing.T) {
	for _, v := range []any{isolar.YieldSample{}, isolar.DailyYield{}, isolar.DeviceSnapshot{}, isolar.Plant{}, isolar.Device{}, isolar.Fault{}} {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			name := typ.Field(i).Name
			for _, bad := range []string{"Wh", "W"} {
				if strings.HasSuffix(name, bad) && !strings.HasSuffix(name, "k"+bad) {
					t.Errorf("%s.%s carries a watt unit across the adapter boundary", typ.Name(), name)
				}
			}
			if typ.Field(i).Type == reflect.TypeOf(&decimal.Decimal{}) && !strings.HasSuffix(name, "Kwh") && !strings.HasSuffix(name, "Kw") {
				t.Errorf("%s.%s is a quantity without a kWh/kW unit in its name", typ.Name(), name)
			}
		}
	}
}
