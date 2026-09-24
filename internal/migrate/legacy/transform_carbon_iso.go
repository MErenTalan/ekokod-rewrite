package legacy

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	iso "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// artifactRef lists one file the artifacts step copies and registers (R428).
func (t *transformer) artifactRef(ref, company, ownerType, ownerID, clause, sourcePath, name string) error {
	if strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	if name == "" {
		name = path.Base(sourcePath)
	}
	return t.w.row("artifact_refs", map[string]any{"ref": ref, "company_id": ID("companies", company).String(), "owner_type": ownerType,
		"owner_id": ownerID, "clause_id": strOrNil(clause), "source_path": sourcePath, "original_name": name})
}

// automatedCarbon marks legacy's daily accrual rows (cron/daily-carbon): derived
// data, recomputed by `ekokod recompute carbon` (08 §1), never migrated.
const automatedCarbon = "Günlük tüketim otomatik olarak eklenmiştir"

var carbonStatuses = map[string]string{"onay bekliyor": "pending", "pending": "pending", "onaylandi": "approved", "approved": "approved",
	"reddedildi": "rejected", "rejected": "rejected"}

// carbonStep is R426.
func (t *transformer) carbonStep() error {
	if err := ReadExtract(t.dir, "carbonfootprint", func(c bson.M) error {
		hex, company, building := hexID(c["_id"]), hexID(c["company_id"]), hexID(c["building_id"])
		known := t.buildingCompany[building] == company && company != ""
		for _, v := range asArray(c["selectedActivities"]) {
			t.count("carbon_selected")
			key, _ := v.(string)
			ref := hex + ":" + key
			if !known {
				t.rj.Reject("carbon_selected", ref, "building_id", "building_unknown", building)
				continue
			}
			if _, ok := carbon.SubByKey(key); !ok {
				t.rj.Reject("carbon_selected", ref, "selectedActivities", "unknown_sub_category", key)
				continue
			}
			t.rj.Accept("carbon_selected")
			if err := t.w.row("carbon_selected_activities", map[string]any{"company_id": ID("companies", company).String(),
				"building_id": ID("buildings", building).String(), "activity_key": key}); err != nil {
				return err
			}
		}
		for i, v := range asArray(c["activities"]) {
			t.count("carbon_activities")
			a := asDoc(v)
			ref := fmt.Sprintf("%s:%d", hex, i)
			if id := hexID(a["_id"]); id != "" {
				ref = hex + ":" + id
			}
			sub, okSub := carbon.SubByKey(str(a, "subCategory"))
			from, errFrom := ParseDate(str(a, "date"))
			to, errTo := ParseDate(str(a, "endDate"))
			if errTo != nil && str(a, "endDate") == "" {
				to, errTo = from, errFrom
			}
			qty, _ := num(a["amount"])
			emission, _ := num(a["emissionCo2e"])
			switch {
			case !known:
				t.rj.Reject("carbon_activities", ref, "building_id", "building_unknown", building)
				continue
			case str(a, "description") == automatedCarbon:
				t.rj.Reject("carbon_activities", ref, "description", "automated_accrual_recomputed", nil)
				continue
			case !okSub:
				t.rj.Reject("carbon_activities", ref, "subCategory", "unknown_sub_category", str(a, "subCategory"))
				continue
			case errFrom != nil || errTo != nil || to.Before(from):
				t.rj.Reject("carbon_activities", ref, "date", "bad_date", str(a, "date")+"/"+str(a, "endDate"))
				continue
			case qty == nil || !qty.IsPositive() || str(a, "unit") == "":
				t.rj.Reject("carbon_activities", ref, "amount", "required", nil)
				continue
			case emission == nil:
				t.rj.Reject("carbon_activities", ref, "emissionCo2e", "emission_missing", nil)
				continue
			}
			status, ok := carbonStatuses[fold(str(a, "status"))]
			if !ok {
				status = "pending"
			}
			factor, _ := num(a["emissionFactorUnitValue"])
			var factorValue any
			if factor != nil && factor.IsPositive() {
				factorValue = factor.String()
			}
			details, err := canonical(a["details"])
			if err != nil {
				return err
			}
			t.rj.Accept("carbon_activities")
			if err := t.w.row("carbon_activities", map[string]any{"id": ID("carbon_activities", ref).String(), "company_id": ID("companies", company).String(),
				"building_id": ID("buildings", building).String(), "main_category": sub.Main, "sub_category": sub.Key, "activity_type": sub.Key,
				"period_start": civilDate(from), "period_end": civilDate(to), "quantity": qty.String(), "unit": str(a, "unit"),
				"factor_key": strOrNil(str(a, "factorKey")), "factor_value": factorValue, "emission_kgco2e": emission.Round(6).String(),
				"scope": sub.Scope, "iso_category": sub.ISO, "description": strOrNil(str(a, "description")), "details": details,
				"status": status, "is_automated": false, "created_at": ts(bdate(c["createdAt"]))}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := t.companyFactors(); err != nil {
		return err
	}
	return t.carbonReports()
}

// companyFactors keeps only real company overrides: legacy copied the whole
// master list into every company by default, which is not an override.
func (t *transformer) companyFactors() error {
	master, err := seed.EmissionFactors()
	if err != nil {
		return err
	}
	byKey := map[string]seed.EmissionFactor{}
	for _, f := range master {
		byKey[f.Key] = f
	}
	return ReadExtract(t.dir, "companyemissionfactors", func(d bson.M) error {
		hex, company := hexID(d["_id"]), hexID(d["company_id"])
		seen := map[string]bool{}
		for i, v := range asArray(d["emissionFactors"]) {
			t.count("company_emission_factors")
			f := asDoc(v)
			key := strings.TrimSpace(str(f, "key"))
			ref := fmt.Sprintf("%s:%d", hex, i)
			base, _ := num(f["baseFactor"])
			switch {
			case !t.companies[company]:
				t.rj.Reject("company_emission_factors", ref, "company_id", "company_unknown", company)
				continue
			case key == "" || base == nil || str(f, "baseUnit") == "" || str(f, "mainCategory") == "":
				t.rj.Reject("company_emission_factors", ref, "", "required", key)
				continue
			case seen[key]:
				t.rj.Reject("company_emission_factors", ref, "key", "duplicate", key)
				continue
			}
			seen[key] = true
			if m, ok := byKey[key]; ok && m.BaseFactor.Equal(*base) && m.BaseUnit == str(f, "baseUnit") {
				t.notes["factor_same_as_master"]++
				t.rj.Accept("company_emission_factors") // nothing to override: the master row applies
				continue
			}
			id := ID("company_emission_factors", company+":"+key)
			meta := asDoc(f["metadata"])
			year, _ := num(meta["year"])
			var subs, catPath []string
			for _, s := range asArray(f["subCategory"]) {
				if k, ok := s.(string); ok {
					subs = append(subs, k)
				}
			}
			for _, s := range asArray(f["categoryPath"]) {
				if k, ok := s.(string); ok {
					catPath = append(catPath, k)
				}
			}
			var scope, sourceYear any
			if sc := str(f, "scope"); sc == "scope_1" || sc == "scope_2" || sc == "scope_3" {
				scope = sc
			}
			if year != nil {
				sourceYear = year.IntPart()
			}
			if subs == nil {
				subs = []string{}
			}
			if catPath == nil {
				catPath = []string{}
			}
			t.rj.Accept("company_emission_factors")
			if err := t.w.row("emission_factors", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "key": key,
				"label": strOrNil(str(f, "label")), "main_category": str(f, "mainCategory"), "sub_categories": subs, "category_path": catPath,
				"base_factor": base.String(), "base_unit": str(f, "baseUnit"), "fuel_type": strOrNil(str(f, "fuelType")),
				"vehicle_type": strOrNil(str(f, "busType")), "scope": scope, "iso_category": strOrNil(str(f, "category")),
				"status": strOrNil(str(f, "status")), "source": strOrNil(str(meta, "source")), "source_year": sourceYear,
				"source_url": strOrNil(str(meta, "url"))}); err != nil {
				return err
			}
			for _, c := range asArray(f["conversion"]) {
				conv := asDoc(c)
				mult, _ := num(conv["multiplier"])
				if str(conv, "unit") == "" || mult == nil {
					continue
				}
				if err := t.w.row("emission_factor_conversions", map[string]any{"factor_id": id.String(), "unit": str(conv, "unit"),
					"multiplier": mult.String(), "label": str(conv, "label")}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (t *transformer) carbonReports() error {
	return ReadExtract(t.dir, "carbonreporthistories", func(d bson.M) error {
		hex, company, building := hexID(d["_id"]), hexID(d["companyID"]), hexID(d["buildingID"])
		for i, v := range asArray(d["reports"]) {
			t.count("carbon_reports")
			r := asDoc(v)
			ref := fmt.Sprintf("%s:%d", hex, i)
			if id := hexID(r["_id"]); id != "" {
				ref = hex + ":" + id
			}
			typ := str(r, "reportType")
			switch {
			case t.buildingCompany[building] != company || company == "":
				t.rj.Reject("carbon_reports", ref, "buildingID", "building_unknown", building)
				continue
			case typ != "ghg" && typ != "iso":
				t.rj.Reject("carbon_reports", ref, "reportType", "unknown_type", typ)
				continue
			}
			payload, err := canonical(bson.M{"legacy": bson.M{"name": str(r, "name"), "stored_file_name": str(r, "storedFileName"),
				"created_date": str(r, "createdDate"), "date_period": str(r, "datePeriod")}})
			if err != nil {
				return err
			}
			id := ID("carbon_reports", ref)
			name := strings.TrimSpace(str(r, "name"))
			if name == "" {
				name = str(r, "storedFileName")
			}
			t.rj.Accept("carbon_reports")
			if err := t.w.row("carbon_reports", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(),
				"building_id": ID("buildings", building).String(), "name": name, "report_type": typ, "period": str(r, "datePeriod"),
				"payload": payload, "pdf_path": strOrNil(str(r, "pdfPath")), "created_at": ts(bdate(r["createdDate"]))}); err != nil {
				return err
			}
			if err := t.artifactRef("carbon_reports:"+ref, company, "carbon", id.String(), "", str(r, "pdfPath"), str(r, "storedFileName")); err != nil {
				return err
			}
		}
		return nil
	})
}

// isoBuilding is Q-J9: the building a legacy user's ISO work belongs to.
func (t *transformer) isoBuilding(user string) (string, bool, error) {
	if b := t.answer(AnswerISOBuilding, user); b != "" {
		if _, ok := t.buildingCompany[b]; !ok || t.buildingCompany[b] != t.userCompany[user] {
			return "", false, fmt.Errorf("legacy: answers.csv: iso_building %s: %q is not a building of the user's company", user, b)
		}
		return b, true, nil
	}
	if in := t.inCharge[user]; len(in) == 1 {
		return in[0], true, nil
	}
	if all := t.companyBuildings[t.userCompany[user]]; len(all) == 1 {
		return all[0], true, nil
	}
	return "", false, nil
}

// isoStep is R427.
func (t *transformer) isoStep() error {
	projects := map[string]bool{}
	resolve := func(collection, ref, user string) (string, bool, error) {
		if t.userCompany[user] == "" {
			t.rj.Reject(collection, ref, "userId", "user_unknown", user)
			return "", false, nil
		}
		b, ok, err := t.isoBuilding(user)
		if err != nil || !ok {
			if err == nil {
				t.ask(AnswerISOBuilding, user, "the user's ISO 50001 work needs a building")
				t.rj.Reject(collection, ref, "userId", "building_ambiguous", user)
			}
			return "", false, err
		}
		if !projects[b] {
			projects[b] = true
			if err := t.w.row("iso50001_projects", map[string]any{"id": ID("iso50001_projects", b).String(),
				"company_id": ID("companies", t.buildingCompany[b]).String(), "building_id": ID("buildings", b).String()}); err != nil {
				return "", false, err
			}
		}
		return b, true, nil
	}
	// A clause's dates come from the most recently updated document (Q-J9).
	type dated struct {
		at         string
		start, end any
	}
	clauseDates := map[string]map[string]dated{}
	if err := ReadExtract(t.dir, "projectDates", func(d bson.M) error {
		hex, user := hexID(d["_id"]), str(d, "userId")
		dates := asDoc(d["dates"])
		clauses := make([]string, 0, len(dates))
		for c := range dates {
			clauses = append(clauses, c)
		}
		sort.Strings(clauses)
		for _, c := range clauses {
			t.count("iso_project_dates")
			ref := hex + ":" + c
			b, ok, err := resolve("iso_project_dates", ref, user)
			if err != nil || !ok {
				return err
			}
			if !iso.IsMain(c) {
				t.rj.Reject("iso_project_dates", ref, "dates", "unknown_clause", c)
				continue
			}
			v := asDoc(dates[c])
			civil := func(k string) any {
				if dt := bdate(v[k]); !dt.IsZero() {
					return civilDate(dt)
				}
				return nil
			}
			if s, e := civil("startDate"), civil("endDate"); s != nil && e != nil && s.(string) > e.(string) {
				t.rj.Reject("iso_project_dates", ref, "dates", "bad_range", fmt.Sprint(s, "/", e))
				continue
			}
			t.rj.Accept("iso_project_dates")
			at := ts(bdate(d["updatedAt"]))
			stamp, _ := at.(string)
			if clauseDates[b] == nil {
				clauseDates[b] = map[string]dated{}
			}
			if prev, ok := clauseDates[b][c]; !ok || stamp >= prev.at {
				clauseDates[b][c] = dated{stamp, civil("startDate"), civil("endDate")}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	buildings := make([]string, 0, len(clauseDates))
	for b := range clauseDates {
		buildings = append(buildings, b)
	}
	sort.Strings(buildings)
	for _, b := range buildings {
		clauses := make([]string, 0, len(clauseDates[b]))
		for c := range clauseDates[b] {
			clauses = append(clauses, c)
		}
		sort.Strings(clauses)
		for _, c := range clauses {
			d := clauseDates[b][c]
			if err := t.w.row("iso50001_clause_dates", map[string]any{"project_id": ID("iso50001_projects", b).String(), "clause_id": c,
				"start_date": d.start, "end_date": d.end}); err != nil {
				return err
			}
		}
	}
	if err := ReadExtract(t.dir, "notes", func(n bson.M) error {
		t.count("iso_notes")
		hex, user := hexID(n["_id"]), str(n, "userId")
		b, ok, err := resolve("iso_notes", hex, user)
		if err != nil || !ok {
			return err
		}
		clause := strings.TrimSpace(str(n, "subItem"))
		switch {
		case !validSub(clause):
			t.rj.Reject("iso_notes", hex, "subItem", "unknown_clause", clause)
			return nil
		case strings.TrimSpace(str(n, "text")) == "":
			t.rj.Reject("iso_notes", hex, "text", "required", nil)
			return nil
		}
		var by any
		if t.users[user] {
			by = ID("users", user).String()
		}
		t.rj.Accept("iso_notes")
		return t.w.row("iso50001_notes", map[string]any{"id": ID("iso_notes", hex).String(), "project_id": ID("iso50001_projects", b).String(),
			"clause_id": clause, "title": strOrNil(str(n, "title")), "body": str(n, "text"), "created_by": by,
			"created_at": ts(bdate(n["createdAt"])), "updated_at": ts(bdate(n["updatedAt"]))})
	}); err != nil {
		return err
	}
	return ReadExtract(t.dir, "files", func(f bson.M) error {
		t.count("iso_files")
		hex, user := hexID(f["_id"]), str(f, "userId")
		b, ok, err := resolve("iso_files", hex, user)
		if err != nil || !ok {
			return err
		}
		clause := strings.TrimSpace(str(f, "itemNo"))
		if !validSub(clause) {
			t.rj.Reject("iso_files", hex, "itemNo", "unknown_clause", clause)
			return nil
		}
		t.rj.Accept("iso_files")
		return t.artifactRef("files:"+hex, t.buildingCompany[b], "iso50001", ID("buildings", b).String(), clause, str(f, "path"), str(f, "originalName"))
	})
}

func validSub(clause string) bool {
	_, ok := iso.SubByID(clause)
	return ok
}
