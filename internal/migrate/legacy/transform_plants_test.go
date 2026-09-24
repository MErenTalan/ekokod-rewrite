package legacy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

const (
	plantHex  = "64f000000000000000007a01"
	plant2Hex = "64f000000000000000007a02"
	alarmHex  = "64f000000000000000008a01"
)

// fullSource is every collection the transform reads.
func fullSource(t *testing.T) *legacy.MemSource {
	src := historySource(t)
	day := func(d int) string { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC).Format(time.RFC3339) }
	src.Add("powerplants",
		bson.D{{Key: "_id", Value: oid(plantHex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "santral_ismi", Value: "Arazi GES"},
			{Key: "panellerin_yonu", Value: "güney"}, {Key: "pv_sayisi", Value: int32(1000)}, {Key: "lat", Value: 0.0}, {Key: "isolar_linked", Value: true},
			{Key: "monthly_target_production", Value: bson.D{{Key: "ocak", Value: 1000.0}, {Key: "aralik", Value: 900.0}}},
			{Key: "inverters", Value: bson.A{bson.D{{Key: "inverter_id", Value: "INV-1"}, {Key: "marka", Value: "Sungrow"}, {Key: "guc", Value: 50.0}}}},
			{Key: "isolar_devices", Value: bson.A{
				bson.D{{Key: "device_sn", Value: "INV-1"}, {Key: "device_name", Value: "Inverter 1"}, {Key: "device_type", Value: int32(1)}, {Key: "ps_key", Value: "k1"}},
				bson.D{{Key: "device_sn", Value: ""}}}},
			{Key: "isolar_alarm_email_recipients", Value: bson.A{"Ops@Acme.test", "ops@acme.test", "nope"}},
			{Key: "solarTariffs", Value: bson.A{bson.D{{Key: "effectiveFrom", Value: "01-01-2026"}, {Key: "feedInTariff", Value: 2.0}, {Key: "currency", Value: "tl"}}}},
			{Key: "santral_values", Value: bson.D{
				{Key: "daily", Value: bson.A{bson.D{{Key: "timestamp", Value: day(1)}, {Key: "production_kwh", Value: 300.0}}}},
				{Key: "hourly", Value: bson.A{
					bson.D{{Key: "timestamp", Value: "2026-08-31T22:30:00Z"}, {Key: "production_kwh", Value: 10.0}}, // 1 Sep in Istanbul: the daily total wins
					bson.D{{Key: "timestamp", Value: "2026-09-02T09:00:00Z"}, {Key: "production_kwh", Value: 40.0}, {Key: "instant_power_kw", Value: 45.0}}}},
				{Key: "monthly", Value: bson.A{bson.D{{Key: "timestamp", Value: day(1)}, {Key: "production_kwh", Value: 9000.0}}}}}}},
		bson.D{{Key: "_id", Value: oid(plant2Hex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "santral_ismi", Value: "Merkez Çatı GES"}},
	)
	src.Add("tariffs",
		bson.D{{Key: "_id", Value: oid("64f000000000000000001b01")}, {Key: "energy_source", Value: "solar"}, {Key: "powerPlant", Value: oid(plantHex)},
			{Key: "effectiveFrom", Value: "01-01-2026"}, {Key: "feed_in_tariff", Value: 2.1}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000001b02")}, {Key: "energy_source", Value: "solar"}, {Key: "powerPlant", Value: oid(plant2Hex)},
			{Key: "effectiveFrom", Value: "01-06-2026"}, {Key: "feed_in_tariff", Value: 1.9}, {Key: "currency", Value: "tl"}},
	)
	src.Add("alarms",
		bson.D{{Key: "_id", Value: oid(alarmHex)}, {Key: "name", Value: "Güç"}, {Key: "type", Value: "Current - Voltage - Power Alarm"}, {Key: "isEnable", Value: true},
			{Key: "analyzers", Value: bson.A{oid("64f0000000000000000000a1"), oid("64f0000000000000000000ff")}},
			{Key: "settings", Value: bson.D{{Key: "voltageMax", Value: 240.0}, {Key: "notificationFrequencyUnit", Value: "hours"}, {Key: "notificationFrequencyValue", Value: int32(6)},
				{Key: "emailRecipients", Value: bson.A{"Enerji@Acme.test", "enerji@acme.test"}}, {Key: "smsNumbers", Value: bson.A{"05551112233"}}}},
			{Key: "logs", Value: bson.A{bson.D{{Key: "timestamp", Value: bson.NewDateTimeFromTime(now.Add(-time.Hour))}, {Key: "message", Value: "Güç eşiği aşıldı"}}}},
			{Key: "alarmTriggeredBillIds", Value: bson.A{"b1"}}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000008a02")}, {Key: "name", Value: "X"}, {Key: "type", Value: "Voltage Alarm"}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000008a03")}, {Key: "name", Value: "Y"}, {Key: "type", Value: "Data Communication Alarm"},
			{Key: "analyzers", Value: bson.A{oid("64f0000000000000000000ff")}}},
	)
	return src
}

func TestPlantsAndTheirProduction(t *testing.T) {
	out, res, _ := transformWith(t, fullSource(t), nil)
	plants := rows(t, out, "power_plants")
	require.Len(t, plants, 2)
	p := plants[0]
	require.Equal(t, []any{"Arazi GES", "grid", "s", float64(1000), nil}, []any{p["name"], p["plant_kind"], p["orientation"], p["panel_count"], p["latitude"]})
	require.Nil(t, p["isolar_credential_id"], "the fixture company has no iSolar credential")
	require.Equal(t, 1, res.Notes["plant_kind_review"], "the 'çatı' plant is flagged, never reclassified")

	require.Len(t, rows(t, out, "power_plant_monthly_targets"), 2)
	devices := rows(t, out, "power_plant_devices")
	require.Len(t, devices, 1, "the inverter and its iSolar device are one serial")
	require.Equal(t, []any{"Sungrow", "Inverter 1", "k1", "50"}, []any{devices[0]["brand"], devices[0]["device_name"], devices[0]["provider_key"], devices[0]["rated_power_kw"]})
	require.Equal(t, []map[string]any{{"plant_id": p["id"], "email": "ops@acme.test"}}, rows(t, out, "power_plant_alarm_recipients"))

	solar := rows(t, out, "solar_tariffs")
	require.Len(t, solar, 2, "the plant's own 2026-01-01 record wins over the standalone one; plant 2's standalone record is kept")
	require.Equal(t, []any{"2026-01-01", "2", "TRY"}, []any{solar[0]["effective_from"], solar[0]["feed_in_tariff"], solar[0]["currency"]})
	reasons := rejectReasons(t, out)
	require.Contains(t, reasons["duplicate_merged"], "64f000000000000000001b01")

	prod := rows(t, out, "plant_production_totals")
	require.Equal(t, []any{"2026-08-31T21:00:00Z", "300", "daily_total"}, []any{prod[0]["ts"], prod[0]["production_kwh"], prod[0]["basis"]}, "a day's total at local midnight")
	require.Equal(t, []any{"2026-09-02T09:00:00Z", "40", "plant_meter", "45"}, []any{prod[1]["ts"], prod[1]["production_kwh"], prod[1]["basis"], prod[1]["active_power_kw"]})
	require.Len(t, prod, 2)
	require.Len(t, reasons["monthly_rollup"], 1, "Q-J12")
	require.Len(t, reasons["day_has_daily_total"], 1, "R278: a day is never merged")
}

func TestAlarmsKeepTheirCompanyAnalyzersAndChannels(t *testing.T) {
	out, res, _ := transformWith(t, fullSource(t), nil)
	alarms := rows(t, out, "alarms")
	require.Len(t, alarms, 1)
	a := alarms[0]
	require.Equal(t, []any{"current_voltage_power", false, "hours", float64(6)}, []any{a["type"], a["is_enabled"], a["notification_frequency_unit"], a["notification_frequency_value"]},
		"a voltage-only alarm has no threshold left after F7: migrated disabled")
	require.Nil(t, a["voltage_max"])
	require.Equal(t, 1, res.Notes["voltage_dropped"])
	require.Equal(t, 1, res.Notes["alarm_disabled_voltage_only"])
	require.Len(t, rows(t, out, "alarm_analyzers"), 1)
	require.Equal(t, []map[string]any{{"alarm_id": a["id"], "channel": "email", "target": "enerji@acme.test"}, {"alarm_id": a["id"], "channel": "sms", "target": "05551112233"}},
		rows(t, out, "alarm_channels"))
	require.Equal(t, "Güç eşiği aşıldı", rows(t, out, "alarm_events")[0]["message"])
	reasons := rejectReasons(t, out)
	require.Equal(t, []string{"64f000000000000000008a02"}, reasons["unknown_type"])
	require.Contains(t, reasons["company_unknown"], "64f000000000000000008a03")
	require.Equal(t, []string{alarmHex + ":b1"}, reasons["bill_recomputed"], "Q-J11")
}
