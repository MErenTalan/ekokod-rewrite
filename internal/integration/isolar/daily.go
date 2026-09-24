package isolar

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

const (
	opGetPowerStationPointDayMonthYearDataList = "get_power_station_point_day_month_year_data_list"
	// Legacy's historical route: query_type "1" = day, data_type "2", point
	// p83022 (daily yield, Wh). Unverified against a live account (Q-F3).
	dailyQueryType = "1"
	dailyDataType  = "2"
	dailyPoint     = "p83022"
	dayLayout      = "20060102"
)

// DailyYield is one Istanbul day's plant yield; Day is local midnight.
type DailyYield struct {
	Day time.Time
	Kwh *decimal.Decimal
}

// PlantDailySeries fetches end-of-day plant yields for the Istanbul days in
// [from, to), at most MaxWindowDay; the request's end_time is inclusive.
func (c *Client) PlantDailySeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]DailyYield, error) {
	if psID == "" {
		return nil, c.configError(opGetPowerStationPointDayMonthYearDataList)
	}
	if err := checkMaxWindow(from, to, MaxWindowDay); err != nil {
		return nil, err
	}
	loc := normalize.Istanbul
	last := to.Add(-time.Nanosecond).In(loc)
	raw, err := c.call(ctx, creds, callOptions{
		op:     opGetPowerStationPointDayMonthYearDataList,
		bearer: true,
		body: map[string]any{
			"query_type":        dailyQueryType,
			"data_type":         dailyDataType,
			"ps_id_list":        []string{psID},
			"data_point":        dailyPoint,
			"start_time":        from.In(loc).Format(dayLayout),
			"end_time":          last.Format(dayLayout),
			"order":             "0",
			"is_get_point_dict": "1",
		},
	})
	if err != nil {
		return nil, err
	}
	var byPlant map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byPlant); err != nil {
		return nil, malformedErr(opGetPowerStationPointDayMonthYearDataList)
	}
	var points map[string][]wireDailyPoint
	if body, ok := byPlant[psID]; ok {
		if err := json.Unmarshal(body, &points); err != nil {
			return nil, malformedErr(opGetPowerStationPointDayMonthYearDataList)
		}
	}
	var out []DailyYield
	for _, p := range points[dailyPoint] {
		stamp := rawText(p["time_stamp"])
		day, err := normalize.LocalLayout(dayLayout, stamp)
		if err != nil || day.Before(from) || !day.Before(to) {
			continue // adapter-patterns.md item 7: skip one bad row
		}
		wh, err := normalize.OptionalNumber(rawText(p[dailyDataType]))
		if err != nil {
			continue
		}
		out = append(out, DailyYield{Day: day.In(loc), Kwh: normalize.WhToKWh(wh)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

// rawText reads a JSON string or number as text; null or absent is "".
func rawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
