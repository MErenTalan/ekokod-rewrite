package isolar

import (
	"context"
	"encoding/json"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/shopspring/decimal"
)

// opQueryPowerStationList / opGetDeviceListByPsID are
// integration_definitions.json's isolar row keys (R40) — the source of
// truth is internal/seed/data/integration_definitions.json, read verbatim
// as creds.Endpoints; this package never hard-codes a path.
const (
	opQueryPowerStationList = "query_power_station_list"
	opGetDeviceListByPsID   = "get_device_list_by_ps_id"
)

// plantsPageSize / devicesPageSize are the brief's stated page sizes:
// "page/size 50, follows rowCount" for Plants, "page/size 100, pageList"
// for Devices.
const (
	plantsPageSize  = 50
	devicesPageSize = 100
)

// Plant is one power station as queryPowerStationList reports it.
type Plant struct {
	PSID        string
	Name        string
	InstalledKw *decimal.Decimal
}

// Device is one inverter/logger under a plant, as getDeviceListByPsId
// reports it.
type Device struct {
	PSKey      string
	DeviceSN   string
	DeviceType *int32
	DeviceName *string
}

// Plants lists every power station the credential can see, following
// rowCount across pages of plantsPageSize. R44: pages are derived from
// rowCount against a hard budget (maxPageBudget) — exceeding it is a
// non-retryable *integration.Error, never a silently truncated list; the
// rowCount comparison counts every row this call has RECEIVED so far
// (received, incremented per raw page length), not only the ones that
// mapped cleanly, so a page containing one bad row cannot make this loop
// think there is more data left than there really is.
func (c *Client) Plants(ctx context.Context, creds integration.Credentials) ([]Plant, error) {
	var out []Plant
	received := 0
	for page := 1; ; page++ {
		if page > maxPageBudget {
			return nil, c.pageBudgetExceeded(opQueryPowerStationList)
		}
		raw, err := c.call(ctx, creds, callOptions{
			op:     opQueryPowerStationList,
			bearer: true,
			body:   map[string]any{"page": page, "size": plantsPageSize},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedPlants
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opQueryPowerStationList)
		}
		received += len(pd.PageList)
		for _, wp := range pd.PageList {
			p, mapErr := mapPlant(wp)
			if mapErr != nil {
				// adapter-patterns.md item 7: one bad row is skipped, not
				// a whole-page failure. Plant/Device rows have no caller
				// facing warning channel in this package's API (unlike
				// integration.FetchResult.Warnings) — see the task report
				// for that scope note.
				continue
			}
			out = append(out, p)
		}
		rowCount, rcErr := pd.RowCount.Int64()
		if rcErr != nil || received >= int(rowCount) || len(pd.PageList) == 0 {
			break
		}
	}
	return out, nil
}

func mapPlant(w wirePlant) (Plant, error) {
	if w.PsID == "" {
		return Plant{}, malformedErr(opQueryPowerStationList)
	}
	kw, err := normalize.OptionalNumber(derefOr(w.InstalledPower, ""))
	if err != nil {
		return Plant{}, err
	}
	return Plant{PSID: w.PsID, Name: w.PsName, InstalledKw: kw}, nil
}

// Devices lists every device under plant psID, following rowCount across
// pages of devicesPageSize — same R44 page-budget/received-count rule as
// Plants. psID is refused empty before any call (adapter-patterns.md item
// 12).
func (c *Client) Devices(ctx context.Context, creds integration.Credentials, psID string) ([]Device, error) {
	if psID == "" {
		return nil, c.configError(opGetDeviceListByPsID)
	}
	var out []Device
	received := 0
	for page := 1; ; page++ {
		if page > maxPageBudget {
			return nil, c.pageBudgetExceeded(opGetDeviceListByPsID)
		}
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetDeviceListByPsID,
			bearer: true,
			body: map[string]any{
				"ps_id":                   psID,
				"page":                    page,
				"size":                    devicesPageSize,
				"is_virtual_unit":         "0",
				"is_get_firmware_version": "0",
				"device_type_list":        []int{},
			},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedDevices
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opGetDeviceListByPsID)
		}
		pageList := pd.PageListCamel
		if len(pageList) == 0 {
			pageList = pd.PageListSnake
		}
		received += len(pageList)
		for _, wd := range pageList {
			d, mapErr := mapDevice(wd)
			if mapErr != nil {
				continue
			}
			out = append(out, d)
		}
		rowCount, rcErr := devicesRowCount(pd)
		if rcErr != nil || received >= int(rowCount) || len(pageList) == 0 {
			break
		}
	}
	return out, nil
}

// devicesRowCount prefers rowCount (camelCase), falling back to row_count
// when rowCount's json.Number is empty (the key was absent) — see wire.go's
// wirePagedDevices doc for why this call hedges both spellings.
func devicesRowCount(pd wirePagedDevices) (int64, error) {
	if pd.RowCountCamel.String() != "" {
		return pd.RowCountCamel.Int64()
	}
	return pd.RowCountSnake.Int64()
}

func mapDevice(w wireDevice) (Device, error) {
	if w.PsKey == "" || w.DeviceSN == "" {
		return Device{}, malformedErr(opGetDeviceListByPsID)
	}
	var deviceType *int32
	if w.DeviceType != nil {
		n, err := w.DeviceType.Int64()
		if err != nil {
			return Device{}, malformedErr(opGetDeviceListByPsID)
		}
		v := int32(n)
		deviceType = &v
	}
	return Device{PSKey: w.PsKey, DeviceSN: w.DeviceSN, DeviceType: deviceType, DeviceName: w.DeviceName}, nil
}

// derefOr returns *p, or fallback when p is nil.
func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
