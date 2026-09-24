package legacy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	orientations = map[string]string{"kuzey": "n", "guney": "s", "dogu": "e", "bati": "w", "kd": "ne", "gd": "se", "kb": "nw", "gb": "sw"}
	targetMonths = []string{"ocak", "subat", "mart", "nisan", "mayis", "haziran", "temmuz", "agustos", "eylul", "ekim", "kasim", "aralik"}
)

func intOrNil(v any) any {
	n, err := num(v)
	if err != nil || n == nil {
		return nil
	}
	return n.IntPart()
}

func zeroNil(v any) any {
	n, err := num(v)
	if err != nil || n == nil || n.IsZero() {
		return nil
	}
	return n.String()
}

// plantsStep is R424 with Q-J12's production rules.
func (t *transformer) plantsStep() error {
	plants := map[string]string{} // plant hex → company hex
	if err := ReadExtract(t.dir, "powerplants", func(p bson.M) error {
		t.count("powerplants")
		hex, company := hexID(p["_id"]), hexID(p["company_id"])
		name := strings.TrimSpace(str(p, "santral_ismi"))
		switch {
		case !t.companies[company]:
			t.rj.Reject("powerplants", hex, "company_id", "company_unknown", company)
			return nil
		case name == "":
			t.rj.Reject("powerplants", hex, "santral_ismi", "required", nil)
			return nil
		}
		id := ID("powerplants", hex)
		plants[hex] = company
		if strings.Contains(fold(name), "cati") {
			if err := t.note("powerplants", hex, "plant_kind_review", name); err != nil {
				return err
			}
		}
		var credential, installed any
		if linked, _ := p["isolar_linked"].(bool); linked {
			if c, ok := t.isolarCredential[company]; ok {
				credential = c
			}
		}
		if d := bdate(p["installation_date"]); !d.IsZero() {
			installed = civilDate(d)
		}
		n := func(key string) any {
			v, err := num(p[key])
			if err != nil {
				return nil
			}
			return dec(v)
		}
		var orientation any
		if o, ok := orientations[fold(str(p, "panellerin_yonu"))]; ok {
			orientation = o
		}
		t.rj.Accept("powerplants")
		if err := t.w.row("power_plants", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "name": name,
			"installation_number": strOrNil(str(p, "tesisat_no")), "plant_kind": "grid", "pv_brand_model": strOrNil(str(p, "pv_marka_ve_modeli")),
			"panel_power_w": n("panel_gucu"), "panel_efficiency_pct": n("panel_verimi"), "panel_count": intOrNil(p["pv_sayisi"]),
			"string_count": intOrNil(p["string_sayisi"]), "orientation": orientation, "tilt_angle_deg": n("panelin_zemin_acisi"),
			"total_capacity_kw": n("total_capacity_kw"), "yearly_target_kwh": n("yearly_target_production"), "installation_date": installed,
			"address": strOrNil(str(p, "address")), "latitude": zeroNil(p["lat"]), "longitude": zeroNil(p["long"]),
			"isolar_ps_id": strOrNil(str(p, "isolar_ps_id")), "isolar_ps_key": strOrNil(str(p, "isolar_ps_key")), "isolar_ps_name": strOrNil(str(p, "isolar_ps_name")),
			"isolar_installed_kw": n("isolar_installed_power"), "isolar_linked_at": ts(bdate(p["isolar_linked_at"])), "isolar_credential_id": credential,
			"created_at": ts(bdate(p["createdAt"])), "updated_at": ts(bdate(p["updatedAt"]))}); err != nil {
			return err
		}
		if err := t.w.legacyID("powerplants", hex, "power_plants", id); err != nil {
			return err
		}
		targets := asDoc(p["monthly_target_production"])
		for i, m := range targetMonths {
			v, err := num(targets[m])
			if err != nil || v == nil {
				continue
			}
			if err := t.w.row("power_plant_monthly_targets", map[string]any{"plant_id": id.String(), "month": i + 1, "target_kwh": v.String()}); err != nil {
				return err
			}
		}
		if err := t.plantDevices(hex, p); err != nil {
			return err
		}
		if err := t.plantRecipients(hex, p); err != nil {
			return err
		}
		if err := t.plantSolarTariffs(hex, company, p); err != nil {
			return err
		}
		return t.plantProduction(hex, asDoc(p["santral_values"]))
	}); err != nil {
		return err
	}
	return t.standaloneSolarTariffs(plants)
}

func (t *transformer) plantDevices(hex string, p bson.M) error {
	devices := map[string]map[string]any{}
	add := func(sn string, fields map[string]any) {
		d, ok := devices[sn]
		if !ok {
			d = map[string]any{"id": ID("power_plant_devices", hex+":"+sn).String(), "plant_id": ID("powerplants", hex).String(), "device_sn": sn}
			devices[sn] = d
		}
		for k, v := range fields {
			if v != nil {
				d[k] = v
			}
		}
	}
	for i, v := range asArray(p["inverters"]) {
		t.count("plant_devices")
		inv := asDoc(v)
		sn := strings.TrimSpace(str(inv, "inverter_id"))
		if sn == "" {
			t.rj.Reject("plant_devices", fmt.Sprintf("%s:inverters:%d", hex, i), "inverter_id", "required", nil)
			continue
		}
		t.rj.Accept("plant_devices")
		add(sn, map[string]any{"brand": strOrNil(str(inv, "marka")), "model": strOrNil(str(inv, "model")), "rated_power_kw": zeroNil(inv["guc"]),
			"status": strOrNil(str(inv, "status")), "efficiency_pct": zeroNil(inv["efficiency"])})
	}
	for i, v := range asArray(p["isolar_devices"]) {
		t.count("plant_devices")
		dev := asDoc(v)
		sn := strings.TrimSpace(str(dev, "device_sn"))
		if sn == "" {
			t.rj.Reject("plant_devices", fmt.Sprintf("%s:isolar_devices:%d", hex, i), "device_sn", "required", nil)
			continue
		}
		t.rj.Accept("plant_devices")
		add(sn, map[string]any{"device_name": strOrNil(str(dev, "device_name")), "device_type": intOrNil(dev["device_type"]),
			"device_type_name": strOrNil(str(dev, "device_type_name")), "provider_key": strOrNil(str(dev, "ps_key"))})
	}
	sns := make([]string, 0, len(devices))
	for sn := range devices {
		sns = append(sns, sn)
	}
	sort.Strings(sns)
	for _, sn := range sns {
		if err := t.w.row("power_plant_devices", devices[sn]); err != nil {
			return err
		}
	}
	return nil
}

func (t *transformer) plantRecipients(hex string, p bson.M) error {
	seen := map[string]bool{}
	for i, v := range asArray(p["isolar_alarm_email_recipients"]) {
		t.count("plant_alarm_recipients")
		ref := fmt.Sprintf("%s:%d", hex, i)
		email, _ := v.(string)
		email = strings.ToLower(strings.TrimSpace(email))
		switch {
		case !strings.Contains(email, "@"):
			t.rj.Reject("plant_alarm_recipients", ref, "email", "invalid_email", email)
			continue
		case seen[email]:
			t.rj.Reject("plant_alarm_recipients", ref, "email", "duplicate", email)
			continue
		}
		seen[email] = true
		t.rj.Accept("plant_alarm_recipients")
		if err := t.w.row("power_plant_alarm_recipients", map[string]any{"plant_id": ID("powerplants", hex).String(), "email": email}); err != nil {
			return err
		}
	}
	return nil
}

func (t *transformer) solarTariffRow(id, company, plant string, from time.Time, feedIn *decimal.Decimal, purchase any, currency, notes string, created time.Time) map[string]any {
	cur, ok := currencis[fold(currency)]
	if !ok {
		cur = "TRY" // legacy's schema default is "tl"
	}
	return map[string]any{"id": id, "company_id": ID("companies", company).String(), "plant_id": ID("powerplants", plant).String(),
		"effective_from": civilDate(from), "feed_in_tariff": feedIn.String(), "purchase_price": purchase, "currency": cur,
		"notes": strOrNil(notes), "created_at": ts(created)}
}

// plantSolarTariffs writes a plant's own tariff history; its dates win over standalone records.
func (t *transformer) plantSolarTariffs(hex, company string, p bson.M) error {
	if t.solarDates == nil {
		t.solarDates = map[string]bool{}
	}
	for i, v := range asArray(p["solarTariffs"]) {
		t.count("plant_solar_tariffs")
		ref := fmt.Sprintf("%s:%d", hex, i)
		st := asDoc(v)
		from, err := ParseDate(str(st, "effectiveFrom"))
		if err != nil {
			t.rj.Reject("plant_solar_tariffs", ref, "effectiveFrom", reasonOf(err), str(st, "effectiveFrom"))
			continue
		}
		feedIn, _ := num(st["feedInTariff"])
		if feedIn == nil || !feedIn.IsPositive() {
			t.rj.Reject("plant_solar_tariffs", ref, "feedInTariff", "no_feed_in_tariff", nil)
			continue
		}
		purchase, _ := num(st["purchasePrice"])
		t.solarDates[hex+":"+civilDate(from)] = true
		t.rj.Accept("plant_solar_tariffs")
		if err := t.w.row("solar_tariffs", t.solarTariffRow(ID("plant_solar_tariffs", ref).String(), company, hex, from, feedIn, dec(purchase),
			str(st, "currency"), str(st, "notes"), bdate(st["createdAt"]))); err != nil {
			return err
		}
	}
	return nil
}

// standaloneSolarTariffs accounts the standalone `tariffs` records set aside by tariffsStep.
func (t *transformer) standaloneSolarTariffs(plants map[string]string) error {
	for _, d := range t.solarTariffDocs {
		hex, plant := hexID(d["_id"]), hexID(d["powerPlant"])
		company, ok := plants[plant]
		if !ok {
			t.rj.Reject("tariffs", hex, "powerPlant", "plant_unknown", plant)
			continue
		}
		from, err := ParseDate(str(d, "effectiveFrom"))
		if err != nil {
			t.rj.Reject("tariffs", hex, "effectiveFrom", reasonOf(err), str(d, "effectiveFrom"))
			continue
		}
		feedIn, _ := num(d["feed_in_tariff"])
		switch {
		case feedIn == nil || !feedIn.IsPositive():
			t.rj.Reject("tariffs", hex, "feed_in_tariff", "no_feed_in_tariff", nil)
			continue
		case t.solarDates[plant+":"+civilDate(from)]:
			t.rj.Reject("tariffs", hex, "effectiveFrom", "duplicate_merged", plant)
			continue
		}
		id := ID("tariffs", hex)
		t.rj.Accept("tariffs")
		if err := t.w.row("solar_tariffs", t.solarTariffRow(id.String(), company, plant, from, feedIn, nil, str(d, "currency"), "", bdate(d["createdAt"]))); err != nil {
			return err
		}
		if err := t.w.legacyID("tariffs", hex, "solar_tariffs", id); err != nil {
			return err
		}
	}
	return nil
}

// plantProduction is Q-J12: a day comes from its daily total, else its hourly
// rows; monthly roll-ups would double count next to them (R276, R278).
func (t *transformer) plantProduction(hex string, values bson.M) error {
	type row struct {
		at         time.Time
		kwh, power any
		basis, ref string
	}
	rows := map[time.Time]row{}
	daily := map[string]bool{}
	for i := range asArray(values["monthly"]) {
		t.count("plant_production")
		t.rj.Reject("plant_production", fmt.Sprintf("%s:monthly:%d", hex, i), "monthly", "monthly_rollup", nil)
	}
	for _, kind := range []string{"daily", "hourly"} {
		for i, v := range asArray(values[kind]) {
			t.count("plant_production")
			ref := fmt.Sprintf("%s:%s:%d", hex, kind, i)
			e := asDoc(v)
			at := bdate(e["timestamp"])
			kwh, _ := num(e["production_kwh"])
			switch {
			case at.IsZero():
				t.rj.Reject("plant_production", ref, "timestamp", "bad_date", str(e, "timestamp"))
				continue
			case kwh == nil:
				t.rj.Reject("plant_production", ref, "production_kwh", "required", nil)
				continue
			case at.After(t.opt.Now):
				t.rj.Reject("plant_production", ref, "timestamp", "future", ts(at))
				continue
			}
			day := civilDate(at)
			r := row{kwh: kwh.String(), basis: "daily_total", ref: ref}
			if kind == "daily" {
				daily[day] = true
				d := dateOf(day)
				r.at = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, istanbul).UTC() // local midnight (R279)
			} else {
				if daily[day] {
					t.rj.Reject("plant_production", ref, "hourly", "day_has_daily_total", day)
					continue
				}
				r.at, r.basis, r.power = at.UTC(), "plant_meter", zeroNil(e["instant_power_kw"])
			}
			if _, dup := rows[r.at]; dup {
				t.rj.Reject("plant_production", ref, "timestamp", "duplicate_last_wins", ts(r.at))
			} else {
				t.rj.Accept("plant_production")
			}
			rows[r.at] = r
		}
	}
	keys := make([]time.Time, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	for _, k := range keys {
		r := rows[k]
		if err := t.w.row("plant_production_totals", map[string]any{"plant_id": ID("powerplants", hex).String(), "ts": ts(r.at),
			"production_kwh": r.kwh, "active_power_kw": r.power, "basis": r.basis}); err != nil {
			return err
		}
	}
	return nil
}
