package legacy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// Reason attributes a difference (08 §8 plus F14c Q-K5).
type Reason string

// The documented reasons; anything else is Unexplained, which blocks cutover.
const (
	ReasonDistributionOnTotal Reason = "distribution_on_total_consumption"
	ReasonNoTieringMultiTime  Reason = "no_tiering_on_multi_time"
	ReasonMeterResetWithheld  Reason = "meter_reset_withheld"
	ReasonReactiveBelow9kW    Reason = "reactive_exempt_below_9kw"
	ReasonVATRateCorrected    Reason = "vat_rate_corrected"
	ReasonPTFGapFlagged       Reason = "ptf_gap_flagged"
	ReasonCapacityRemoved     Reason = "capacity_cost_removed"
	ReasonMonomialPower       Reason = "monomial_power_charge_dropped"
	ReasonReactiveNotApplies  Reason = "reactive_not_applicable"
	ReasonAutomatedCarbon     Reason = "automated_carbon_recomputed"
	ReasonMultiplierPending   Reason = "multiplier_unconfirmed"
	ReasonDiscarded           Reason = "derived_discarded"
	Unexplained               Reason = "unexplained"
)

// Checks (08 §8).
const (
	CheckRowCounts          = "row_counts"
	CheckReadings           = "readings_per_analyzer_month"
	CheckAnalyzerKwh        = "monthly_consumption_per_analyzer"
	CheckAnalyzerInvoice    = "monthly_invoice_per_analyzer"
	CheckBuildingInvoice    = "monthly_invoice_per_building"
	CheckReactiveApplied    = "reactive_penalty_applied"
	CheckBuildingAnnualKwh  = "annual_consumption_per_building"
	CheckBuildingAnnualCost = "annual_invoice_per_building"
	CheckCarbon             = "carbon_per_building_year"
	CheckArtifacts          = "artifacts"
)

// Row is one out-of-tolerance figure (or a global count line).
type Row struct {
	Company string           `json:"company,omitempty"`
	Check   string           `json:"check"`
	Subject string           `json:"subject"`
	Period  string           `json:"period"`
	Legacy  *decimal.Decimal `json:"legacy"`
	New     *decimal.Decimal `json:"new"`
	Diff    *decimal.Decimal `json:"diff"`
	Pct     *decimal.Decimal `json:"pct"`
	Reasons []Reason         `json:"reasons"`
}

// Blocks is true when the row carries an unexplained difference.
func (r Row) Blocks() bool {
	for _, x := range r.Reasons {
		if x == Unexplained {
			return true
		}
	}
	return false
}

// CompanyReport is one company's section (R442).
type CompanyReport struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Pass    bool           `json:"pass"`
	Reasons map[Reason]int `json:"reasons"`
	Rows    []Row          `json:"rows"`
}

// Report is the whole reconciliation.
type Report struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Pass        bool            `json:"pass"`
	Global      []Row           `json:"global"`
	Companies   []CompanyReport `json:"companies"`
}

// Inputs are everything reconcile compares; the CLI gathers them.
type Inputs struct {
	Now          time.Time
	LegacyBills  []admin.LegacyBillRow
	LiveBills    []admin.LiveBillRow
	Anomalies    []admin.Anomaly
	Names        admin.Names
	Readings     map[uuid.UUID]map[string]int // database, per Istanbul month
	Expected     map[uuid.UUID]map[string]int // the transform's meter_readings lines
	Withheld     map[uuid.UUID]bool
	CarbonNew    map[uuid.UUID]map[int]decimal.Decimal
	CarbonLegacy map[uuid.UUID]map[int]decimal.Decimal
	CarbonAuto   map[uuid.UUID]map[int]bool // years with legacy automated accruals
	Global       []Row                      // row counts and artifacts, built by the CLI
}

var (
	hundred      = decimal.NewFromInt(100)
	kwhTolerance = decimal.RequireFromString("0.1")
	tlTolerance  = decimal.NewFromInt(1)
	minComponent = decimal.RequireFromString("0.5")
	nearZero     = decimal.RequireFromString("0.01")
)

func dptr(d decimal.Decimal) *decimal.Decimal { return &d }

// compare returns the row's figures and whether they are out of tolerance (R436).
func compare(legacy, now decimal.Decimal, pct decimal.Decimal) (Row, bool) {
	diff := now.Sub(legacy)
	r := Row{Legacy: dptr(legacy), New: dptr(now), Diff: dptr(diff.Round(4))}
	if legacy.IsZero() {
		return r, diff.Abs().GreaterThan(nearZero)
	}
	p := diff.Div(legacy.Abs()).Mul(hundred).Round(3)
	r.Pct = &p
	return r, p.Abs().GreaterThan(pct)
}

type billKey struct {
	scope   string
	subject uuid.UUID
	period  string
}

func legacySubject(b admin.LegacyBillRow) uuid.UUID {
	if b.AnalyzerID != nil {
		return *b.AnalyzerID
	}
	return b.BuildingID
}

func liveSubject(b admin.LiveBillRow) uuid.UUID {
	if b.AnalyzerID != nil {
		return *b.AnalyzerID
	}
	return b.BuildingID
}

func val(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}

// Reconcile is 08 §8.
func Reconcile(in Inputs) Report {
	live := map[billKey]admin.LiveBillRow{}
	for _, b := range in.LiveBills {
		live[billKey{b.Scope, liveSubject(b), b.Period}] = b
	}
	rows := map[uuid.UUID][]Row{}
	add := func(company uuid.UUID, r Row) {
		r.Company = company.String()
		rows[company] = append(rows[company], r)
	}
	type annual struct {
		legacyKwh, newKwh, legacyCost, newCost decimal.Decimal
		reasons                                map[Reason]bool
		company                                uuid.UUID
	}
	years := map[string]*annual{}
	for _, lb := range in.LegacyBills {
		subject := legacySubject(lb)
		name := in.Names.Buildings[lb.BuildingID]
		if lb.AnalyzerID != nil {
			name += " / " + in.Names.Analyzers[*lb.AnalyzerID]
		}
		nb, ok := live[billKey{lb.Scope, subject, lb.Period}]
		missing := in.missingReasons(lb, nb, ok)
		var yr *annual
		if lb.Scope == "building" {
			key := lb.BuildingID.String() + ":" + lb.Period[:4]
			if years[key] == nil {
				years[key] = &annual{reasons: map[Reason]bool{}, company: lb.CompanyID}
			}
			yr = years[key]
			yr.legacyKwh = yr.legacyKwh.Add(val(lb.TotalActiveKwh))
			yr.legacyCost = yr.legacyCost.Add(val(lb.TotalCost))
		}
		if missing != nil {
			r := Row{Check: CheckBuildingInvoice, Subject: name, Period: lb.Period, Legacy: lb.TotalCost, Reasons: missing}
			if lb.Scope == "analyzer" {
				r.Check = CheckAnalyzerInvoice
			}
			add(lb.CompanyID, r)
			if yr != nil {
				for _, x := range missing {
					yr.reasons[x] = true
				}
			}
			continue
		}
		if yr != nil {
			yr.newKwh = yr.newKwh.Add(nb.ActiveImport)
			yr.newCost = yr.newCost.Add(nb.TotalCost)
		}
		if lb.Scope == "analyzer" {
			if r, out := compare(val(lb.TotalActiveKwh), nb.ActiveImport, kwhTolerance); out {
				r.Check, r.Subject, r.Period, r.Reasons = CheckAnalyzerKwh, name, lb.Period, []Reason{Unexplained}
				add(lb.CompanyID, r)
			}
		}
		if r, out := compare(val(lb.TotalCost), nb.TotalCost, tlTolerance); out {
			r.Check, r.Subject, r.Period = CheckBuildingInvoice, name, lb.Period
			if lb.Scope == "analyzer" {
				r.Check = CheckAnalyzerInvoice
			}
			r.Reasons = Attribute(lb, nb)
			add(lb.CompanyID, r)
			if yr != nil {
				for _, x := range r.Reasons {
					yr.reasons[x] = true
				}
			}
		}
		if lb.ReactivePenaltyApplied != nil && *lb.ReactivePenaltyApplied != nb.ReactivePenaltyApplied {
			reason := reactiveReason(nb)
			if nb.ReactivePenaltyApplied {
				reason = Unexplained // the rewrite never penalises where legacy did not
			}
			add(lb.CompanyID, Row{Check: CheckReactiveApplied, Subject: name, Period: lb.Period, Reasons: []Reason{reason}})
		}
	}
	keys := make([]string, 0, len(years))
	for k := range years {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		y := years[k]
		building, year := uuid.MustParse(k[:36]), k[37:]
		reasons := sortedReasons(y.reasons)
		if len(reasons) == 0 {
			reasons = []Reason{Unexplained}
		}
		if r, out := compare(y.legacyKwh, y.newKwh, kwhTolerance); out {
			r.Check, r.Subject, r.Period, r.Reasons = CheckBuildingAnnualKwh, in.Names.Buildings[building], year, reasons
			add(y.company, r)
		}
		if r, out := compare(y.legacyCost, y.newCost, tlTolerance); out {
			r.Check, r.Subject, r.Period, r.Reasons = CheckBuildingAnnualCost, in.Names.Buildings[building], year, reasons
			add(y.company, r)
		}
	}
	in.readingRows(add)
	in.carbonRows(add)
	return in.assemble(rows)
}

func sortedReasons(set map[Reason]bool) []Reason {
	out := make([]Reason, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// missingReasons explains a legacy bill with no comparable new bill; nil means comparable.
func (in Inputs) missingReasons(lb admin.LegacyBillRow, nb admin.LiveBillRow, ok bool) []Reason {
	if ok && nb.FlagReason != nil && containsFlag(*nb.FlagReason, "ptf_data_missing") {
		return []Reason{ReasonPTFGapFlagged}
	}
	if ok {
		return nil
	}
	from, to := periodOf(lb)
	for _, a := range in.Anomalies {
		inScope := (lb.AnalyzerID != nil && a.AnalyzerID == *lb.AnalyzerID) ||
			(lb.AnalyzerID == nil && in.Names.AnalyzerBuilding[a.AnalyzerID] == lb.BuildingID)
		if inScope && a.From.Before(to) && a.To.After(from) {
			return []Reason{ReasonMeterResetWithheld}
		}
	}
	return []Reason{Unexplained}
}

func containsFlag(reason, flag string) bool { return strings.Contains(reason, flag) }

func periodOf(lb admin.LegacyBillRow) (time.Time, time.Time) {
	if lb.StartDate != nil && lb.EndDate != nil {
		return *lb.StartDate, lb.EndDate.AddDate(0, 0, 1)
	}
	m, err := time.ParseInLocation("2006-01", lb.Period, istanbul)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	return m, m.AddDate(0, 1, 0)
}

// legacyPayload are the bill-history fields attribution reads.
type legacyPayload struct {
	IsTwoPartCalculation bool            `json:"isTwoPartCalculation"`
	NormalConsumption    json.RawMessage `json:"normalConsumption"`
}

func (p legacyPayload) normal() *decimal.Decimal {
	var v any
	if json.Unmarshal(p.NormalConsumption, &v) != nil || v == nil {
		return nil
	}
	d, err := decimal.NewFromString(fmt.Sprint(v))
	if err != nil {
		return nil
	}
	return &d
}

// Attribute is R439: every differing component must map to a documented reason.
func Attribute(lb admin.LegacyBillRow, nb admin.LiveBillRow) []Reason {
	var p legacyPayload
	_ = json.Unmarshal(lb.Payload, &p)
	threshold := decimal.Max(minComponent, val(lb.TotalCost).Abs().Mul(decimal.RequireFromString("0.005")))
	differs := func(legacy, now decimal.Decimal) bool { return now.Sub(legacy).Abs().GreaterThan(threshold) }
	str := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	set := map[Reason]bool{}
	if differs(val(lb.EnergyCost), nb.EnergyCost) {
		switch {
		case str(nb.PriceType) == "multi_time" && p.IsTwoPartCalculation:
			set[ReasonNoTieringMultiTime] = true
		case nb.FlagReason != nil && containsFlag(*nb.FlagReason, "ptf_data_missing"):
			set[ReasonPTFGapFlagged] = true
		default:
			set[Unexplained] = true
		}
	}
	if differs(val(lb.DistributionCost), nb.DistributionCost) {
		n := p.normal()
		if n != nil && nb.TariffDistributionPrice != nil && !near(val(lb.DistributionCost), n.Mul(*nb.TariffDistributionPrice)).GreaterThan(decimal.NewFromInt(1)) {
			set[ReasonDistributionOnTotal] = true
		} else {
			set[Unexplained] = true
		}
	}
	if differs(val(lb.GreenEnergyCost), nb.GreenEnergyCost) {
		set[Unexplained] = true
	}
	if val(lb.CapacityCost).GreaterThan(threshold) {
		set[ReasonCapacityRemoved] = true
	}
	if differs(val(lb.PowerCost), nb.PowerCost.Add(nb.DemandOverrunCost)) {
		if str(nb.Term) == "monomial" && val(lb.PowerCost).IsPositive() && nb.PowerCost.IsZero() {
			set[ReasonMonomialPower] = true
		} else {
			set[Unexplained] = true
		}
	}
	if differs(val(lb.ReactivePenalty), nb.ReactivePenalty) {
		if val(lb.ReactivePenalty).IsPositive() && nb.ReactivePenalty.IsZero() {
			set[reactiveReason(nb)] = true
		} else {
			set[Unexplained] = true
		}
	}
	if nb.TariffVatRate != nil {
		base := val(lb.TotalCost).Sub(val(lb.VatCost))
		if base.IsPositive() {
			implied := val(lb.VatCost).Div(base).Mul(hundred)
			if implied.Sub(*nb.TariffVatRate).Abs().GreaterThan(decimal.RequireFromString("0.1")) {
				set[ReasonVATRateCorrected] = true
			}
		}
	}
	if len(set) == 0 {
		set[Unexplained] = true // the total differs and no component explains it
	}
	return sortedReasons(set)
}

// near is |a−b| as a percentage of |b| (0 when both are 0).
func near(a, b decimal.Decimal) decimal.Decimal {
	if b.IsZero() {
		if a.IsZero() {
			return decimal.Zero
		}
		return hundred
	}
	return a.Sub(b).Abs().Div(b.Abs()).Mul(hundred)
}

// reactiveReason explains a penalty legacy charged and the rewrite does not (02 §6.6).
func reactiveReason(nb admin.LiveBillRow) Reason {
	str := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	switch {
	case str(nb.Term) == "monomial", str(nb.UserGroup) == "residential", str(nb.UserGroup) == "lighting", nb.ActiveExport.GreaterThan(decimal.NewFromInt(1)):
		return ReasonReactiveNotApplies
	case nb.InstalledPower != nil && nb.InstalledPower.LessThan(decimal.NewFromInt(9)):
		return ReasonReactiveBelow9kW
	}
	return Unexplained
}

func (in Inputs) readingRows(add func(uuid.UUID, Row)) {
	analyzers := map[uuid.UUID]bool{}
	for a := range in.Expected {
		analyzers[a] = true
	}
	for a := range in.Readings {
		analyzers[a] = true
	}
	ids := make([]uuid.UUID, 0, len(analyzers))
	for a := range analyzers {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, a := range ids {
		months := map[string]bool{}
		for m := range in.Expected[a] {
			months[m] = true
		}
		for m := range in.Readings[a] {
			months[m] = true
		}
		sorted := make([]string, 0, len(months))
		for m := range months {
			sorted = append(sorted, m)
		}
		sort.Strings(sorted)
		building := in.Names.AnalyzerBuilding[a]
		company := in.Names.BuildingCompany[building]
		for _, m := range sorted {
			want, got := in.Expected[a][m], in.Readings[a][m]
			if want == got {
				continue
			}
			reason := Unexplained
			if in.Withheld[a] && got == 0 {
				reason = ReasonMultiplierPending
			}
			add(company, Row{Check: CheckReadings, Subject: in.Names.Buildings[building] + " / " + in.Names.Analyzers[a], Period: m,
				Legacy: dptr(decimal.NewFromInt(int64(want))), New: dptr(decimal.NewFromInt(int64(got))),
				Diff: dptr(decimal.NewFromInt(int64(got - want))), Reasons: []Reason{reason}})
		}
	}
}

func (in Inputs) carbonRows(add func(uuid.UUID, Row)) {
	buildings := map[uuid.UUID]bool{}
	for b := range in.CarbonLegacy {
		buildings[b] = true
	}
	for b := range in.CarbonNew {
		buildings[b] = true
	}
	ids := make([]uuid.UUID, 0, len(buildings))
	for b := range buildings {
		ids = append(ids, b)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, b := range ids {
		years := map[int]bool{}
		for y := range in.CarbonLegacy[b] {
			years[y] = true
		}
		for y := range in.CarbonNew[b] {
			years[y] = true
		}
		sorted := make([]int, 0, len(years))
		for y := range years {
			sorted = append(sorted, y)
		}
		sort.Ints(sorted)
		for _, y := range sorted {
			r, out := compare(in.CarbonLegacy[b][y], in.CarbonNew[b][y], tlTolerance)
			if !out {
				continue
			}
			r.Check, r.Subject, r.Period, r.Reasons = CheckCarbon, in.Names.Buildings[b], strconv.Itoa(y), []Reason{Unexplained}
			if in.CarbonAuto[b][y] {
				r.Reasons = []Reason{ReasonAutomatedCarbon}
			}
			add(in.Names.BuildingCompany[b], r)
		}
	}
}

func (in Inputs) assemble(rows map[uuid.UUID][]Row) Report {
	rep := Report{GeneratedAt: in.Now, Pass: true, Global: in.Global}
	for _, r := range rep.Global {
		if r.Blocks() {
			rep.Pass = false
		}
	}
	companies := map[uuid.UUID]bool{}
	for c := range in.Names.Companies {
		companies[c] = true
	}
	for c := range rows {
		companies[c] = true
	}
	ids := make([]uuid.UUID, 0, len(companies))
	for c := range companies {
		ids = append(ids, c)
	}
	sort.Slice(ids, func(i, j int) bool {
		return in.Names.Companies[ids[i]]+ids[i].String() < in.Names.Companies[ids[j]]+ids[j].String()
	})
	for _, c := range ids {
		cr := CompanyReport{ID: c.String(), Name: in.Names.Companies[c], Pass: true, Reasons: map[Reason]int{}, Rows: rows[c]}
		if cr.Rows == nil {
			cr.Rows = []Row{}
		}
		for _, r := range cr.Rows {
			for _, x := range r.Reasons {
				cr.Reasons[x]++
			}
			if r.Blocks() {
				cr.Pass, rep.Pass = false, false
			}
		}
		rep.Companies = append(rep.Companies, cr)
	}
	return rep
}
