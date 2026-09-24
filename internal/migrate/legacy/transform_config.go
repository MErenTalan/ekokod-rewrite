package legacy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var providerEnum = map[string]string{"OSOS": "osos", "GRIDBOX": "gridbox", "ARIL": "aril", "PM5340": "pm5340", "ISOLAR": "isolar"}

var roles = map[string]string{
	"admin": "admin", "company-admin": "company_admin", "company-readonly-admin": "company_readonly_admin",
	"building-admin": "building_admin", "building-readonly-admin": "building_readonly_admin", "demo": "demo",
}

func bdate(v any) time.Time {
	switch d := v.(type) {
	case bson.DateTime:
		return d.Time().UTC()
	case time.Time:
		return d.UTC()
	case string:
		t, err := ParseDate(d)
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// civilDate is the Istanbul calendar date of an instant, as YYYY-MM-DD.
func civilDate(t time.Time) string { return t.In(istanbul).Format(time.DateOnly) }

func (t *transformer) integrations() error {
	return ReadExtract(t.dir, "integrations", func(d bson.M) error {
		t.count("integrations")
		hex := hexID(d["_id"])
		provider, ok := providerEnum[strings.ToUpper(str(d, "type"))]
		if !ok {
			t.rj.Reject("integrations", hex, "type", "unknown_provider", str(d, "type"))
			return nil
		}
		id := ID("integrations", hex)
		endpoints, err := canonical(d["endpoints"])
		if err != nil {
			endpoints = json.RawMessage("{}")
		}
		t.definitions[strings.ToUpper(str(d, "type"))+":"+str(d, "subType")] = id
		t.rj.Accept("integrations")
		if err := t.w.row("integration_definitions", map[string]any{"id": id.String(), "provider": provider, "subtype": str(d, "subType"), "endpoints": endpoints}); err != nil {
			return err
		}
		return t.w.legacyID("integrations", hex, "integration_definitions", id)
	})
}

func (t *transformer) companiesStep() error {
	return ReadExtract(t.dir, "companies", func(c bson.M) error {
		t.count("companies")
		hex := hexID(c["_id"])
		if strings.TrimSpace(str(c, "name")) == "" {
			t.rj.Reject("companies", hex, "name", "required", nil)
			return nil
		}
		id := ID("companies", hex)
		t.companies[hex] = true
		subs := map[string]string{}
		contact := asDoc(c["contactPerson"])
		area, _ := num(c["totalArea"])
		people, _ := num(c["personnelCount"])
		t.rj.Accept("companies")
		if err := t.w.row("companies", map[string]any{"id": id.String(), "name": strings.TrimSpace(str(c, "name")), "address": strOrNil(str(c, "address")),
			"sector": strOrNil(str(c, "sector")), "contact_name": strOrNil(str(contact, "name")), "contact_phone": strOrNil(str(contact, "phone")),
			"total_area_m2": dec(area), "personnel_count": dec(people), "created_at": ts(bdate(c["createdAt"]))}); err != nil {
			return err
		}
		if err := t.w.legacyID("companies", hex, "companies", id); err != nil {
			return err
		}
		vac := asDoc(c["vacations"])
		for _, day := range asArray(vac["weekend"]) {
			n, err := num(day)
			if err != nil || n == nil || n.IntPart() < 0 || n.IntPart() > 6 {
				continue // the legacy validator already enforced 0..6
			}
			if err := t.w.row("company_weekend_days", map[string]any{"company_id": id.String(), "day_of_week": n.IntPart()}); err != nil {
				return err
			}
		}
		for i, v := range asArray(vac["other"]) {
			p := asDoc(v)
			from, to := bdate(p["startDate"]), bdate(p["endDate"])
			if from.IsZero() || to.IsZero() || to.Before(from) {
				continue
			}
			if err := t.w.row("company_vacations", map[string]any{"id": ID("company_vacations", fmt.Sprintf("%s:%d", hex, i)).String(), "company_id": id.String(),
				"start_date": civilDate(from), "end_date": civilDate(to), "description": strOrNil(str(p, "description"))}); err != nil {
				return err
			}
		}
		for _, v := range append(asArray(c["events"]), asArray(c["calendarEvents"])...) {
			e := asDoc(v)
			start, end := bdate(e["start"]), bdate(e["end"])
			if start.IsZero() || end.IsZero() || str(e, "title") == "" {
				continue
			}
			allDay, _ := e["allDay"].(bool)
			if err := t.w.row("calendar_events", map[string]any{"id": ID("calendar_events", hex+":"+str(e, "id")).String(), "company_id": id.String(),
				"title": str(e, "title"), "starts_at": ts(start), "ends_at": ts(end), "all_day": allDay, "colour": strOrNil(str(e, "color"))}); err != nil {
				return err
			}
		}
		for i, v := range asArray(c["integrations"]) {
			in := asDoc(v)
			subs[str(in, "subType")] = str(in, "type")
			if err := t.credential(hex, i, in); err != nil {
				return err
			}
		}
		t.companySubtypes[hex] = subs
		return nil
	})
}

func (t *transformer) credential(companyHex string, i int, in bson.M) error {
	t.count("integration_credentials")
	legacyRef := fmt.Sprintf("%s:%d", companyHex, i)
	def, ok := t.definitions[strings.ToUpper(str(in, "type"))+":"+str(in, "subType")]
	if !ok {
		t.rj.Reject("integration_credentials", legacyRef, "subType", "definition_unknown", str(in, "type")+":"+str(in, "subType"))
		return nil
	}
	company := ID("companies", companyHex)
	row := map[string]any{"id": ID("integration_credentials", legacyRef).String(), "company_id": company.String(), "definition_id": def.String(),
		"username": strOrNil(str(in, "username")), "pm5340_url": strOrNil(str(in, "pm5340Url")), "isolar_region": strOrNil(str(in, "isolar_region")),
		"token_expires_at": ts(bdate(in["isolar_token_expires_at"])), "is_active": true}
	useBilling, _ := in["useBillingEndexes"].(bool)
	row["settings"] = map[string]any{"use_billing_indexes": useBilling}
	if pw := str(in, "password"); pw != "" {
		token, _, err := t.resealer.Reseal(company, def, pw)
		if err != nil {
			t.rj.Reject("integration_credentials", legacyRef, "password", "decrypt_failed", nil)
			return nil
		}
		row["secret_enc"] = token
	}
	extra := map[string]string{}
	for _, k := range []string{"isolar_appkey", "isolar_secretkey", "isolar_app_id", "isolar_access_token", "isolar_refresh_token"} {
		if v := str(in, k); v != "" {
			plain := v
			if strings.HasPrefix(v, encPrefix) {
				var err error
				if plain, err = t.opt.Keys.Decrypt(v); err != nil {
					t.rj.Reject("integration_credentials", legacyRef, k, "decrypt_failed", nil)
					return nil
				}
			}
			extra[strings.TrimPrefix(k, "isolar_")] = plain
		}
	}
	if len(extra) > 0 {
		b, err := json.Marshal(extra)
		if err != nil {
			return err
		}
		token, _, err := t.resealer.Reseal(company, def, string(b))
		if err != nil {
			return err
		}
		row["extra_enc"] = token
	}
	t.rj.Accept("integration_credentials")
	return t.w.row("integration_credentials", row)
}

func (t *transformer) usersStep() error {
	return ReadExtract(t.dir, "users", func(u bson.M) error {
		t.count("users")
		hex := hexID(u["_id"])
		role, ok := roles[str(u, "userType")]
		switch {
		case !ok:
			t.rj.Reject("users", hex, "userType", "unknown_role", str(u, "userType"))
			return nil
		case !t.companies[hexID(u["company"])]:
			t.rj.Reject("users", hex, "company", "company_unknown", hexID(u["company"]))
			return nil
		case !isBcrypt(str(u, "password")):
			t.rj.Reject("users", hex, "password", "not_bcrypt", nil) // Q-J4: this user gets a forced reset
			return nil
		}
		id := ID("users", hex)
		t.users[hex] = true
		t.rj.Accept("users")
		if err := t.w.row("users", map[string]any{"id": id.String(), "company_id": ID("companies", hexID(u["company"])).String(), "name": str(u, "name"),
			"email": strings.ToLower(strings.TrimSpace(str(u, "email"))), "phone": strOrNil(str(u, "phone")), "password_hash": "legacy$" + str(u, "password"),
			"password_changed_at": ts(bdate(u["passwordChangedAt"])), "role": role, "is_active": true, "created_at": ts(bdate(u["createdAt"]))}); err != nil {
			return err
		}
		for i, h := range asArray(u["passwordHistory"]) {
			hash, _ := h.(string)
			if !isBcrypt(hash) {
				continue
			}
			if err := t.w.row("user_password_history", map[string]any{"id": ID("user_password_history", fmt.Sprintf("%s:%d", hex, i)).String(),
				"user_id": id.String(), "password_hash": "legacy$" + hash}); err != nil {
				return err
			}
		}
		return t.w.legacyID("users", hex, "users", id)
	})
}

func isBcrypt(s string) bool {
	return len(s) == 60 && (strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$"))
}

func (t *transformer) buildingsStep() error {
	return ReadExtract(t.dir, "buildings", func(b bson.M) error {
		t.count("buildings")
		hex := hexID(b["_id"])
		company := hexID(b["company_id"])
		if !t.companies[company] {
			t.rj.Reject("buildings", hex, "company_id", "company_unknown", company)
			return nil
		}
		id := ID("buildings", hex)
		t.buildingCompany[hex] = company
		lat, _ := num(b["lat"])
		lon, _ := num(b["long"])
		floors, _ := num(b["floors"])
		people, _ := num(b["personel_count"]) //nolint:misspell // the legacy field is spelled this way
		area, _ := num(b["total_area"])
		cutoff := int64(1)
		if n, _ := num(b["billCutoffDay"]); n != nil && n.IntPart() >= 1 && n.IntPart() <= 31 {
			cutoff = n.IntPart()
		}
		var responsible any
		if u := hexID(b["user_in_charge"]); t.users[u] {
			responsible = ID("users", u).String()
		}
		zeroIsNull := func(v any) any {
			if s, ok := v.(string); ok && (s == "0" || s == "") {
				return nil
			}
			return v
		}
		t.rj.Accept("buildings")
		if err := t.w.row("buildings", map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "name": str(b, "name"),
			"address": strOrNil(str(b, "address")), "latitude": zeroIsNull(dec(lat)), "longitude": zeroIsNull(dec(lon)), "floors": dec(floors),
			"personnel_count": dec(people), "total_area_m2": dec(area), "sector": strOrNil(str(b, "sector")), "responsible_user_id": responsible,
			"bill_cutoff_day": cutoff, "created_at": ts(bdate(b["createdAt"]))}); err != nil {
			return err
		}
		for i, v := range asArray(b["contact_persons"]) {
			p := asDoc(v)
			if str(p, "name") == "" && str(p, "phone") == "" {
				continue
			}
			if err := t.w.row("building_contacts", map[string]any{"id": ID("building_contacts", fmt.Sprintf("%s:%d", hex, i)).String(), "building_id": id.String(),
				"name": strOrNil(str(p, "name")), "phone": strOrNil(str(p, "phone")), "sort_order": i}); err != nil {
				return err
			}
		}
		return t.w.legacyID("buildings", hex, "buildings", id)
	})
}
