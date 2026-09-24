package legacy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

// tariffClass is what billing needs and embedded legacy tariffs never stored (Q-J7).
type tariffClass struct {
	voltage model.VoltageLevel
	group   model.DistributionUserGroup
	term    model.TariffTerm
	supply  model.SupplyCompany
}

func (c tariffClass) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", c.voltage, c.group, c.term, c.supply)
}

var (
	userGroups = map[string]model.DistributionUserGroup{
		"residental":       model.UserGroupResidential, //nolint:misspell // the legacy enum value is spelled this way "residential": model.UserGroupResidential, "residentialplus": model.UserGroupResidentialPlus,
		"residential_plus": model.UserGroupResidentialPlus, "commercial": model.UserGroupCommercial, "commercialplus": model.UserGroupCommercialPlus,
		"commercial_plus": model.UserGroupCommercialPlus, "industrial": model.UserGroupIndustrial, "agricultural": model.UserGroupAgricultural,
		"lighting": model.UserGroupLighting, "martyrsfamilies": model.UserGroupMartyrsFamilies, "martyrs_families": model.UserGroupMartyrsFamilies,
		"publiclighting": model.UserGroupPublicLighting, "public_lighting": model.UserGroupPublicLighting,
	}
	voltages  = map[string]model.VoltageLevel{"ag": model.VoltageLevelLV, "lv": model.VoltageLevelLV, "og": model.VoltageLevelMV, "mv": model.VoltageLevelMV}
	terms     = map[string]model.TariffTerm{"monomial": model.TariffTermMonomial, "binomial": model.TariffTermBinomial}
	supplies  = map[string]model.SupplyCompany{"attendant_company": model.SupplyCompanyIncumbent, "incumbent": model.SupplyCompanyIncumbent, "private_company": model.SupplyCompanyPrivate, "private": model.SupplyCompanyPrivate}
	currencis = map[string]model.CurrencyCode{"tl": model.CurrencyTRY, "try": model.CurrencyTRY, "usd": model.CurrencyUSD, "eur": model.CurrencyEUR}
)

// classOf reads a standalone tariff's classification; ok is false when any part is missing or unknown.
func classOf(d bson.M) (tariffClass, bool) {
	c := tariffClass{voltages[fold(str(d, "distribution_type"))], userGroups[fold(str(d, "distribution_system_user"))],
		terms[fold(str(d, "term"))], supplies[fold(str(d, "supply_company"))]}
	return c, c.voltage != "" && c.group != "" && c.term != "" && c.supply != ""
}

// parseClass reads an operator answer "lv/commercial/monomial/private".
func parseClass(s string) (tariffClass, bool) {
	p := strings.Split(strings.ToLower(strings.TrimSpace(s)), "/")
	if len(p) != 4 {
		return tariffClass{}, false
	}
	c := tariffClass{voltages[p[0]], userGroups[strings.ReplaceAll(p[1], " ", "")], terms[p[2]], supplies[p[3]]}
	return c, c.voltage != "" && c.group != "" && c.term != "" && c.supply != ""
}

// tariffCandidate is one legacy tariff for a (building, effective date).
type tariffCandidate struct {
	collection, legacyID string // "building_tariffs" (hex:index) or "tariffs" (hex)
	doc                  bson.M
	date                 string
	class                tariffClass
	hasClass             bool
}

// priceFields counts the stated price fields: the "more complete record" of 08 §5.
func (c tariffCandidate) priceFields() int {
	n := 0
	price := asDoc(c.doc["price"])
	for _, v := range price {
		switch x := v.(type) {
		case nil:
		case bson.D, bson.M:
			for _, inner := range asDoc(x) {
				if inner != nil {
					n++
				}
			}
		default:
			n++
		}
	}
	for _, v := range asDoc(c.doc["kbk"]) {
		if v != nil {
			n++
		}
	}
	return n
}

type standalone struct {
	hex   string
	doc   bson.M
	class tariffClass
	ok    bool
}

// tariffsStep is R417: embedded history plus standalone tariffs, merged per
// (building, effective date), classified per Q-J7, validated by the API's rules.
func (t *transformer) tariffsStep() error {
	byBuilding := map[string][]standalone{}
	byHex := map[string]standalone{}
	if err := ReadExtract(t.dir, "tariffs", func(d bson.M) error {
		t.count("tariffs")
		hex := hexID(d["_id"])
		if strings.EqualFold(str(d, "energy_source"), "solar") || d["powerPlant"] != nil {
			t.solarTariffDocs = append(t.solarTariffDocs, d) // R424: accounted in plantsStep
			return nil
		}
		building := hexID(d["building"])
		if building == "" {
			t.rj.Reject("tariffs", hex, "building", "no_building", nil) // Q-J8
			return nil
		}
		if _, ok := t.buildingCompany[building]; !ok {
			t.rj.Reject("tariffs", hex, "building", "building_unknown", building)
			return nil
		}
		c, ok := classOf(d)
		s := standalone{hex, d, c, ok}
		byBuilding[building] = append(byBuilding[building], s)
		byHex[hex] = s
		return nil
	}); err != nil {
		return err
	}
	return ReadExtract(t.dir, "buildings", func(b bson.M) error {
		hex := hexID(b["_id"])
		entries := asArray(b["tariffs"])
		if cur := asDoc(b["tariff"]); cur != nil {
			dup := false
			for _, e := range entries {
				if str(asDoc(e), "effectiveFrom") == str(cur, "effectiveFrom") {
					dup = true
				}
			}
			if !dup {
				entries = append(entries, cur)
			}
		}
		company, known := t.buildingCompany[hex]
		groups := map[string][]tariffCandidate{}
		for i, e := range entries {
			t.count("building_tariffs")
			ref := fmt.Sprintf("%s:%d", hex, i)
			if !known {
				t.rj.Reject("building_tariffs", ref, "building", "building_rejected", hex)
				continue
			}
			doc := asDoc(e)
			day, err := ParseDate(str(doc, "effectiveFrom"))
			if err != nil {
				t.rj.Reject("building_tariffs", ref, "effectiveFrom", reasonOf(err), str(doc, "effectiveFrom"))
				continue
			}
			date := civilDate(day)
			groups[date] = append(groups[date], tariffCandidate{collection: "building_tariffs", legacyID: ref, doc: doc, date: date})
		}
		if !known {
			return nil
		}
		for _, s := range byBuilding[hex] {
			day, err := ParseDate(str(s.doc, "effectiveFrom"))
			if err != nil {
				t.rj.Reject("tariffs", s.hex, "effectiveFrom", reasonOf(err), str(s.doc, "effectiveFrom"))
				continue
			}
			date := civilDate(day)
			groups[date] = append(groups[date], tariffCandidate{collection: "tariffs", legacyID: s.hex, doc: s.doc, date: date, class: s.class, hasClass: s.ok})
		}
		dates := make([]string, 0, len(groups))
		for d := range groups {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		for _, date := range dates {
			if err := t.writeBuildingTariff(company, hex, groups[date], byBuilding[hex], byHex); err != nil {
				return err
			}
		}
		return nil
	})
}

func reasonOf(err error) string {
	switch {
	case errors.Is(err, ErrAmbiguousDate):
		return "ambiguous_date"
	default:
		return "bad_date"
	}
}

func (t *transformer) writeBuildingTariff(company, building string, cands []tariffCandidate, standalones []standalone, byHex map[string]standalone) error {
	win := 0
	for i, c := range cands {
		if c.priceFields() > cands[win].priceFields() || (c.priceFields() == cands[win].priceFields() && c.collection == "building_tariffs" && cands[win].collection != "building_tariffs") {
			win = i
		}
	}
	w := cands[win]
	// Q-J7: same key → originalTariffId → one classification for the building → answer.
	class, ok := tariffClass{}, false
	for _, c := range cands {
		if c.hasClass {
			class, ok = c.class, true
		}
	}
	if !ok {
		if s, found := byHex[str(w.doc, "originalTariffId")]; found && s.ok {
			class, ok = s.class, true
		}
	}
	if !ok {
		seen := map[tariffClass]bool{}
		for _, s := range standalones {
			if s.ok {
				seen[s.class] = true
			}
		}
		if len(seen) == 1 {
			for c := range seen {
				class, ok = c, true
			}
		}
	}
	if !ok {
		if a := t.answer(AnswerTariffClass, building); a != "" {
			if class, ok = parseClass(a); !ok {
				return fmt.Errorf("legacy: answers.csv: tariff_class %s: %q is not lv|mv/<group>/<term>/<supply>", building, a)
			}
		}
	}
	for i, c := range cands {
		if i != win {
			t.rj.Reject(c.collection, c.legacyID, "effectiveFrom", "duplicate_merged", w.legacyID)
		}
	}
	if !ok {
		t.ask(AnswerTariffClass, building, "voltage/user group/term/supply company unknown for its tariffs")
		t.rj.Reject(w.collection, w.legacyID, "classification", "classification_unknown", building)
		return nil
	}
	id := ID(w.collection, w.legacyID)
	draft, reason, value, err := t.tariffDraft(w.collection, w.legacyID, w.doc, class)
	if err != nil {
		return err
	}
	if reason != "" {
		t.rj.Reject(w.collection, w.legacyID, "price", reason, value)
		return nil
	}
	draft.Tariff.ID, draft.Tariff.CompanyID = id, ID("companies", company)
	b := ID("buildings", building)
	draft.Tariff.BuildingID = &b
	draft.Tariff.EffectiveFrom = dateOf(w.date)
	valid, err := tariff.Validate(draft)
	if err != nil {
		t.rj.Reject(w.collection, w.legacyID, "tariff", "invalid_tariff", err.Error())
		return nil
	}
	t.rj.Accept(w.collection)
	if err := t.w.row("tariffs", tariffRow(valid, w.date)); err != nil {
		return err
	}
	for i, tax := range draft.Taxes {
		if err := t.w.row("tariff_taxes", map[string]any{"id": ID("tariff_taxes", fmt.Sprintf("%s:%d", id, i)).String(), "tariff_id": id.String(),
			"name": tax.Name, "rate": tax.Rate.String(), "sort_order": i}); err != nil {
			return err
		}
	}
	for _, y := range draft.ManualYekdem {
		if err := t.w.row("tariff_manual_yekdem", map[string]any{"tariff_id": id.String(), "year": y.Year, "month": y.Month, "value": y.Value.String()}); err != nil {
			return err
		}
	}
	return t.w.legacyID(w.collection, w.legacyID, "tariffs", id)
}

// dateOf is a YYYY-MM-DD civil date as the midnight the date columns carry.
func dateOf(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

// tariffDraft maps a legacy tariff (embedded or standalone: same price shape)
// onto the API's draft. A non-empty reason is a rejection.
func (t *transformer) tariffDraft(collection, legacyID string, d bson.M, class tariffClass) (tariff.Draft, string, any, error) {
	price, kbk := asDoc(d["price"]), asDoc(d["kbk"])
	n := func(doc bson.M, key string) *decimal.Decimal {
		v, err := num(doc[key])
		if err != nil {
			return nil
		}
		return v
	}
	cur, ok := currencis[fold(str(d, "currency"))]
	if !ok {
		return tariff.Draft{}, "unknown_currency", str(d, "currency"), nil
	}
	vat := n(price, "vat_rate")
	if vat == nil {
		return tariff.Draft{}, "vat_rate_missing", nil, nil
	}
	ptf, _ := d["usePtfYekdem"].(bool)
	if ptf && n(kbk, "energyKbk") == nil {
		return tariff.Draft{}, "kbk_energy_missing", nil, nil
	}
	dist, reactive := n(price, "distribution_cost"), n(price, "reactive_power_price")
	if dist == nil || reactive == nil {
		return tariff.Draft{}, "required", "distribution_cost/reactive_power_price", nil
	}
	tr := model.Tariff{Currency: cur, EnergyType: model.EnergyTypeGrid, VoltageLevel: class.voltage, UserGroup: class.group,
		PriceType: model.PriceType(str(d, "price_type")), Term: class.term, SupplyCompany: class.supply,
		SingleTimePrice: n(price, "single_time_price"), OverusePrice: n(price, "overuse_price"),
		OveruseThresholdKwhPerDay: n(price, "overuse_threshold_kwh_per_day"), DistributionCost: *dist, ReactivePowerPrice: *reactive,
		GreenEnergyPrice: n(price, "green_energy_price"), GreenEnergyDistributionCost: n(price, "green_energy_distribution_cost"),
		ContractedPowerKw: n(price, "contracted_power"), PowerUnitPrice: n(price, "power_unit_price"),
		GenerationUsage: model.GenerationUsageNone, GenerationPricePerKwh: n(price, "generation_price_per_kwh"), UsePtfYekdem: ptf}
	if name := strings.TrimSpace(str(d, "tariff_name")); name != "" {
		tr.Name = &name
	}
	if et := str(d, "energy_type"); et == string(model.EnergyTypeGreen) || (et == "" && tr.GreenEnergyPrice != nil && tr.GreenEnergyPrice.IsPositive()) {
		tr.EnergyType = model.EnergyTypeGreen
	}
	if g := str(price, "generation_usage_type"); g != "" {
		tr.GenerationUsage = model.GenerationUsage(g)
	}
	if mt := asDoc(price["multi_time_price"]); mt != nil {
		tr.T1Price, tr.T2Price, tr.T3Price = n(mt, "t1"), n(mt, "t2"), n(mt, "t3")
	}
	// Legacy priced energy as single_time_price || power_price (bill/route.ts).
	if tr.PriceType == model.PriceTypeSingleTime && tr.SingleTimePrice == nil {
		tr.SingleTimePrice = n(price, "power_price")
	}
	if tr.Term == model.TariffTermMonomial && (tr.ContractedPowerKw != nil || tr.PowerUnitPrice != nil) {
		// R125: a monomial tariff has no power charge; legacy charged contracted × unit price regardless.
		if charged := tr.ContractedPowerKw != nil && tr.PowerUnitPrice != nil && tr.ContractedPowerKw.IsPositive() && tr.PowerUnitPrice.IsPositive(); charged {
			if err := t.note(collection, legacyID, "monomial_power_dropped", map[string]string{"contracted_power": tr.ContractedPowerKw.String(), "power_unit_price": tr.PowerUnitPrice.String()}); err != nil {
				return tariff.Draft{}, "", nil, err
			}
		}
		tr.ContractedPowerKw, tr.PowerUnitPrice = nil, nil
	}
	if ptf {
		tr.KbkEnergy, tr.KbkT1, tr.KbkT2, tr.KbkT3 = n(kbk, "energyKbk"), n(kbk, "t1Kbk"), n(kbk, "t2Kbk"), n(kbk, "t3Kbk")
		tr.KbkPowerPrice, tr.KbkOverusePrice, tr.KbkReactivePower = n(kbk, "powerPriceKbk"), n(kbk, "overusePriceKbk"), n(kbk, "reactivePowerKbk")
		tr.KbkDistributionCostTlPerKwh = n(kbk, "distributionCostTlPerKwh")
		tr.UseManualYekdem, _ = kbk["useManualYekdem"].(bool)
		src := func(v *decimal.Decimal) model.PriceSource {
			if v != nil {
				return model.PriceSourceKbk
			}
			return model.PriceSourceFixed
		}
		tr.PowerPriceSource, tr.ReactivePriceSource, tr.DistributionPriceSource = src(tr.KbkPowerPrice), src(tr.KbkReactivePower), src(tr.KbkDistributionCostTlPerKwh)
	}
	draft := tariff.Draft{Tariff: tr, VatRate: vat}
	for _, v := range asArray(price["allTaxes"]) {
		tax := asDoc(v)
		rate := n(tax, "rate")
		if strings.TrimSpace(str(tax, "name")) == "" || rate == nil {
			continue
		}
		draft.Taxes = append(draft.Taxes, model.TariffTax{Name: strings.TrimSpace(str(tax, "name")), Rate: *rate, SortOrder: int16(len(draft.Taxes))}) //nolint:gosec // a handful of taxes
	}
	if other := n(price, "other_taxes_rate"); len(draft.Taxes) == 0 && other != nil && other.IsPositive() {
		draft.Taxes = []model.TariffTax{{Name: "Diğer vergi ve fonlar", Rate: *other}} // 08 §5
	}
	for _, v := range asArray(kbk["manualYekdem"]) {
		e := asDoc(v)
		y, m, val := n(e, "year"), n(e, "month"), n(e, "value")
		if y == nil || m == nil || val == nil {
			continue
		}
		draft.ManualYekdem = append(draft.ManualYekdem, model.TariffManualYekdem{Year: int16(y.IntPart()), Month: int16(m.IntPart()), Value: *val}) //nolint:gosec // calendar values
	}
	return draft, "", nil, nil
}

func tariffRow(tr model.Tariff, date string) map[string]any {
	var building any
	if tr.BuildingID != nil {
		building = tr.BuildingID.String()
	}
	var name any
	if tr.Name != nil {
		name = *tr.Name
	}
	return map[string]any{"id": tr.ID.String(), "company_id": tr.CompanyID.String(), "building_id": building, "name": name, "effective_from": date,
		"currency": tr.Currency, "energy_type": tr.EnergyType, "voltage_level": tr.VoltageLevel, "user_group": tr.UserGroup, "price_type": tr.PriceType,
		"term": tr.Term, "supply_company": tr.SupplyCompany, "single_time_price": dec(tr.SingleTimePrice), "t1_price": dec(tr.T1Price),
		"t2_price": dec(tr.T2Price), "t3_price": dec(tr.T3Price), "overuse_price": dec(tr.OverusePrice),
		"overuse_threshold_kwh_per_day": dec(tr.OveruseThresholdKwhPerDay), "distribution_cost": tr.DistributionCost.String(),
		"reactive_power_price": tr.ReactivePowerPrice.String(), "green_energy_price": dec(tr.GreenEnergyPrice),
		"green_energy_distribution_cost": dec(tr.GreenEnergyDistributionCost), "contracted_power_kw": dec(tr.ContractedPowerKw),
		"power_unit_price": dec(tr.PowerUnitPrice), "generation_usage": tr.GenerationUsage, "generation_price_per_kwh": dec(tr.GenerationPricePerKwh),
		"vat_rate": tr.VatRate.String(), "use_ptf_yekdem": tr.UsePtfYekdem, "kbk_energy": dec(tr.KbkEnergy), "kbk_t1": dec(tr.KbkT1),
		"kbk_t2": dec(tr.KbkT2), "kbk_t3": dec(tr.KbkT3), "kbk_power_price": dec(tr.KbkPowerPrice), "kbk_overuse_price": dec(tr.KbkOverusePrice),
		"kbk_reactive_power": dec(tr.KbkReactivePower), "kbk_distribution_cost_tl_per_kwh": dec(tr.KbkDistributionCostTlPerKwh),
		"use_manual_yekdem": tr.UseManualYekdem, "power_price_source": tr.PowerPriceSource, "reactive_price_source": tr.ReactivePriceSource,
		"distribution_price_source": tr.DistributionPriceSource}
}
