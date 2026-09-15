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

// ProductionSample is one minute-series reading, from either a device
// (PSKey non-nil) or a plant (PSKey nil — the BLOCKER case: a plant-level
// sample carries no device attribution and internal/ingest/production
// quarantines it rather than storing it against a fabricated device).
type ProductionSample struct {
	// PSID is populated only when the caller's own request already names
	// it: PlantMinuteSeries sets it to the psID it was asked for (the
	// provider's minute-series result_data has no per-record ps_id/ps_key
	// pair — see wire.go), and DeviceMinuteSeries leaves it empty (its
	// caller — internal/ingest/production.Store — already has the plant id
	// as an explicit parameter, so this field is informational only, never
	// load-bearing for store attribution).
	PSID     string
	PSKey    *string
	DeviceSN *string
	Ts       time.Time

	ProductionKwh *decimal.Decimal
	ActivePowerKw *decimal.Decimal
	IrradianceWm2 *decimal.Decimal
	ModuleTempC   *decimal.Decimal
	AmbientTempC  *decimal.Decimal
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
func (c *Client) DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]ProductionSample, error) {
	if len(psKeys) == 0 {
		return nil, nil
	}
	if err := checkMaxWindow(from, to, MaxWindowMinute); err != nil {
		return nil, err
	}

	var out []ProductionSample
	for _, w := range subWindows(from, to, legacyMaxSubWindow) {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetDevicePointMinuteDataList,
			bearer: true,
			body: map[string]any{
				"ps_key_list":       psKeys,
				"points":            pointIDList,
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
				s, mapErr := mapPoint(wp, w[0], w[1])
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
func (c *Client) PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]ProductionSample, error) {
	if psID == "" {
		return nil, c.configError(opGetPowerStationPointMinuteDataList)
	}
	if err := checkMaxWindow(from, to, MaxWindowMinute); err != nil {
		return nil, err
	}

	var out []ProductionSample
	for _, w := range subWindows(from, to, legacyMaxSubWindow) {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetPowerStationPointMinuteDataList,
			bearer: true,
			body: map[string]any{
				// isolarClient.ts:610: ps_id_list is an ARRAY, even for one
				// plant — this call has no singular ps_id request field.
				"ps_id_list":        []string{psID},
				"points":            pointIDList,
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
				s, mapErr := mapPoint(wp, w[0], w[1])
				if mapErr != nil {
					continue
				}
				s.PSID = psID
				// PSKey/DeviceSN stay nil: this IS the plant-level BLOCKER case.
				out = append(out, s)
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

// mapPoint converts one wirePoint into a ProductionSample, applying the
// unit conversions 06 §6 requires (Wh→kWh for point 1, W→kW for point 24)
// and dropping a row whose timestamp does not parse or falls outside
// [from, to) — adapter-patterns.md item 4's half-open window rule.
func mapPoint(w wirePoint, from, to time.Time) (ProductionSample, error) {
	ts, err := normalize.LocalLayout(isolarTimestampLayout, w.TimeStamp)
	if err != nil {
		return ProductionSample{}, err
	}
	if ts.Before(from) || !ts.Before(to) {
		return ProductionSample{}, errOutsideWindow
	}

	yieldWh, err := normalize.OptionalNumber(derefOr(w.Point1, ""))
	if err != nil {
		return ProductionSample{}, err
	}
	powerW, err := normalize.OptionalNumber(derefOr(w.Point24, ""))
	if err != nil {
		return ProductionSample{}, err
	}
	irradiance, err := normalize.OptionalNumber(derefOr(w.Point2001, ""))
	if err != nil {
		return ProductionSample{}, err
	}
	ambient, err := normalize.OptionalNumber(derefOr(w.Point2009, ""))
	if err != nil {
		return ProductionSample{}, err
	}
	module, err := normalize.OptionalNumber(derefOr(w.Point2010, ""))
	if err != nil {
		return ProductionSample{}, err
	}

	return ProductionSample{
		Ts:            ts,
		ProductionKwh: normalize.WhToKWh(yieldWh),
		ActivePowerKw: normalize.WToKW(powerW),
		IrradianceWm2: irradiance,
		AmbientTempC:  ambient,
		ModuleTempC:   module,
	}, nil
}

// errOutsideWindow marks a row mapPoint drops for falling outside
// [from, to); it is never wrapped into a returned error — the caller skips
// the row and continues (adapter-patterns.md item 4).
var errOutsideWindow = errors.New("isolar: sample outside window")
