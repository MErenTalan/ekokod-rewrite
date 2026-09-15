package isolar

import "encoding/json"

// This file holds ONLY provider-shaped structs (the adapter template's
// rule): every field is a string, *string or json.Number so decoding never
// touches float64 or `any` (internal/arch's
// TestIntegrationTreesDoNotParseFloats). Conversion to decimal.Decimal /
// time.Time happens in the mapping code in plants.go, series.go and
// alarms.go, never here.
//
// PROVENANCE: 06-integrations.md §6 gives the operation names
// (queryPowerStationList, getDeviceListByPsId, getDevicePointMinuteDataList,
// getPowerStationPointMinuteDataList, getFaultAlarmInfo), the point-id
// catalogue and the top-level envelope shape ({result_code, result_msg,
// result_data}, "1" = success). It does NOT give the exact field names
// inside result_data — the legacy TypeScript interfaces it cites
// (isolarTypes.ts) are not present in this repository. The field names
// below are this task's own, chosen to match the spec's own vocabulary
// (ps_id, ps_key, device_sn, point_id_<n>, curPage/size/rowCount paging)
// and Sungrow's publicly documented OpenAPI convention. Like R21's result
// codes and R35's OSOS field semantics, this shape is UNVERIFIED against a
// real response — verify in F14 (06 §6 note: "iSolar error codes are to be
// verified against real responses in F14").
//
// envelope is every call's top-level response shape. ResultCode is a
// pointer so "the key is missing or null" (malformed: adapter-pattern
// item 6) is distinguishable from "the key is present and empty" — both
// are shape violations, but only the pointer lets decodeEnvelope tell them
// apart from a genuine (if empty-string) provider code.
type envelope struct {
	ResultCode *string         `json:"result_code"`
	ResultMsg  string          `json:"result_msg"`
	ResultData json.RawMessage `json:"result_data"`
}

// wireTokenData is result_data for /openapi/apiManage/token and
// /openapi/apiManage/refreshToken. ExpiresIn is a pointer: absent means the
// legacy default of 7200s (R22) applies, and json.Number (not int) keeps a
// literal string parse rather than a float conversion.
type wireTokenData struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    *json.Number `json:"expires_in"`
}

// wirePagedPlants is result_data for queryPowerStationList.
type wirePagedPlants struct {
	RowCount json.Number `json:"rowCount"`
	PageList []wirePlant `json:"pageList"`
}

type wirePlant struct {
	PsID          string  `json:"ps_id"`
	PsName        string  `json:"ps_name"`
	TotalCapacity *string `json:"total_capacity"` // kW, no unit conversion
}

// wirePagedDevices is result_data for getDeviceListByPsId.
type wirePagedDevices struct {
	RowCount json.Number  `json:"rowCount"`
	PageList []wireDevice `json:"pageList"`
}

type wireDevice struct {
	PsKey      string       `json:"ps_key"`
	DeviceSN   string       `json:"device_sn"`
	DeviceType *json.Number `json:"device_type"`
	DeviceName *string      `json:"device_name"`
}

// wirePoint is one minute-series sample for either a device (keyed by
// ps_key) or a plant (keyed by ps_id) — 06 §6's measurement-point
// catalogue: 1 yield today (Wh), 24 total active power (W), 2001 daily
// horizontal irradiation (Wh/m²), 2009 ambient temperature (°C), 2010
// module temperature (°C). A field is a pointer so "", "null" and "-" all
// parse to nil via normalize.OptionalNumber (removed-behaviour 21: never a
// fabricated zero).
type wirePoint struct {
	TimeStamp string  `json:"time_stamp"`
	Point1    *string `json:"point_id_1"`
	Point24   *string `json:"point_id_24"`
	Point2001 *string `json:"point_id_2001"`
	Point2009 *string `json:"point_id_2009"`
	Point2010 *string `json:"point_id_2010"`
}

// wirePagedFaults is result_data for getFaultAlarmInfo.
type wirePagedFaults struct {
	RowCount json.Number `json:"rowCount"`
	PageList []wireFault `json:"pageList"`
}

type wireFault struct {
	AlarmID     string  `json:"alarm_id"`
	PsID        string  `json:"ps_id"`
	PsKey       *string `json:"ps_key"`
	WarningCode string  `json:"warning_code"`
	WarningName string  `json:"warning_name"`
	StartTime   string  `json:"start_time"`
}
