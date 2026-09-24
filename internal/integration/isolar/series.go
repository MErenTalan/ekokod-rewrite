package isolar

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/shopspring/decimal"
)

// opGetDevicePointMinuteDataList / opGetPowerStationPointMinuteDataList are
// integration_definitions.json's isolar row keys (R40) for the two
// minute-series calls. "Minute series result_data is keyed by ps_key/ps_id"
// (task brief): the device call's result_data is a map keyed by ps_key, one
// entry per requested device; the plant call's is a map keyed by ps_id,
// always exactly one entry (the single plant asked for).
const (
	opGetDevicePointMinuteDataList       = "get_device_point_minute_data_list"
	opGetPowerStationPointMinuteDataList = "get_power_station_point_minute_data_list"
)

// legacyMaxSubWindow is isolarClient.ts:593,621's own documented per-request
// cap ("API supports max 3-hour time intervals per request"). R42: a
// [from, to) accepted by DeviceMinuteSeries/PlantMinuteSeries (bounded by
// MaxWindowMinute) is internally split into contiguous half-open
// sub-windows no larger than this, one call per sub-window, merged in
// order.
const legacyMaxSubWindow = 3 * time.Hour

// YieldSample is one minute-series reading from a device (PSKey set) or a
// plant (PSKey nil). YieldTodayKwh is CUMULATIVE since Istanbul midnight
// (points 1 / 83022), never an interval; callers derive intervals (R277).
type YieldSample struct {
	PSKey         *string
	Ts            time.Time
	YieldTodayKwh *decimal.Decimal
	ActivePowerKw *decimal.Decimal
}

// DeviceMinuteSeries fetches minute-interval production for the given
// ps_keys over [from, to). R42: the window is refused (a non-retryable
// *integration.Error, before any call) if it exceeds MaxWindowMinute (1
// day); an accepted window is internally split into contiguous <=3h
// half-open sub-windows (legacyMaxSubWindow) and issued as one call per
// sub-window, merged in order. psKeys is refused empty (no call, no error —
// "no keys requested" is a valid degenerate case, matching the general
// adapter convention that an empty request yields an empty result, not a
// failure).
func (c *Client) DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]YieldSample, error) {
	if len(psKeys) == 0 {
		return nil, nil
	}
	if err := checkMaxWindow(from, to, MaxWindowMinute); err != nil {
		return nil, err
	}

	var out []YieldSample
	for _, w := range subWindows(from, to, legacyMaxSubWindow) {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetDevicePointMinuteDataList,
			bearer: true,
			body: map[string]any{
				"ps_key_list":       psKeys,
				"points":            devicePoints,
				"start_time_stamp":  formatIsolarTime(w[0]),
				"end_time_stamp":    formatIsolarTime(w[1]),
				"minute_interval":   c.minuteInterval,
				"is_get_point_dict": "1",
			},
		})
		if err != nil {
			return nil, err
		}

		var byKey map[string][]wirePoint
		if err := json.Unmarshal(raw, &byKey); err != nil {
			return nil, malformedErr(opGetDevicePointMinuteDataList)
		}

		for psKey, points := range byKey {
			key := psKey
			for _, wp := range points {
				s, mapErr := mapPoint(wp.TimeStamp, wp.Point1, wp.Point24, w[0], w[1])
				if mapErr != nil {
					// adapter-patterns.md item 7: skip one bad row rather
					// than failing the whole call. See plants.go's
					// Plants/Devices for the same no-warning-channel scope
					// note.
					continue
				}
				s.PSKey = &key
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// PlantMinuteSeries fetches plant-level minute-interval production for
// psID over [from, to). Every returned ProductionSample has PSKey and
// DeviceSN nil — it is, by definition, the BLOCKER's plant-level case.
// Same R42 MaxWindowMinute refusal and <=3h sub-window splitting as
// DeviceMinuteSeries; psID is refused empty before any call
// (adapter-patterns.md item 12 — unlike a ps_key LIST, a single empty psID
// can only mean a caller error, never "fetch nothing").
func (c *Client) PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]YieldSample, error) {
	if psID == "" {
		return nil, c.configError(opGetPowerStationPointMinuteDataList)
	}
	if err := checkMaxWindow(from, to, MaxWindowMinute); err != nil {
		return nil, err
	}

	var out []YieldSample
	for _, w := range subWindows(from, to, legacyMaxSubWindow) {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetPowerStationPointMinuteDataList,
			bearer: true,
			body: map[string]any{
				// isolarClient.ts:610: ps_id_list is an ARRAY, even for one
				// plant — this call has no singular ps_id request field.
				"ps_id_list":        []string{psID},
				"points":            plantPoints,
				"start_time_stamp":  formatIsolarTime(w[0]),
				"end_time_stamp":    formatIsolarTime(w[1]),
				"minute_interval":   c.minuteInterval,
				"is_get_point_dict": "1",
			},
		})
		if err != nil {
			return nil, err
		}

		var byKey map[string][]wirePoint
		if err := json.Unmarshal(raw, &byKey); err != nil {
			return nil, malformedErr(opGetPowerStationPointMinuteDataList)
		}

		for _, points := range byKey {
			for _, wp := range points {
				s, mapErr := mapPoint(wp.TimeStamp, wp.Point83022, wp.Point83025, w[0], w[1])
				if mapErr != nil {
					continue
				}
				out = append(out, s) // PSKey nil: a plant-level sample (R276)
			}
		}
	}
	return out, nil
}

// checkMaxWindow refuses (R42), before any call, a [from, to) wider than
// max or empty/inverted (to <= from). Non-retryable — R48/I5: ErrConfig
// (this used to report ErrAuth; an over-wide window request is a caller
// precondition failure, not an authentication failure — see configError's
// doc in client.go for the same reclassification).
func checkMaxWindow(from, to time.Time, max time.Duration) error {
	if !from.Before(to) {
		return &integration.Error{Kind: integration.ErrConfig, Provider: integration.ProviderISolar, Op: "window"}
	}
	if to.Sub(from) > max {
		return &integration.Error{Kind: integration.ErrConfig, Provider: integration.ProviderISolar, Op: "window"}
	}
	return nil
}

// subWindows splits [from, to) into contiguous, half-open sub-windows of at
// most size each — the LAST sub-window is clipped to end exactly at to, so
// the split is exact regardless of whether (to-from) is a whole multiple of
// size (R42: "internally splits ... into <=3h contiguous half-open
// sub-windows").
func subWindows(from, to time.Time, size time.Duration) [][2]time.Time {
	var out [][2]time.Time
	cur := from
	for cur.Before(to) {
		end := cur.Add(size)
		if end.After(to) {
			end = to
		}
		out = append(out, [2]time.Time{cur, end})
		cur = end
	}
	return out
}

// mapPoint converts a yield (Wh) and power (W) pair to kWh/kW (06 §6), dropping
// a row whose timestamp does not parse or falls outside [from, to).
func mapPoint(stamp string, yieldWh, powerW *string, from, to time.Time) (YieldSample, error) {
	ts, err := normalize.LocalLayout(isolarTimestampLayout, stamp)
	if err != nil {
		return YieldSample{}, err
	}
	if ts.Before(from) || !ts.Before(to) {
		return YieldSample{}, errOutsideWindow
	}
	yield, err := normalize.OptionalNumber(derefOr(yieldWh, ""))
	if err != nil {
		return YieldSample{}, err
	}
	power, err := normalize.OptionalNumber(derefOr(powerW, ""))
	if err != nil {
		return YieldSample{}, err
	}
	return YieldSample{Ts: ts, YieldTodayKwh: normalize.WhToKWh(yield), ActivePowerKw: normalize.WToKW(power)}, nil
}

// errOutsideWindow marks a row mapPoint drops for falling outside
// [from, to); it is never wrapped into a returned error — the caller skips
// the row and continues (adapter-patterns.md item 4).
var errOutsideWindow = errors.New("isolar: sample outside window")
