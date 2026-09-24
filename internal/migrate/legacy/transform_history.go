package legacy

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var periodKey = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// canonical is a legacy sub-document as relaxed extended JSON with sorted
// keys: a bson.M marshals in random order, which would break R412.
func canonical(v any) (json.RawMessage, error) {
	doc := asDoc(v)
	if doc == nil {
		return json.RawMessage("{}"), nil
	}
	b, err := bson.MarshalExtJSON(doc, false, false)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return nil, err
	}
	return json.Marshal(x)
}

// billHistoryStep is R421: every legacy invoice lands in legacy_bills for reconcile, never in bills.
func (t *transformer) billHistoryStep() error {
	if err := ReadExtract(t.dir, "buildings", func(b bson.M) error {
		hex := hexID(b["_id"])
		return t.bills("building_bill_history", "building", hex, hex, b["billHistory"])
	}); err != nil {
		return err
	}
	return ReadExtract(t.dir, "analyzers", func(a bson.M) error {
		hex := hexID(a["_id"])
		return t.bills("analyzer_bill_history", "analyzer", hex, t.analyzerBuilding[hex], a["billHistory"])
	})
}

func (t *transformer) bills(collection, scope, ownerHex, buildingHex string, history any) error {
	h := asDoc(history)
	periods := make([]string, 0, len(h))
	for k := range h {
		periods = append(periods, k)
	}
	sort.Strings(periods)
	company, known := t.buildingCompany[buildingHex]
	for _, period := range periods {
		t.count(collection)
		ref := ownerHex + ":" + period
		switch {
		case !known || (scope == "analyzer" && t.analyzerBuilding[ownerHex] == ""):
			t.rj.Reject(collection, ref, scope, scope+"_rejected", ownerHex)
			continue
		case !periodKey.MatchString(period):
			t.rj.Reject(collection, ref, "period", "bad_period", period)
			continue
		}
		rec := asDoc(h[period])
		payload, err := canonical(rec)
		if err != nil {
			return err
		}
		n := func(key string) any {
			v, err := num(rec[key])
			if err != nil {
				return nil
			}
			return dec(v)
		}
		civil := func(key string) any {
			if d := bdate(rec[key]); !d.IsZero() {
				return civilDate(d)
			}
			return nil
		}
		var analyzer, applied any
		if scope == "analyzer" {
			analyzer = ID("analyzers", ownerHex).String()
		}
		if v, ok := rec["reactivePenaltyApplied"].(bool); ok {
			applied = v
		}
		id := ID("legacy_bills", scope+":"+ref)
		t.rj.Accept(collection)
		if err := t.w.row("legacy_bills", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(),
			"building_id": ID("buildings", buildingHex).String(), "analyzer_id": analyzer, "scope": scope, "period": period,
			"start_date": civil("startDate"), "end_date": civil("endDate"), "total_active_kwh": n("totalActiveKWh"), "t1_kwh": n("t1KWh"),
			"t2_kwh": n("t2KWh"), "t3_kwh": n("t3KWh"), "inductive_kvarh": n("totalInductiveKVarh"), "capacitive_kvarh": n("totalCapacitiveKVarh"),
			"energy_cost": n("energyCost"), "distribution_cost": n("distributionCost"), "capacity_cost": n("capacityCost"), "power_cost": n("powerCost"),
			"green_energy_cost": n("greenEnergyCost"), "reactive_penalty": n("reactivePenalty"), "reactive_penalty_applied": applied,
			"vat_cost": n("vatCost"), "other_taxes_cost": n("otherTaxesCost"), "total_cost": n("totalCost"), "pdf_path": strOrNil(str(rec, "pdfPath")),
			"payload": payload}); err != nil {
			return err
		}
	}
	return nil
}

// reportsStep is R422.
func (t *transformer) reportsStep() error {
	return ReadExtract(t.dir, "reports", func(r bson.M) error {
		t.count("reports")
		hex, company, building := hexID(r["_id"]), hexID(r["company"]), hexID(r["building"])
		switch {
		case !t.companies[company]:
			t.rj.Reject("reports", hex, "company", "company_unknown", company)
			return nil
		case building != "" && t.buildingCompany[building] != company:
			t.rj.Reject("reports", hex, "building", "building_unknown", building)
			return nil
		case str(r, "reportType") == "" || str(r, "period") == "":
			t.rj.Reject("reports", hex, "", "required", nil)
			return nil
		}
		payload := bson.M{}
		for _, k := range []string{"monthlyReport", "yearlyReport", "carbonEmission", "solarPlantSelection"} {
			if r[k] != nil {
				payload[k] = r[k]
			}
		}
		raw, err := canonical(payload)
		if err != nil {
			return err
		}
		var b any
		if building != "" {
			b = ID("buildings", building).String()
		}
		id := ID("reports", hex)
		t.rj.Accept("reports")
		if err := t.w.row("legacy_reports", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "building_id": b,
			"report_type": str(r, "reportType"), "period": str(r, "period"), "payload": raw, "pdf_path": strOrNil(str(r, "pdfPath")),
			"excel_path": strOrNil(str(r, "excelPath")), "created_at": ts(bdate(r["createdAt"]))}); err != nil {
			return err
		}
		return t.w.legacyID("reports", hex, "legacy_reports", id)
	})
}

var logKinds = map[string]string{"cron": "job", "alarm": "alarm", "system": "system"}
var logStatuses = map[string]bool{"success": true, "error": true, "warning": true, "info": true}

// logsStep is R423 with Q-J10's retention window.
func (t *transformer) logsStep() error {
	days := t.opt.LogsDays
	if days <= 0 {
		days = 180
	}
	since := t.opt.Now.AddDate(0, 0, -days)
	return ReadExtract(t.dir, "logs", func(l bson.M) error {
		t.count("logs")
		hex := hexID(l["_id"])
		at := bdate(l["timestamp"])
		if at.IsZero() {
			at = bdate(l["createdAt"])
		}
		kind, okKind := logKinds[str(l, "type")]
		switch {
		case at.IsZero():
			t.rj.Reject("logs", hex, "timestamp", "required", nil)
			return nil
		case at.Before(since):
			t.rj.Reject("logs", hex, "timestamp", "outside_retention", at.Format(time.RFC3339))
			return nil
		case !okKind || !logStatuses[str(l, "status")] || str(l, "message") == "" || str(l, "category") == "":
			t.rj.Reject("logs", hex, "", "invalid", str(l, "type")+"/"+str(l, "status"))
			return nil
		}
		meta := asDoc(l["metadata"])
		if meta == nil {
			meta = bson.M{}
		}
		meta["legacy_id"] = hex // Q-J15: load replaces the migrated rows by this key
		company, relatedType, related := t.relatedOf(str(l, "relatedModel"), str(l, "relatedId"))
		if related == nil && str(l, "relatedId") != "" {
			meta["legacy_related"] = str(l, "relatedModel") + ":" + str(l, "relatedId")
		}
		raw, err := canonical(meta)
		if err != nil {
			return err
		}
		t.rj.Accept("logs")
		return t.w.row("operational_messages", map[string]any{"company_id": company, "kind": kind, "category": str(l, "category"),
			"status": str(l, "status"), "message": str(l, "message"), "detail": strOrNil(str(l, "details")), "related_type": relatedType,
			"related_id": related, "metadata": raw, "created_at": ts(at)})
	})
}

// relatedOf resolves a legacy (model, id) to the migrated company, type and id.
func (t *transformer) relatedOf(model, id string) (company, relatedType, related any) {
	if model != "" {
		relatedType = strings.ToLower(model)
	}
	switch strings.ToLower(model) {
	case "analyzer":
		if b, ok := t.analyzerBuilding[id]; ok {
			return ID("companies", t.buildingCompany[b]).String(), relatedType, ID("analyzers", id).String()
		}
	case "building":
		if c, ok := t.buildingCompany[id]; ok {
			return ID("companies", c).String(), relatedType, ID("buildings", id).String()
		}
	case "company":
		if t.companies[id] {
			return ID("companies", id).String(), relatedType, ID("companies", id).String()
		}
	}
	return nil, relatedType, nil
}
