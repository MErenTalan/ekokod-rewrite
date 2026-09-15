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
// 06 §6's operation names for the two minute-series calls. "Minute series
// result_data is keyed by ps_key/ps_id" (task brief): the device call's
// result_data is a map keyed by ps_key, one entry per requested device; the
// plant call's is a map keyed by ps_id, always exactly one entry (the
// single plant asked for).
const (
	opGetDevicePointMinuteDataList       = "getDevicePointMinuteDataList"
	opGetPowerStationPointMinuteDataList = "getPowerStationPointMinuteDataList"
)

// isolarTimeLayout is this package's own choice for time_stamp's wire
// format ("yyyy-MM-dd HH:mm:ss", Sungrow's public OpenAPI convention) —
// see wire.go's provenance note; UNVERIFIED against a real response.
const isolarTimeLayout = "2006-01-02 15:04:05"

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
// ps_keys over [from, to). MaxWindow enforcement (1 day for minute series,
// Provider defaults table) is the caller's responsibility — Client makes
// exactly one call per invocation, over whatever [from, to) it is given.
func (c *Client) DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]ProductionSample, error) {
	if len(psKeys) == 0 {
		return nil, nil
	}
	raw, err := c.call(ctx, creds, callOptions{
		op:     opGetDevicePointMinuteDataList,
		bearer: true,
		body: map[string]any{
			"ps_key_list":      psKeys,
			"start_time_stamp": formatIsolarTime(from),
			"end_time_stamp":   formatIsolarTime(to),
			"minute_interval":  c.minuteInterval,
			"point_id_list":    pointIDList,
		},
	})
	if err != nil {
		return nil, err
	}

	var byKey map[string][]wirePoint
	if err := json.Unmarshal(raw, &byKey); err != nil {
		return nil, malformedErr(opGetDevicePointMinuteDataList)
	}

	var out []ProductionSample
	for psKey, points := range byKey {
		key := psKey
		for _, wp := range points {
			s, mapErr := mapPoint(wp, from, to)
			if mapErr != nil {
				// adapter-patterns.md item 7: skip one bad row rather than
				// failing the whole call. See plants.go's Plants/Devices
				// for the same no-warning-channel scope note.
				continue
			}
			s.PSKey = &key
			out = append(out, s)
		}
	}
	return out, nil
}

// PlantMinuteSeries fetches plant-level minute-interval production for
// psID over [from, to). Every returned ProductionSample has PSKey and
// DeviceSN nil — it is, by definition, the BLOCKER's plant-level case.
func (c *Client) PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]ProductionSample, error) {
	raw, err := c.call(ctx, creds, callOptions{
		op:     opGetPowerStationPointMinuteDataList,
		bearer: true,
		body: map[string]any{
			"ps_id":            psID,
			"start_time_stamp": formatIsolarTime(from),
			"end_time_stamp":   formatIsolarTime(to),
			"minute_interval":  c.minuteInterval,
			"point_id_list":    pointIDList,
		},
	})
	if err != nil {
		return nil, err
	}

	var byKey map[string][]wirePoint
	if err := json.Unmarshal(raw, &byKey); err != nil {
		return nil, malformedErr(opGetPowerStationPointMinuteDataList)
	}

	var out []ProductionSample
	for _, points := range byKey {
		for _, wp := range points {
			s, mapErr := mapPoint(wp, from, to)
			if mapErr != nil {
				continue
			}
			s.PSID = psID
			// PSKey/DeviceSN stay nil: this IS the plant-level BLOCKER case.
			out = append(out, s)
		}
	}
	return out, nil
}

// mapPoint converts one wirePoint into a ProductionSample, applying the
// unit conversions 06 §6 requires (Wh→kWh for point 1, W→kW for point 24)
// and dropping a row whose timestamp does not parse or falls outside
// [from, to) — adapter-patterns.md item 4's half-open window rule.
func mapPoint(w wirePoint, from, to time.Time) (ProductionSample, error) {
	ts, err := normalize.LocalLayout(isolarTimeLayout, w.TimeStamp)
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
