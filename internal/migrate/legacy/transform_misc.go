package legacy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// templateDefaultClass is R418's placeholder: a template is a starting point a
// user edits before applying, so it is flagged for review, never used to bill.
var templateDefaultClass = tariffClass{model.VoltageLevelLV, model.UserGroupCommercial, model.TariffTermMonomial, model.SupplyCompanyPrivate}

// templatesStep is R418.
func (t *transformer) templatesStep() error {
	type kept struct {
		hex, ask string // ask: the question when the classification was defaulted
		row      map[string]any
	}
	last := map[string]kept{} // company + lower(name) → last by _id
	var order []string
	if err := ReadExtract(t.dir, "tarifftemplates", func(d bson.M) error {
		t.count("tarifftemplates")
		hex, company := hexID(d["_id"]), hexID(d["company_id"])
		if !t.companies[company] {
			t.rj.Reject("tarifftemplates", hex, "company_id", "company_unknown", company)
			return nil
		}
		name := strings.TrimSpace(str(d, "name"))
		if name == "" {
			t.rj.Reject("tarifftemplates", hex, "name", "required", nil)
			return nil
		}
		class, ok := templateDefaultClass, false
		if a := t.answer(AnswerTariffTemplateClass, hex); a != "" {
			if class, ok = parseClass(a); !ok {
				return fmt.Errorf("legacy: answers.csv: tariff_template_class %s: %q is not lv|mv/<group>/<term>/<supply>", hex, a)
			}
		}
		question := ""
		if !ok {
			class, question = templateDefaultClass, "defaulted to "+templateDefaultClass.String()+": "+name
		}
		draft, reason, value, err := t.tariffDraft("tarifftemplates", hex, asDoc(d["tariff"]), class)
		if err != nil {
			return err
		}
		if reason != "" {
			t.rj.Reject("tarifftemplates", hex, "tariff", reason, value)
			return nil
		}
		if _, err := tariff.Validate(draft); err != nil {
			t.rj.Reject("tarifftemplates", hex, "tariff", "invalid_tariff", err.Error())
			return nil
		}
		payload, err := json.Marshal(tariffsvc.Input{Tariff: draft.Tariff, VatRate: draft.VatRate, Taxes: draft.Taxes, ManualYekdem: draft.ManualYekdem})
		if err != nil {
			return err
		}
		id := ID("tarifftemplates", hex)
		isDefault, _ := d["isDefault"].(bool)
		key := company + ":" + strings.ToLower(name)
		if prev, dup := last[key]; dup {
			t.rj.Reject("tarifftemplates", prev.hex, "name", "duplicate_name", hex)
		} else {
			order = append(order, key)
		}
		last[key] = kept{hex, question, map[string]any{"id": id.String(), "company_id": ID("companies", company).String(), "name": name,
			"description": strOrNil(str(d, "description")), "is_default": isDefault, "payload": json.RawMessage(payload),
			"created_at": ts(bdate(d["createdAt"])), "updated_at": ts(bdate(d["updatedAt"]))}}
		return nil
	}); err != nil {
		return err
	}
	for _, key := range order {
		k := last[key]
		if k.ask != "" {
			t.ask(AnswerTariffTemplateClass, k.hex, k.ask)
		}
		t.rj.Accept("tarifftemplates")
		if err := t.w.row("tariff_templates", k.row); err != nil {
			return err
		}
		if err := t.w.legacyID("tarifftemplates", k.hex, "tariff_templates", ID("tarifftemplates", k.hex)); err != nil {
			return err
		}
	}
	return nil
}

// smtpStep is R419: one row per company, the password re-sealed under the row's AAD.
func (t *transformer) smtpStep() error {
	type kept struct {
		hex string
		row map[string]any
	}
	last := map[string]kept{}
	if err := ReadExtract(t.dir, "smtpsettings", func(d bson.M) error {
		t.count("smtpsettings")
		hex, company := hexID(d["_id"]), hexID(d["company"])
		auth := asDoc(d["auth"])
		port, _ := num(d["port"])
		switch {
		case !t.companies[company]:
			t.rj.Reject("smtpsettings", hex, "company", "company_unknown", company)
			return nil
		case str(d, "host") == "" || str(d, "from") == "" || str(auth, "user") == "" || str(auth, "pass") == "" || port == nil:
			t.rj.Reject("smtpsettings", hex, "", "required", nil)
			return nil
		}
		pass := str(auth, "pass")
		if strings.HasPrefix(pass, encPrefix) {
			var err error
			if pass, err = t.opt.Keys.Decrypt(pass); err != nil {
				t.rj.Reject("smtpsettings", hex, "auth.pass", "decrypt_failed", nil)
				return nil
			}
		}
		cid := ID("companies", company)
		token, err := t.opt.Cipher.Seal([]byte(pass), postgres.SMTPPasswordAAD(cid))
		if err != nil {
			return err
		}
		secure, _ := d["secure"].(bool)
		if prev, dup := last[company]; dup {
			t.rj.Reject("smtpsettings", prev.hex, "company", "duplicate_last_wins", hex)
		}
		last[company] = kept{hex, map[string]any{"company_id": cid.String(), "host": str(d, "host"), "port": port.IntPart(), "secure": secure,
			"username": str(auth, "user"), "password_enc": token, "from_address": strings.TrimSpace(str(d, "from")), "updated_at": ts(bdate(d["updatedAt"]))}}
		return nil
	}); err != nil {
		return err
	}
	companies := make([]string, 0, len(last))
	for c := range last {
		companies = append(companies, c)
	}
	sort.Strings(companies)
	for _, c := range companies {
		t.rj.Accept("smtpsettings")
		if err := t.w.row("smtp_settings", last[c].row); err != nil {
			return err
		}
	}
	return nil
}

// epiasStep is R420.
func (t *transformer) epiasStep() error {
	type yk struct {
		date  string
		value decimal.Decimal
	}
	hourly := map[time.Time]decimal.Decimal{}
	yekdem := map[[2]int]yk{}
	if err := ReadExtract(t.dir, "epiashistories", func(d bson.M) error {
		t.count("epiashistories")
		hex := hexID(d["_id"])
		day, err := time.ParseInLocation(time.DateOnly, str(d, "date"), istanbul)
		if err != nil {
			t.rj.Reject("epiashistories", hex, "date", "bad_date", str(d, "date"))
			return nil
		}
		if y, _ := num(d["yekdem"]); y != nil {
			key := [2]int{day.Year(), int(day.Month())}
			if prev, ok := yekdem[key]; ok && !prev.value.Equal(*y) {
				t.notes["yekdem_conflicts"]++
			}
			if prev, ok := yekdem[key]; !ok || str(d, "date") >= prev.date {
				yekdem[key] = yk{str(d, "date"), *y}
			}
		}
		hour, _ := num(d["hour"])
		if hour == nil {
			t.rj.Accept("epiashistories") // a daily average: its YEKDEM is used, its PTF is derivable
			return nil
		}
		ptf, _ := num(d["ptf"])
		if ptf == nil || hour.IntPart() < 0 || hour.IntPart() > 23 {
			t.rj.Reject("epiashistories", hex, "ptf", "required", nil)
			return nil
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), int(hour.IntPart()), 0, 0, 0, istanbul).UTC()
		if _, dup := hourly[at]; dup {
			t.rj.Reject("epiashistories", hex, "hour", "duplicate_last_wins", nil)
		} else {
			t.rj.Accept("epiashistories")
		}
		hourly[at] = *ptf
		return nil
	}); err != nil {
		return err
	}
	hours := make([]time.Time, 0, len(hourly))
	for h := range hourly {
		hours = append(hours, h)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i].Before(hours[j]) })
	for _, h := range hours {
		if err := t.w.row("market_prices_hourly", map[string]any{"ts": ts(h), "ptf": hourly[h].String()}); err != nil {
			return err
		}
	}
	months := make([][2]int, 0, len(yekdem))
	for m := range yekdem {
		months = append(months, m)
	}
	sort.Slice(months, func(i, j int) bool { return months[i][0]*100+months[i][1] < months[j][0]*100+months[j][1] })
	for _, m := range months {
		if err := t.w.row("yekdem_monthly", map[string]any{"year": m[0], "month": m[1], "value": yekdem[m].value.String()}); err != nil {
			return err
		}
	}
	return nil
}
