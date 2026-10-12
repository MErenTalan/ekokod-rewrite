// Package invoicepdf renders a bill as a deterministic A4 PDF with embedded
// Go fonts (Turkish glyphs, BSD licence).
package invoicepdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

// Member is one analyzer rolled into a bill.
type Member struct {
	AnalyzerID         uuid.UUID
	Name               string
	InstallationNumber string
}

// Document is everything one invoice PDF prints.
type Document struct {
	Bill         model.Bill
	Lines        []model.BillLine
	Members      []Member
	CompanyName  string
	BuildingName string // empty for company scope
	// Tariff is the version that priced the bill; nil for a company bill.
	Tariff *model.Tariff
	// InstalledPowerKw is the members' summed kurulu güç, as billing used it.
	InstalledPowerKw *decimal.Decimal
	Locale           string // "tr" (default) | "en"
	// Uncompressed disables stream compression (tests inspect the font CMap).
	Uncompressed bool
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// invoice carries one render's state.
type invoice struct {
	painter
	d Document
	b model.Bill
}

// Render is deterministic: identical Documents give identical bytes.
func Render(d Document) ([]byte, error) {
	loc := normaliseLocale(d.Locale)
	f := fpdf.New("P", "mm", "A4", "")
	f.SetCompression(!d.Uncompressed)
	f.SetCatalogSort(true)
	f.SetCreationDate(d.Bill.ComputedAt)
	f.SetModificationDate(d.Bill.ComputedAt)
	f.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	f.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	f.AddUTF8FontFromBytes("GoMedium", "", gomedium.TTF)
	f.SetTitle(label(loc, "title")+" "+d.Bill.PeriodKey, true)
	f.SetMargins(margin, 14, margin)
	f.SetCellMargin(1.5)
	f.SetAutoPageBreak(true, breakAt)
	f.AliasNbPages("{nb}")

	inv := &invoice{painter: painter{f: f, loc: loc}, d: d, b: d.Bill}
	f.SetHeaderFuncMode(inv.pageHeader, false)
	f.SetFooterFunc(inv.pageFooter)
	f.AddPage()

	inv.masthead()
	inv.banners()
	inv.cards()
	inv.tiles()
	inv.lines()
	inv.details()
	inv.readings()
	inv.members()

	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, fmt.Errorf("invoicepdf: %w", err)
	}
	return buf.Bytes(), nil
}

// sep separates facts on one line (Go's middle dot sits too high).
const sep = "  •  "

func (v *invoice) money(x decimal.Decimal) string { return render.FormatMoney(x, v.loc) }
func (v *invoice) num(x decimal.Decimal, places int32) string {
	return render.FormatDecimal(x, places, v.loc)
}

func (v *invoice) optNum(x *decimal.Decimal, places int32) string {
	if x == nil {
		return "—"
	}
	return v.num(*x, places)
}

func (v *invoice) date(t time.Time) string { return t.In(istanbul).Format("02.01.2006") }

func (v *invoice) periodLine() string {
	return monthLabel(v.loc, v.b.PeriodKey) + sep + v.date(v.b.PeriodStart) + " – " + v.date(v.b.PeriodEnd) +
		sep + fmt.Sprint(v.b.DaysInPeriod) + " " + label(v.loc, "days")
}

// masthead: wordmark, title and period on the left; amount due on the right.
func (v *invoice) masthead() {
	f := v.f
	v.fill(brand)
	f.Rect(0, 0, pageW, 2.5, "F")
	f.RoundedRect(margin, 10.6, 4.6, 4.6, 1.2, "1234", "F")
	v.font("B", 12)
	v.color(brand)
	v.text(margin+5.2, 10, 40, 5.8, "ekokod", "L")
	v.font("B", 17)
	v.color(ink)
	v.text(margin-1.5, 18.5, 110, 9, v.upper(label(v.loc, "title")), "L")
	v.font("", 9)
	v.color(muted)
	v.text(margin-1.5, 27.5, 110, 5, v.periodLine(), "L")

	const bx, by, bw, bh = 124.0, 9.0, 74.0, 26.0
	v.fill(brand)
	f.RoundedRect(bx, by, bw, bh, 3, "1234", "F")
	v.font("B", 7.5)
	v.color(brandTint)
	v.text(bx+3, by+3, bw-6, 4.5, v.upper(label(v.loc, "amount_due")), "L")
	v.font("B", 19)
	v.color(white)
	v.text(bx+3, by+8.5, bw-6, 9, v.money(v.b.TotalCost), "L")
	v.font("", 7.5)
	v.color(brandTint)
	v.text(bx+3, by+19, bw-6, 4.5, label(v.loc, "vat_included")+sep+string(v.b.Currency), "L")
	f.SetY(by + bh + 5)
}

// pageHeader runs on every page; the first page has the masthead instead.
func (v *invoice) pageHeader() {
	if v.f.PageNo() == 1 {
		return
	}
	v.fill(brand)
	v.f.Rect(0, 0, pageW, 2.5, "F")
	v.font("", 7.5)
	v.color(muted)
	v.text(margin-1.5, 6, contentW, 4.5, label(v.loc, "title")+sep+monthLabel(v.loc, v.b.PeriodKey)+sep+v.d.CompanyName, "L")
	v.f.SetY(14)
}

func (v *invoice) pageFooter() {
	f := v.f
	v.stroke(rule)
	f.SetLineWidth(0.3)
	f.Line(margin, footerTop, margin+contentW, footerTop)
	v.font("", 7)
	v.color(subtle)
	v.text(margin-1.5, footerTop+1.5, 150, 4, label(v.loc, "footer_note"), "L")
	v.text(margin-1.5, footerTop+5.5, 150, 4, label(v.loc, "computed")+" "+v.b.ComputedAt.In(istanbul).Format("02.01.2006 15:04")+
		sep+label(v.loc, "bill_no")+" "+strings.ToUpper(v.b.ID.String()[:8]), "L")
	v.text(pageW-margin-40+1.5, footerTop+1.5, 40, 4, fmt.Sprintf("%s %d / {nb}", label(v.loc, "page"), f.PageNo()), "R")
}

// banners: the preview and missing-days notices.
func (v *invoice) banners() {
	f := v.f
	if v.b.Status == model.BillStatusFlagged {
		text := label(v.loc, "preview")
		if v.b.FlagReason != nil {
			text += sep + flagText(v.loc, *v.b.FlagReason)
		}
		y := f.GetY()
		v.fill(dangerTint)
		f.RoundedRect(margin, y, contentW, 8, 2, "1234", "F")
		v.font("B", 8.5)
		v.color(danger)
		v.text(margin+2, y, contentW-4, 8, text, "L")
		f.SetY(y + 10)
	}
	if len(v.b.MissingDays) > 0 {
		text := label(v.loc, "missing_days") + strings.Join(DayRanges(v.b.MissingDays), ", ")
		v.font("B", 8)
		lines := f.SplitText(text, contentW-8)
		h := float64(len(lines))*4.2 + 4
		y := f.GetY()
		v.fill(warningTint)
		f.RoundedRect(margin, y, contentW, h, 2, "1234", "F")
		v.color(warning)
		for i, l := range lines {
			v.text(margin+2, y+2+float64(i)*4.2, contentW-4, 4.2, l, "L")
		}
		f.SetY(y + h + 2)
	}
}

// cards: subscriber and tariff side by side.
func (v *invoice) cards() {
	b := v.b
	sub := [][2]string{{label(v.loc, "company"), v.d.CompanyName}}
	if v.d.BuildingName != "" {
		sub = append(sub, [2]string{label(v.loc, "building"), v.d.BuildingName})
	}
	sub = append(sub, [2]string{label(v.loc, "scope"), label(v.loc, "scope_"+string(b.Scope))})
	if b.Scope == model.BillScopeAnalyzer && len(v.d.Members) > 0 {
		m := v.d.Members[0]
		sub = append(sub, [2]string{label(v.loc, "meter"), strings.TrimSpace(m.InstallationNumber + "  " + m.Name)})
	} else if len(v.d.Members) > 0 {
		sub = append(sub, [2]string{label(v.loc, "meters"), fmt.Sprint(len(v.d.Members))})
	}
	if v.d.InstalledPowerKw != nil {
		sub = append(sub, [2]string{label(v.loc, "installed_power"), v.num(*v.d.InstalledPowerKw, 3) + " kW"})
	}

	var tar [][2]string
	if t := v.d.Tariff; t != nil {
		if t.Name != nil && *t.Name != "" {
			tar = append(tar, [2]string{label(v.loc, "tariff_name"), *t.Name})
		}
		tar = append(tar,
			[2]string{label(v.loc, "user_group"), label(v.loc, "ug:"+string(t.UserGroup))},
			[2]string{label(v.loc, "voltage_term"), label(v.loc, "vl:"+string(t.VoltageLevel)) + sep + label(v.loc, "term:"+string(t.Term))})
		pt := label(v.loc, "pt:"+string(t.PriceType))
		if t.UsePtfYekdem {
			pt += sep + label(v.loc, "ptf_yekdem")
		}
		tar = append(tar, [2]string{label(v.loc, "price_type"), pt}, [2]string{label(v.loc, "supply"), label(v.loc, "sc:"+string(t.SupplyCompany))})
		if t.ContractedPowerKw != nil {
			tar = append(tar, [2]string{label(v.loc, "contracted_power"), v.num(*t.ContractedPowerKw, 3) + " kW"})
		}
	} else if b.Scope == model.BillScopeCompany {
		tar = append(tar, [2]string{label(v.loc, "tariff"), label(v.loc, "per_building")})
	}
	if b.TariffEffectiveFrom != nil {
		tar = append(tar, [2]string{label(v.loc, "tariff_date"), b.TariffEffectiveFrom.Format("02.01.2006")})
	}

	h := cardHeight(max(len(sub), len(tar)))
	v.ensure(h)
	y := v.f.GetY()
	w := (contentW - gap) / 2
	v.card(margin, y, w, h, label(v.loc, "subscriber"), nil)
	v.kv(margin+1.5, y+9, w-3, 30, sub)
	v.card(margin+w+gap, y, w, h, label(v.loc, "tariff"), nil)
	v.kv(margin+w+gap+1.5, y+9, w-3, 30, tar)
	v.f.SetY(y + h + gap)
}

// tiles: net consumption, max demand and the two reactive ratios.
func (v *invoice) tiles() {
	const h = 23.0
	b := v.b
	v.ensure(h)
	y := v.f.GetY()
	w := (contentW - 3*3) / 4
	tile := func(i int, title, value, unit, sub string) float64 {
		x := margin + float64(i)*(w+3)
		v.stroke(rule)
		v.f.SetLineWidth(0.3)
		v.fill(wash)
		v.f.RoundedRect(x, y, w, h, 2, "1234", "FD")
		v.font("B", 6.8)
		v.color(subtle)
		v.text(x+1.5, y+2.5, w-3, 4, v.upper(title), "L")
		v.font("B", 12.5)
		v.color(ink)
		v.text(x+1.5, y+6.8, w-3, 7, value, "L")
		vw := v.f.GetStringWidth(value)
		v.font("", 7.5)
		v.color(muted)
		v.text(x+1.5+vw+1, y+7.9, w-vw-4, 6, unit, "L")
		v.font("", 7)
		v.color(muted)
		v.text(x+1.5, y+h-5.6, w-3, 4.5, sub, "L")
		return x
	}

	tile(0, label(v.loc, "net"), v.num(b.NetConsumption, 3), "kWh",
		label(v.loc, "import")+" "+v.num(b.ActiveImport, 0)+sep+label(v.loc, "export")+" "+v.num(b.ActiveExport, 0))

	demandSub := label(v.loc, "no_demand")
	if b.MaxDemandKw != nil {
		demandSub = ""
	}
	if t := v.d.Tariff; t != nil && t.ContractedPowerKw != nil {
		demandSub = label(v.loc, "contract") + " " + v.num(*t.ContractedPowerKw, 0) + " kW"
	}
	unit := "kW"
	if b.MaxDemandKw == nil {
		unit = ""
	}
	tile(1, label(v.loc, "max_demand"), v.optNum(b.MaxDemandKw, 3), unit, demandSub)

	ratio := func(i int, title string, r, limit *decimal.Decimal) {
		sub := label(v.loc, "no_limit")
		if limit != nil {
			sub = label(v.loc, "limit") + " " + Percent(v.loc, limit)
		}
		x := tile(i, title, Percent(v.loc, r), "", sub)
		v.bar(x+3, y+14.6, w-6, r, limit)
	}
	ratio(2, label(v.loc, "inductive_ratio"), b.InductiveRatio, b.InductiveThreshold)
	ratio(3, label(v.loc, "capacitive_ratio"), b.CapacitiveRatio, b.CapacitiveThreshold)
	v.f.SetY(y + h + 2)
}

// splitLines returns the lines above the VAT base in sort order, then the VAT
// and generation credit lines that print below it.
func splitLines(lines []model.BillLine) (body []model.BillLine, vat, credit *model.BillLine) {
	sorted := slices.Clone(lines)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].SortOrder < sorted[j].SortOrder })
	for i := range sorted {
		switch sorted[i].Code {
		case model.BillLineVat:
			vat = &sorted[i]
		case model.BillLineGenerationCredit:
			credit = &sorted[i]
		default:
			body = append(body, sorted[i])
		}
	}
	return body, vat, credit
}

func (v *invoice) lineCells(l model.BillLine) []string {
	qty := ""
	if l.Quantity != nil {
		qty = v.num(*l.Quantity, 3)
		if l.Unit != nil && *l.Unit != "" && *l.Unit != "%" {
			qty += " " + label(v.loc, "unit:"+*l.Unit)
		}
	}
	price := ""
	switch {
	case l.RatePct != nil:
		price = pct(v.loc, *l.RatePct, 2)
	case l.UnitPrice != nil:
		price = v.num(*l.UnitPrice, 6)
	}
	return []string{lineLabel(v.loc, l.Code), qty, price, v.money(l.Amount)}
}

// lines: the charge table, VAT base, VAT, credit and the amount due.
func (v *invoice) lines() {
	v.section(label(v.loc, "lines"))
	t := &table{p: &v.painter, widths: []float64{86, 36, 30, 34}, aligns: []string{"L", "R", "R", "R"},
		head: []string{label(v.loc, "item"), label(v.loc, "quantity"), label(v.loc, "unit_price"), label(v.loc, "amount")}}
	t.header()
	body, vat, credit := splitLines(v.d.Lines)
	for _, l := range body {
		if l.Amount.IsNegative() {
			t.add(v.lineCells(l), ink, ink, ink, brand)
			continue
		}
		t.add(v.lineCells(l))
	}
	t.end()

	sum := func(name, amount string, bold bool) {
		v.ensure(6)
		y := v.f.GetY()
		style := ""
		if bold {
			style = "M"
		}
		v.font(style, 8.5)
		v.color(muted)
		v.text(margin+92, y, 60, 6, name, "R")
		v.color(ink)
		v.text(margin+contentW-34, y, 34, 6, amount, "R")
		v.f.SetY(y + 6)
	}
	v.f.SetY(v.f.GetY() + 1)
	sum(label(v.loc, "vat_base"), v.money(v.b.VatBase), true)
	if vat != nil {
		name := label(v.loc, model.BillLineVat)
		if vat.RatePct != nil {
			name += " (" + pct(v.loc, *vat.RatePct, 0) + ")"
		}
		sum(name, v.money(vat.Amount), false)
	}
	if credit != nil {
		sum(label(v.loc, model.BillLineGenerationCredit), v.money(credit.Amount), false)
	}

	v.ensure(14)
	y := v.f.GetY() + 1.5
	v.fill(brand)
	v.f.RoundedRect(margin, y, contentW, 10, 2, "1234", "F")
	v.font("B", 9)
	v.color(white)
	v.text(margin+2, y, 90, 10, v.upper(label(v.loc, "amount_due")), "L")
	v.font("B", 12)
	v.text(margin+contentW-80, y, 78, 10, v.money(v.b.TotalCost)+" "+string(v.b.Currency), "R")
	v.f.SetY(y + 11.5)
	v.font("", 6.5)
	v.color(subtle)
	v.f.SetX(margin)
	v.f.MultiCell(contentW, 3.4, label(v.loc, "footnote"), "", "L", false)
	v.f.SetY(v.f.GetY() + 2)
}

// reactiveStatus is the reactive card's chip key.
func reactiveStatus(b model.Bill, lines []model.BillLine) string {
	flagged := func(f string) bool {
		return b.FlagReason != nil && slices.Contains(strings.Split(*b.FlagReason, ","), f)
	}
	switch {
	case b.Scope == model.BillScopeCompany && b.ReactivePenaltyApplied:
		return "st:company_charged"
	case b.Scope == model.BillScopeCompany:
		return "st:company_none"
	case slices.ContainsFunc(lines, func(l model.BillLine) bool { return l.Code == model.BillLineReactiveWaiver }):
		return "st:waived"
	case b.ReactivePenaltyApplied:
		return "st:charged"
	case flagged("reactive_data_missing"):
		return "st:no_data"
	case flagged("installed_power_missing"):
		return "st:no_power"
	case b.InductiveThreshold != nil || b.CapacitiveThreshold != nil:
		return "st:within"
	}
	return "st:exempt"
}

func (v *invoice) statusChip(key string) *chip {
	c := &chip{text: label(v.loc, key), fg: muted, tint: headerFill}
	switch key {
	case "st:charged", "st:company_charged":
		c.fg, c.tint = danger, dangerTint
	case "st:waived", "st:no_power", "st:no_data":
		c.fg, c.tint = warning, warningTint
	case "st:within", "st:company_none":
		c.fg, c.tint = success, successTint
	}
	return c
}

// details: the reactive card beside the PTF or energy-mix card.
func (v *invoice) details() {
	b := v.b
	side := func(kvarh decimal.Decimal, r, limit *decimal.Decimal) string {
		s := v.num(kvarh, 3) + " kVArh"
		switch {
		case limit != nil:
			s += sep + Percent(v.loc, r) + " / " + Percent(v.loc, limit)
		case r != nil:
			s += sep + Percent(v.loc, r)
		}
		return s
	}
	reactive := [][2]string{
		{label(v.loc, "inductive"), side(b.InductiveKvarh, b.InductiveRatio, b.InductiveThreshold)},
		{label(v.loc, "capacitive"), side(b.CapacitiveKvarh, b.CapacitiveRatio, b.CapacitiveThreshold)},
	}
	if b.ReactivePowerPrice != nil {
		reactive = append(reactive, [2]string{label(v.loc, "reactive_price"), v.num(*b.ReactivePowerPrice, 6) + " " + string(b.Currency) + "/kVArh"})
	}

	var title string
	var other [][2]string
	if b.PtfYekdemUsed {
		title = label(v.loc, "ptf_yekdem")
		opt := func(x *int32) string {
			if x == nil {
				return "—"
			}
			return fmt.Sprint(*x)
		}
		other = [][2]string{
			{label(v.loc, "hours"), opt(b.PtfHoursMatched) + " / " + opt(b.PtfHoursExpected)},
			{label(v.loc, "hours_missing"), opt(b.PtfHoursMissing)},
			{label(v.loc, "ptf_average"), v.optNum(b.PtfAverage, 2) + " TL/MWh"},
			{label(v.loc, "yekdem"), v.optNum(b.YekdemUsed, 2) + " TL/MWh"},
		}
	} else {
		title = label(v.loc, "energy_card")
		if b.EffectiveEnergyPrice != nil {
			other = append(other, [2]string{label(v.loc, "effective_price"), v.num(*b.EffectiveEnergyPrice, 6) + " " + string(b.Currency) + "/kWh"})
		}
		if !b.T1Kwh.IsZero() || !b.T2Kwh.IsZero() || !b.T3Kwh.IsZero() {
			for _, band := range []struct {
				key string
				kwh decimal.Decimal
			}{{"t1", b.T1Kwh}, {"t2", b.T2Kwh}, {"t3", b.T3Kwh}} {
				other = append(other, [2]string{label(v.loc, band.key), v.num(band.kwh, 3) + " kWh"})
			}
		}
		if b.TieredApplied {
			other = append(other, [2]string{label(v.loc, "low_tier"), v.num(b.LowTierKwh, 3) + " kWh"},
				[2]string{label(v.loc, "high_tier"), v.num(b.HighTierKwh, 3) + " kWh"})
		}
	}

	h := cardHeight(max(len(reactive), len(other)))
	v.ensure(h + 4)
	y := v.f.GetY() + 2
	w := (contentW - gap) / 2
	if len(other) == 0 {
		w = contentW
	}
	v.card(margin, y, w, h, label(v.loc, "reactive"), v.statusChip(reactiveStatus(b, v.d.Lines)))
	v.kv(margin+1.5, y+9, w-3, 24, reactive)
	if len(other) > 0 {
		v.card(margin+w+gap, y, w, h, title, nil)
		v.kv(margin+w+gap+1.5, y+9, w-3, 40, other)
	}
	v.f.SetY(y + h + 2)
}

func (v *invoice) readings() {
	rows := IndexRows(v.loc, v.b.IndexStart, v.b.IndexEnd)
	if len(rows) == 0 {
		return
	}
	v.section(label(v.loc, "readings"))
	t := &table{p: &v.painter, widths: []float64{66, 40, 40, 40}, aligns: []string{"L", "R", "R", "R"},
		head: []string{label(v.loc, "register"), label(v.loc, "start"), label(v.loc, "end"), label(v.loc, "diff")}}
	t.header()
	for _, r := range rows {
		t.add(r[:])
	}
	t.end()
	v.f.SetY(v.f.GetY() + 2)
}

func (v *invoice) members() {
	if len(v.d.Members) == 0 || v.b.Scope == model.BillScopeAnalyzer {
		return
	}
	v.section(fmt.Sprintf("%s (%d)", label(v.loc, "members"), len(v.d.Members)))
	t := &table{p: &v.painter, widths: []float64{50, 136}, aligns: []string{"L", "L"},
		head: []string{label(v.loc, "installation"), label(v.loc, "name")}}
	t.header()
	for _, m := range v.d.Members {
		t.add([]string{m.InstallationNumber, m.Name})
	}
	t.end()
}

// indexOrder is the order an invoice lists its registers in.
var indexOrder = []string{
	"active_import", "t1_import", "t2_import", "t3_import", "reactive_inductive_import", "reactive_capacitive_import",
	"active_export", "t1_export", "t2_export", "t3_export", "reactive_inductive_export", "reactive_capacitive_export",
}

// IndexRows is the invoice's index table: label, opening and closing index and
// their difference for every register the meter reported, in a fixed order. A
// register with no index on either side is left out — most meters report four
// of the twelve.
func IndexRows(locale string, start, end json.RawMessage) [][4]string {
	locale = normaliseLocale(locale)
	starts, ends := indexes(start), indexes(end)
	index := func(v *decimal.Decimal) string {
		if v == nil {
			return "—"
		}
		return render.FormatDecimal(*v, 3, locale)
	}
	var out [][4]string
	seen := map[string]bool{}
	add := func(key string) {
		if seen[key] || (starts[key] == nil && ends[key] == nil) {
			return
		}
		seen[key] = true
		diff := "—"
		if starts[key] != nil && ends[key] != nil {
			diff = render.FormatDecimal(ends[key].Sub(*starts[key]), 3, locale)
		}
		out = append(out, [4]string{label(locale, "reg:"+key), index(starts[key]), index(ends[key]), diff})
	}
	for _, key := range indexOrder {
		add(key)
	}
	// A register this list does not know still shows, under its own name.
	var rest []string
	for key := range starts {
		rest = append(rest, key)
	}
	for key := range ends {
		rest = append(rest, key)
	}
	sort.Strings(rest)
	for _, key := range rest {
		add(key)
	}
	return out
}

// Percent renders a stored ratio (0.4474) as a percentage ("%44,74",
// "44.74%" in English), "—" when absent.
func Percent(locale string, ratio *decimal.Decimal) string {
	if ratio == nil {
		return "—"
	}
	return pct(normaliseLocale(locale), ratio.Mul(decimal.NewFromInt(100)), 2)
}

// pct places the sign where each language writes it.
func pct(locale string, x decimal.Decimal, places int32) string {
	n := render.FormatDecimal(x, places, locale)
	if locale == "en" {
		return n + "%"
	}
	return "%" + n
}

func indexes(raw json.RawMessage) map[string]*decimal.Decimal {
	var m map[string]*string
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := make(map[string]*decimal.Decimal, len(m))
	for k, v := range m {
		if v == nil {
			out[k] = nil
			continue
		}
		d, err := decimal.NewFromString(*v)
		if err != nil {
			out[k] = nil
			continue
		}
		out[k] = &d
	}
	return out
}

// DayRanges collapses sorted YYYY-MM-DD days into "02.01.2006" or
// "02.01.2006 – 05.01.2006" runs of consecutive days.
func DayRanges(days []string) []string {
	var out []string
	var start, prev time.Time
	flush := func() {
		if start.IsZero() {
			return
		}
		if start.Equal(prev) {
			out = append(out, start.Format("02.01.2006"))
		} else {
			out = append(out, start.Format("02.01.2006")+" – "+prev.Format("02.01.2006"))
		}
	}
	for _, s := range days {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			continue
		}
		if !start.IsZero() && d.Equal(prev.AddDate(0, 0, 1)) {
			prev = d
			continue
		}
		flush()
		start, prev = d, d
	}
	flush()
	return out
}
