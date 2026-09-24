package legacy

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func (t *transformer) analyzersStep() error {
	return ReadExtract(t.dir, "analyzers", func(a bson.M) error {
		t.count("analyzers")
		hex := hexID(a["_id"])
		building := hexID(a["building"])
		company, ok := t.buildingCompany[building]
		if !ok {
			t.rj.Reject("analyzers", hex, "building", "company_unknown", building)
			return nil
		}
		providerName := "UNKNOWN"
		if typ, ok := t.companySubtypes[company][str(a, "subIntegration")]; ok {
			providerName = strings.ToUpper(typ)
		}
		provider, ok := providerEnum[providerName]
		if !ok {
			t.rj.Reject("analyzers", hex, "subIntegration", "provider_unknown", str(a, "subIntegration"))
			return nil
		}
		dec := DecideMultiplier(providerName, str(a, "meterMultiplier"), t.opt.Confirmations[hex])
		res := AnalyzerReadings(a, providerName, dec, t.opt.Now)
		id := ID("analyzers", hex)
		if !dec.Determined {
			t.manual = append(t.manual, manualRow{legacyID: hex, id: id, provider: providerName, stored: str(a, "meterMultiplier"), reason: dec.Reason, readings: len(res.Readings)})
		}
		meterMultiplier := "1"
		if m, err := ParseNumber(str(a, "meterMultiplier")); err == nil && m != nil && m.IsPositive() {
			meterMultiplier = m.String()
		}
		power, _ := num(a["kuruluGucu"])
		lat, _ := num(a["koordinatX"])
		lon, _ := num(a["koordinatY"])
		defType, _ := num(a["definitionType"])
		active, _ := a["isActive"].(bool)
		t.rj.Accept("analyzers")
		if err := t.w.row("analyzers", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "building_id": ID("buildings", building).String(),
			"provider": provider, "provider_subtype": str(a, "subIntegration"), "installation_number": str(a, "installationNumber"),
			"customer_name": strOrNil(str(a, "customerName")), "address": strOrNil(str(a, "address")), "province": strOrNil(str(a, "il")),
			"district": strOrNil(str(a, "ilce")), "neighbourhood": strOrNil(str(a, "koyMahallesi")), "street": strOrNil(str(a, "caddesiSokagi")),
			"tariff_type": strOrNil(str(a, "tarifeTipi")), "tariff_kind": strOrNil(str(a, "tarifeTuru")), "installation_kind": strOrNil(str(a, "tesisatTurTanim")),
			"installed_power_kw": dec2(power), "meter_number": strOrNil(str(a, "meterNumber")), "meter_model": strOrNil(str(a, "meterModel")),
			"meter_multiplier": meterMultiplier, "multiplier_confirmed": dec.Determined, // a load hint, not a column (F14b) "counterparty_no": strOrNil(str(a, "muhatapNo")),
			"metering_point_name": strOrNil(str(a, "sayimNokTanim")), "latitude": dec2(lat), "longitude": dec2(lon), "etso_code": strOrNil(str(a, "etso")),
			"definition_type": dec2(defType), "last_reading_at": ts(bdate(a["lastDataDate"])), "is_active": active}); err != nil {
			return err
		}
		if err := t.w.legacyID("analyzers", hex, "analyzers", id); err != nil {
			return err
		}
		return t.readings(hex, res)
	})
}

func dec2(v any) any { return v }

func (t *transformer) readings(hex string, res Readings) error {
	for _, r := range res.Readings {
		t.count("meter_readings")
		t.rj.Accept("meter_readings")
		if err := t.w.row("meter_readings", readingRow(r)); err != nil {
			return err
		}
	}
	for _, rej := range res.Rejects {
		t.count("meter_readings")
		t.rj.Reject("meter_readings", hex, string(rej.Kind), rej.Reason, rej.Index)
	}
	// Duplicates were read too: they are accounted as rejected ("duplicate_last_wins").
	for i := 0; i < res.Duplicates; i++ {
		t.count("meter_readings")
		t.rj.Reject("meter_readings", hex, "", "duplicate_last_wins", nil)
	}
	for _, d := range res.Drops {
		if err := t.w.row("consumption_anomalies", map[string]any{"id": ID("consumption_anomalies", hex+":"+string(d.Kind)+":"+d.To.UTC().Format("20060102T150405")).String(),
			"analyzer_id": ID("analyzers", hex).String(), "period_start": ts(d.From), "period_end": ts(d.To), "reason": "negative_delta",
			"detail": map[string]any{"kind": d.Kind, "delta": d.Delta.String(), "source": "legacy_migration"}}); err != nil {
			return err
		}
	}
	return nil
}

func readingRow(r model.MeterReading) map[string]any {
	return map[string]any{"analyzer_id": r.AnalyzerID.String(), "ts": ts(r.Ts), "kind": string(r.Kind),
		"active_import": dec(r.ActiveImport), "reactive_inductive_import": dec(r.ReactiveInductiveImport), "reactive_capacitive_import": dec(r.ReactiveCapacitiveImport),
		"t1_import": dec(r.T1Import), "t2_import": dec(r.T2Import), "t3_import": dec(r.T3Import),
		"active_export": dec(r.ActiveExport), "reactive_inductive_export": dec(r.ReactiveInductiveExport), "reactive_capacitive_export": dec(r.ReactiveCapacitiveExport),
		"t1_export": dec(r.T1Export), "t2_export": dec(r.T2Export), "t3_export": dec(r.T3Export), "max_demand_kw": dec(r.MaxDemandKw),
		"meter_serial": r.MeterSerial, "multiplier_applied": r.MultiplierApplied.String(), "source_provider": string(r.SourceProvider), "raw": r.Raw}
}
