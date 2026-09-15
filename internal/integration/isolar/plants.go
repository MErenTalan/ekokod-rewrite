package isolar

import (
	"context"
	"encoding/json"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/shopspring/decimal"
)

// opQueryPowerStationList / opGetDeviceListByPsId are 06 §6's operation
// names, verbatim, used as both this file's creds.Endpoints keys and the
// httpx.Request.Op recorded on every error from these calls.
const (
	opQueryPowerStationList = "queryPowerStationList"
	opGetDeviceListByPsId   = "getDeviceListByPsId" //nolint:stylecheck // 06 §6's own operation name, verbatim
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
// rowCount across pages of plantsPageSize (bounded by maxPages).
func (c *Client) Plants(ctx context.Context, creds integration.Credentials) ([]Plant, error) {
	var out []Plant
	for page := 1; page <= maxPages; page++ {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opQueryPowerStationList,
			bearer: true,
			body:   map[string]any{"curPage": page, "size": plantsPageSize},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedPlants
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opQueryPowerStationList)
		}
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
		if rcErr != nil || len(out) >= int(rowCount) || len(pd.PageList) == 0 {
			break
		}
	}
	return out, nil
}

func mapPlant(w wirePlant) (Plant, error) {
	if w.PsID == "" {
		return Plant{}, malformedErr(opQueryPowerStationList)
	}
	kw, err := normalize.OptionalNumber(derefOr(w.TotalCapacity, ""))
	if err != nil {
		return Plant{}, err
	}
	return Plant{PSID: w.PsID, Name: w.PsName, InstalledKw: kw}, nil
}

// Devices lists every device under plant psID, following rowCount across
// pages of devicesPageSize (bounded by maxPages).
func (c *Client) Devices(ctx context.Context, creds integration.Credentials, psID string) ([]Device, error) {
	var out []Device
	for page := 1; page <= maxPages; page++ {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetDeviceListByPsId,
			bearer: true,
			body:   map[string]any{"ps_id": psID, "curPage": page, "size": devicesPageSize},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedDevices
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opGetDeviceListByPsId)
		}
		for _, wd := range pd.PageList {
			d, mapErr := mapDevice(wd)
			if mapErr != nil {
				continue
			}
			out = append(out, d)
		}
		rowCount, rcErr := pd.RowCount.Int64()
		if rcErr != nil || len(out) >= int(rowCount) || len(pd.PageList) == 0 {
			break
		}
	}
	return out, nil
}

func mapDevice(w wireDevice) (Device, error) {
	if w.PsKey == "" || w.DeviceSN == "" {
		return Device{}, malformedErr(opGetDeviceListByPsId)
	}
	var deviceType *int32
	if w.DeviceType != nil {
		n, err := w.DeviceType.Int64()
		if err != nil {
			return Device{}, malformedErr(opGetDeviceListByPsId)
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
