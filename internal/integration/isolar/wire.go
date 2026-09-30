package isolar

import (
	"bytes"
	"encoding/json"
	"strings"
)

// flexString decodes a JSON string or number: the live API sends ps_id,
// fault_code and point values either way (arti-scada's fixtures: "ps_id":
// 5001, "fault_code": 10). Objects, arrays and booleans decode to "".
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	*f = ""
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(strings.TrimSpace(s))
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		*f = flexString(b)
	}
	return nil
}

// ptr returns the value as a *string, nil for a nil receiver.
func (f *flexString) ptr() *string {
	if f == nil {
		return nil
	}
	s := string(*f)
	return &s
}

// flexNumber decodes a JSON number or numeric string; "", "--" and other
// non-numbers leave it empty instead of failing the whole page.
type flexNumber string

func (f *flexNumber) UnmarshalJSON(b []byte) error {
	var s flexString
	if err := s.UnmarshalJSON(b); err != nil {
		return err
	}
	var n json.Number
	*f = ""
	if s != "" && json.Unmarshal([]byte(s), &n) == nil {
		*f = flexNumber(n)
	}
	return nil
}

func (f flexNumber) Int64() (int64, error) { return json.Number(f).Int64() }

// This file holds ONLY provider-shaped structs (the adapter template's
// rule): every field is a string, *string or json.Number so decoding never
// touches float64 or `any` (internal/arch's
// TestIntegrationTreesDoNotParseFloats). Conversion to decimal.Decimal /
// time.Time happens in the mapping code in plants.go, series.go and
// alarms.go, never here.
//
// PROVENANCE (fix round 1, R40/R41 — task-13-fix1-findings.md):
// 06-integrations.md §6 gives the operation names, the point-id catalogue
// and the top-level envelope shape ({result_code, result_msg, result_data},
// "1" = success), but NOT the field names inside result_data. R41 makes the
// legacy TypeScript the wire authority for those field names, formats and
// paging:
//   - /mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy/src/utils/isolar/isolarTypes.ts
//   - /mnt/c/Users/meren/Desktop/Work/bcem-apps/bcem-energy/src/utils/isolar/isolarClient.ts
//
// Every field name below is cited against those files by line/section in
// this comment. Two fields (queryPowerStationList/getDeviceListByPsId's
// response paging keys, and getFaultAlarmInfo's) are taken from the
// EMPIRICALLY-OBSERVED shape in bcem-energy's own route handlers
// (src/app/api/integration/isolar/{plants,devices,alarms}/route.ts), which
// defensively read `pageList`/`rowCount` (camelCase) over the
// isolarTypes.ts-declared `page_data`/`row_count` because a code comment in
// that file itself says so ("API actually returns pageList, not
// page_data"); the fault list route additionally hedges `page_list` OR
// `pageList` and `row_count` OR `rowCount`, so this package decodes both.
// This shape is otherwise UNVERIFIED against a live iSolarCloud response —
// same class of gap as R21's result codes; verify in F14.
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
// /openapi/apiManage/refreshToken (isolarTypes.ts:367-372,
// ISolarTokenData). ExpiresIn is a pointer: absent means the legacy default
// of 7200s (R22) applies (isolarClient.ts:293 "Number(data.expires_in ||
// 7200)"), and json.Number (not int) keeps a literal string parse rather
// than a float conversion. RefreshToken is a plain string, not a pointer:
// both "the key is absent" and "the key is JSON null" decode to "" with
// Go's encoding/json (neither is an error for a non-pointer target), which
// is exactly the "no new refresh token" case I4 requires Token.RefreshToken
// to expose as a zero Secret (see token.go).
type wireTokenData struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    *json.Number `json:"expires_in"`
}

// wirePagedPlants is result_data for queryPowerStationList
// (isolarTypes.ts:326-329, QueryPowerStationListData: "pageList?:
// PowerStationListItem[]; rowCount?: number" — confirmed as the real shape
// by bcem-energy's plants/route.ts:47,78, which reads `data.pageList` /
// `data.rowCount` directly, no fallback).
type wirePagedPlants struct {
	RowCount flexNumber  `json:"rowCount"`
	PageList []wirePlant `json:"pageList"`
}

// wirePlant is one PowerStationListItem (isolarTypes.ts:315-324).
// InstalledKw comes from installed_power (isolarTypes.ts:321,37 — the same
// field name appears on both PowerStationListItem and PlantInfo); the
// original round-1 field name "total_capacity" does not appear anywhere in
// the legacy source and is corrected here.
type wirePlant struct {
	PsID   flexString `json:"ps_id"`
	PsName flexString `json:"ps_name"`
	// A plain number in W, or {"value": "450", "unit": "kWp"} (arti-scada's
	// live-API parser); some responses name it install_power.
	InstalledPower json.RawMessage `json:"installed_power"`
	InstallPower   json.RawMessage `json:"install_power"`
}

// wirePagedDevices is result_data for getDeviceListByPsId
// (isolarTypes.ts:87-92, GetDeviceListByPsIdData). bcem-energy's OWN two
// call sites disagree on which shape is real: plants/route.ts:192-194
// explicitly comments "API returns pageList (not page_data)" and hedges
// both, while devices/route.ts:66,91 reads ONLY `page_data`/`row_count`
// (snake_case) with no fallback at all. This package therefore decodes
// BOTH spellings and prefers whichever is non-empty (plants.go), the same
// hedge wirePagedFaults already applies for the analogous getFaultAlarmInfo
// inconsistency — picking one over the other risks a silent empty result on
// a live response shaped the other way.
type wirePagedDevices struct {
	RowCountCamel flexNumber   `json:"rowCount"`
	RowCountSnake flexNumber   `json:"row_count"`
	PageListCamel []wireDevice `json:"pageList"`
	PageListSnake []wireDevice `json:"page_data"`
}

// wireDevice is one DeviceInfo (isolarTypes.ts:69-85); only the fields this
// task's Device type needs (ps_key, device_sn, device_type, device_name)
// are decoded.
type wireDevice struct {
	PsKey      flexString  `json:"ps_key"`
	DeviceSN   flexString  `json:"device_sn"`
	DeviceType *flexNumber `json:"device_type"`
	DeviceName *flexString `json:"device_name"`
}

// wirePoint is one minute-series sample for either a device (keyed by
// ps_key) or a plant (keyed by ps_id) — isolarTypes.ts:142-145
// (MinuteDataPoint: "time_stamp: string" plus dynamic "pXXXX" properties)
// and :216-219 (PlantMinuteDataPoint, same shape) — confirmed by
// isolarTypes.ts:104-117's DevicePointData comment: "Point values are
// returned directly as pXXXX properties (e.g., p13134, p83022)". The
// measurement points this package asks for (06 §6): 1 yield today (Wh), 24
// total active power (W), 2001 daily horizontal irradiation (Wh/m²), 2009
// ambient temperature (°C), 2010 module temperature (°C) — so the JSON keys
// are p1/p24/p2001/p2009/p2010, NOT round-1's invented "point_id_<n>". A
// field is a pointer so "", "null" and "-" all parse to nil via
// normalize.OptionalNumber (removed-behaviour 21: never a fabricated
// zero).
type wirePoint struct {
	TimeStamp  flexString  `json:"time_stamp"`
	Point1     *flexString `json:"p1"`
	Point24    *flexString `json:"p24"`
	Point83022 *flexString `json:"p83022"`
	Point83025 *flexString `json:"p83025"`
}

// wirePagedFaults is result_data for getFaultAlarmInfo. isolarTypes.ts:300-
// 303 declares GetFaultAlarmInfoData as "{row_count: number; page_data:
// FaultAlarmRecord[]}" (snake_case, required), but bcem-energy's own
// alarms/route.ts:148-149 and cron/alarm-check/route.ts:750 read BOTH
// `page_list`/`pageList` and `row_count`/`rowCount` defensively — direct
// evidence the real API is inconsistent across these two spellings for
// this one call. This package decodes both and prefers whichever is
// non-empty (alarms.go), rather than picking one and risking a silent
// empty result on a live response shaped the other way.
type wirePagedFaults struct {
	RowCountSnake flexNumber  `json:"row_count"`
	RowCountCamel flexNumber  `json:"rowCount"`
	PageListSnake []wireFault `json:"page_list"`
	PageListCamel []wireFault `json:"pageList"`
}

// wireFault is one fault/alarm row. isolarTypes.ts's FaultAlarmRecord
// (:275-298) declares fault_desc, not fault_name, and no separate
// "alarm_id" — but bcem-energy's actual call sites (alarms/route.ts:151-
// 166, cron/alarm-check/route.ts:757-798) read `fault_name` (never
// `fault_desc`, which one call site hardcodes to `null` as a placeholder —
// evidence it is never populated by a real response) and identify each row
// by `fault_code` (cron's own dedup key falls back to fault_code first:
// "item.fault_code || item.id || ..."). This package therefore uses
// fault_code/fault_name/create_time/ps_id/ps_key/device_sn — the
// empirically-confirmed shape — over isolarTypes.ts's stale declared one.
type wireFault struct {
	PsID       flexString  `json:"ps_id"`
	PsKey      *flexString `json:"ps_key"`
	DeviceSN   *flexString `json:"device_sn"`
	FaultCode  flexString  `json:"fault_code"`
	FaultName  flexString  `json:"fault_name"`
	CreateTime flexString  `json:"create_time"`
	// F9: level/type/device/close time for the alarms tab (R286).
	FaultLevel *flexNumber `json:"fault_level"`
	FaultType  *flexNumber `json:"fault_type"`
	DeviceName *flexString `json:"device_name"`
	OverTime   *flexString `json:"over_time"`
}

// wireDailyPoint is one day of getPowerStationPointDayMonthYearDataList; the
// value sits under the requested data_type key ("2"), as legacy reads it.
type wireDailyPoint map[string]json.RawMessage

// wireRealtime is getDeviceRealTimeData's result_data.
type wireRealtime struct {
	DevicePointList []struct {
		DevicePoint map[string]json.RawMessage `json:"device_point"`
	} `json:"device_point_list"`
}
