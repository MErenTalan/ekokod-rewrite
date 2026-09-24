package legacy

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var alarmTypes = map[string]string{
	"reactive limit detection alarm": "reactive_limit", "data communication alarm": "data_communication",
	"current - voltage - power alarm": "current_voltage_power", "invoice alarm": "invoice_increase", "fatura alarmi": "invoice_increase",
}

func periodUnit(s string) any {
	if s == "hours" || s == "days" {
		return s
	}
	return nil
}

// alarmsStep is R425.
func (t *transformer) alarmsStep() error {
	return ReadExtract(t.dir, "alarms", func(a bson.M) error {
		t.count("alarms")
		hex := hexID(a["_id"])
		typ, ok := alarmTypes[fold(str(a, "type"))]
		if !ok {
			t.rj.Reject("alarms", hex, "type", "unknown_type", str(a, "type"))
			return nil
		}
		companies := map[string]bool{}
		for _, v := range asArray(a["analyzers"]) {
			if b, known := t.analyzerBuilding[hexID(v)]; known {
				companies[t.buildingCompany[b]] = true
			}
		}
		if len(companies) != 1 {
			reason := "company_unknown"
			if len(companies) > 1 {
				reason = "company_ambiguous"
			}
			t.rj.Reject("alarms", hex, "analyzers", reason, len(companies))
			return nil
		}
		var company string
		for c := range companies {
			company = c
		}
		s := asDoc(a["settings"])
		n := func(key string) any {
			v, err := num(s[key])
			if err != nil {
				return nil
			}
			return dec(v)
		}
		enabled := true
		if e, ok := a["isEnable"].(bool); ok {
			enabled = e
		}
		if n("voltageMax") != nil || n("voltageMin") != nil { // F7: a power-only alarm
			if err := t.note("alarms", hex, "voltage_dropped", map[string]any{"max": n("voltageMax"), "min": n("voltageMin")}); err != nil {
				return err
			}
			if typ == "current_voltage_power" && n("powerMax") == nil && n("powerMin") == nil && enabled {
				enabled = false
				if err := t.note("alarms", hex, "alarm_disabled_voltage_only", str(a, "name")); err != nil {
					return err
				}
			}
		}
		id := ID("alarms", hex)
		t.rj.Accept("alarms")
		if err := t.w.row("alarms", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "name": strings.TrimSpace(str(a, "name")),
			"type": typ, "is_enabled": enabled,
			"inductive_ratio_threshold": n("inductiveRatioThreshold"), "inductive_period_value": intOrNil(s["inductiveRatioPeriodValue"]),
			"inductive_period_unit": periodUnit(str(s, "inductiveRatioPeriodUnit")), "capacitive_ratio_threshold": n("capacitiveRatioThreshold"),
			"capacitive_period_value": intOrNil(s["capacitiveRatioPeriodValue"]), "capacitive_period_unit": periodUnit(str(s, "capacitiveRatioPeriodUnit")),
			"active_consumption_max": n("activeConsumptionMax"), "active_consumption_max_period_value": intOrNil(s["activeConsumptionMaxPeriodValue"]),
			"active_consumption_max_period_unit": periodUnit(str(s, "activeConsumptionMaxPeriodUnit")), "active_consumption_min": n("activeConsumptionMin"),
			"active_consumption_min_period_value": intOrNil(s["activeConsumptionMinPeriodValue"]),
			"active_consumption_min_period_unit":  periodUnit(str(s, "activeConsumptionMinPeriodUnit")),
			"communication_threshold_hours":       intOrNil(s["communicationThresholdHours"]), "power_max": n("powerMax"), "power_min": n("powerMin"),
			"invoice_threshold_pct": n("invoiceThresholdPercentage"), "notification_frequency_value": intOrNil(s["notificationFrequencyValue"]),
			"notification_frequency_unit": periodUnit(str(s, "notificationFrequencyUnit")),
			"created_at":                  ts(bdate(a["createdAt"])), "updated_at": ts(bdate(a["updatedAt"]))}); err != nil {
			return err
		}
		if err := t.w.legacyID("alarms", hex, "alarms", id); err != nil {
			return err
		}
		for _, v := range asArray(a["analyzers"]) {
			t.count("alarm_analyzers")
			ah := hexID(v)
			if _, known := t.analyzerBuilding[ah]; !known {
				t.rj.Reject("alarm_analyzers", hex+":"+ah, "analyzer", "analyzer_unknown", ah)
				continue
			}
			t.rj.Accept("alarm_analyzers")
			if err := t.w.row("alarm_analyzers", map[string]any{"alarm_id": id.String(), "analyzer_id": ID("analyzers", ah).String()}); err != nil {
				return err
			}
		}
		if err := t.alarmChannels(hex, id.String(), s); err != nil {
			return err
		}
		for i, v := range asArray(a["logs"]) {
			t.count("alarm_events")
			ref := fmt.Sprintf("%s:%d", hex, i)
			l := asDoc(v)
			at := bdate(l["timestamp"])
			if at.IsZero() || strings.TrimSpace(str(l, "message")) == "" {
				t.rj.Reject("alarm_events", ref, "", "required", nil)
				continue
			}
			var detail any
			if d := str(l, "details"); d != "" {
				b, err := json.Marshal(map[string]string{"details": d})
				if err != nil {
					return err
				}
				detail = json.RawMessage(b)
			}
			t.rj.Accept("alarm_events")
			if err := t.w.row("alarm_events", map[string]any{"id": ID("alarm_events", ref).String(), "alarm_id": id.String(),
				"triggered_at": ts(at), "message": str(l, "message"), "detail": detail}); err != nil {
				return err
			}
		}
		for _, v := range asArray(a["alarmTriggeredBillIds"]) { // Q-J11
			t.count("alarm_fired_bills")
			b, _ := v.(string)
			t.rj.Reject("alarm_fired_bills", hex+":"+b, "bill", "bill_recomputed", nil)
		}
		return nil
	})
}

func (t *transformer) alarmChannels(hex, id string, s bson.M) error {
	seen := map[string]bool{}
	for _, ch := range []struct{ channel, key string }{{"email", "emailRecipients"}, {"sms", "smsNumbers"}} {
		for i, v := range asArray(s[ch.key]) {
			t.count("alarm_channels")
			ref := fmt.Sprintf("%s:%s:%d", hex, ch.channel, i)
			target, _ := v.(string)
			target = strings.TrimSpace(target)
			if ch.channel == "email" {
				target = strings.ToLower(target)
			}
			switch {
			case target == "" || (ch.channel == "email" && !strings.Contains(target, "@")):
				t.rj.Reject("alarm_channels", ref, ch.key, "invalid", target)
				continue
			case seen[ch.channel+":"+target]:
				t.rj.Reject("alarm_channels", ref, ch.key, "duplicate", target)
				continue
			}
			seen[ch.channel+":"+target] = true
			t.rj.Accept("alarm_channels")
			if err := t.w.row("alarm_channels", map[string]any{"alarm_id": id, "channel": ch.channel, "target": target}); err != nil {
				return err
			}
		}
	}
	return nil
}
