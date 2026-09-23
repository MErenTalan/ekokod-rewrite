package isolar

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

const opGetDeviceRealTimeData = "get_device_real_time_data"

// Inverter device types (legacy isolarTypes.ts): 1 grid-tied, 14 storage/hybrid.
const (
	DeviceTypeInverter        int32 = 1
	DeviceTypeStorageInverter int32 = 14
)

// realtimePoints: 24 active power (W), 1 yield today, 87 month, 88 year, 2 total (Wh).
var realtimePoints = []string{"24", "1", "87", "88", "2"}

// DeviceSnapshot is one inverter's realtime reading, in kW/kWh (R282).
// At is the device's own time when it parses, else the fetch time.
type DeviceSnapshot struct {
	PSKey, DeviceSN string
	FaultStatus     *int32
	At              time.Time
	ActivePowerKw   *decimal.Decimal
	YieldTodayKwh   *decimal.Decimal
	YieldMonthKwh   *decimal.Decimal
	YieldYearKwh    *decimal.Decimal
	YieldTotalKwh   *decimal.Decimal
}

// DeviceRealtime fetches the realtime points of the given devices of one type.
func (c *Client) DeviceRealtime(ctx context.Context, creds integration.Credentials, deviceType int32, psKeys []string) ([]DeviceSnapshot, error) {
	if len(psKeys) == 0 {
		return nil, nil
	}
	raw, err := c.call(ctx, creds, callOptions{
		op:     opGetDeviceRealTimeData,
		bearer: true,
		body: map[string]any{
			"device_type":       deviceType,
			"point_id_list":     realtimePoints,
			"ps_key_list":       psKeys,
			"is_get_point_dict": "1",
		},
	})
	if err != nil {
		return nil, err
	}
	var rt wireRealtime
	if err := json.Unmarshal(raw, &rt); err != nil {
		return nil, malformedErr(opGetDeviceRealTimeData)
	}
	out := make([]DeviceSnapshot, 0, len(rt.DevicePointList))
	for _, item := range rt.DevicePointList {
		p := item.DevicePoint
		s := DeviceSnapshot{PSKey: rawText(p["ps_key"]), DeviceSN: rawText(p["device_sn"]), At: c.clock.Now().UTC()}
		if s.PSKey == "" {
			continue
		}
		if at, err := normalize.LocalLayout(isolarTimestampLayout, rawText(p["device_time"])); err == nil {
			s.At = at
		}
		if v, err := strconv.ParseInt(rawText(p["dev_fault_status"]), 10, 32); err == nil {
			status := int32(v)
			s.FaultStatus = &status
		}
		values := map[string]*decimal.Decimal{}
		bad := false
		for _, id := range realtimePoints {
			v, err := normalize.OptionalNumber(rawText(p["p"+id]))
			if err != nil {
				bad = true
				break
			}
			values[id] = v
		}
		if bad {
			continue // adapter-patterns.md item 7
		}
		s.ActivePowerKw = normalize.WToKW(values["24"])
		s.YieldTodayKwh = normalize.WhToKWh(values["1"])
		s.YieldMonthKwh = normalize.WhToKWh(values["87"])
		s.YieldYearKwh = normalize.WhToKWh(values["88"])
		s.YieldTotalKwh = normalize.WhToKWh(values["2"])
		out = append(out, s)
	}
	return out, nil
}
