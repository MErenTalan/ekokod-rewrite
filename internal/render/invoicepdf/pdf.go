// Package invoicepdf renders a bill as a deterministic A4 PDF with embedded
// Go fonts (Turkish glyphs, BSD licence).
package invoicepdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

// Member is one analyzer rolled into a building or company bill.
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
	Locale       string // "tr" (default) | "en"
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

// Render is deterministic: identical Documents give identical bytes.
func Render(d Document) ([]byte, error) {
	loc := d.Locale
	if loc != "en" {
		loc = "tr"
	}
	b := d.Bill
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(!d.Uncompressed)
	pdf.SetCatalogSort(true)
	pdf.SetCreationDate(b.ComputedAt)
	pdf.SetModificationDate(b.ComputedAt)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	pdf.SetTitle(label(loc, "title")+" "+b.PeriodKey, true)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	money := func(v decimal.Decimal) string { return render.FormatMoney(v, loc) }
	num := func(v decimal.Decimal, places int32) string { return render.FormatDecimal(v, places, loc) }
	optNum := func(v *decimal.Decimal, places int32) string {
		if v == nil {
			return "—"
		}
		return num(*v, places)
	}
	heading := func(text string) {
		pdf.Ln(3)
		pdf.SetFont("Go", "B", 11)
		pdf.CellFormat(0, 7, text, "B", 1, "L", false, 0, "")
		pdf.SetFont("Go", "", 9)
	}
	kv := func(k, v string) {
		pdf.CellFormat(55, 5, k, "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 5, v, "", 1, "L", false, 0, "")
	}

	pdf.SetFont("Go", "B", 16)
	pdf.CellFormat(0, 10, label(loc, "title"), "", 1, "L", false, 0, "")
	if b.Status == model.BillStatusFlagged {
		pdf.SetFont("Go", "B", 12)
		pdf.SetTextColor(180, 0, 0)
		reason := ""
		if b.FlagReason != nil {
			reason = " (" + *b.FlagReason + ")"
		}
		pdf.CellFormat(0, 8, label(loc, "preview")+reason, "1", 1, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.SetFont("Go", "", 9)
	kv(label(loc, "company"), d.CompanyName)
	if d.BuildingName != "" {
		kv(label(loc, "building"), d.BuildingName)
	}
	kv(label(loc, "scope"), label(loc, scopeKey(b.Scope)))
	kv(label(loc, "period"), b.PeriodKey)
	kv(label(loc, "dates"), b.PeriodStart.In(istanbul).Format("02.01.2006")+" – "+b.PeriodEnd.In(istanbul).Format("02.01.2006"))
	kv(label(loc, "days"), fmt.Sprint(b.DaysInPeriod))
	if b.TariffEffectiveFrom != nil {
		kv(label(loc, "tariff_date"), b.TariffEffectiveFrom.Format("02.01.2006"))
	}
	kv(label(loc, "currency"), string(b.Currency))

	if starts, ends := indexes(b.IndexStart), indexes(b.IndexEnd); len(starts)+len(ends) > 0 {
		heading(label(loc, "readings"))
		keys := map[string]bool{}
		for k := range starts {
			keys[k] = true
		}
		for k := range ends {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		row(pdf, []float64{60, 50, 50}, []string{label(loc, "register"), label(loc, "start"), label(loc, "end")}, true)
		for _, k := range sorted {
			row(pdf, []float64{60, 50, 50}, []string{k, optNum(starts[k], 3), optNum(ends[k], 3)}, false)
		}
	}

	heading(label(loc, "quantities"))
	kv(label(loc, "active_import"), num(b.ActiveImport, 3)+" kWh")
	kv(label(loc, "active_export"), num(b.ActiveExport, 3)+" kWh")
	kv(label(loc, "net"), num(b.NetConsumption, 3)+" kWh")
	kv(label(loc, "inductive"), num(b.InductiveKvarh, 3)+" kVArh")
	kv(label(loc, "capacitive"), num(b.CapacitiveKvarh, 3)+" kVArh")
	kv(label(loc, "max_demand"), optNum(b.MaxDemandKw, 3)+" kW")

	heading(label(loc, "lines"))
	widths := []float64{70, 35, 35, 40}
	row(pdf, widths, []string{label(loc, "item"), label(loc, "quantity"), label(loc, "unit_price"), label(loc, "amount")}, true)
	lines := append([]model.BillLine(nil), d.Lines...)
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].SortOrder < lines[j].SortOrder })
	for _, l := range lines {
		qty := optNum(l.Quantity, 3)
		if l.Unit != nil && l.Quantity != nil {
			qty += " " + *l.Unit
		}
		price := optNum(l.UnitPrice, 6)
		if l.RatePct != nil {
			price = "%" + num(*l.RatePct, 2)
		}
		row(pdf, widths, []string{lineLabel(loc, l.Code), qty, price, money(l.Amount)}, false)
	}
	pdf.SetFont("Go", "B", 10)
	row(pdf, widths, []string{label(loc, "vat_base"), "", "", money(b.VatBase)}, false)
	row(pdf, widths, []string{label(loc, "total"), "", "", money(b.TotalCost) + " " + string(b.Currency)}, false)
	pdf.SetFont("Go", "", 9)

	if b.InductiveThreshold != nil || b.InductiveRatio != nil {
		heading(label(loc, "reactive"))
		kv(label(loc, "inductive")+" "+label(loc, "ratio")+" / "+label(loc, "limit"), optNum(b.InductiveRatio, 4)+" / "+optNum(b.InductiveThreshold, 2))
		kv(label(loc, "capacitive")+" "+label(loc, "ratio")+" / "+label(loc, "limit"), optNum(b.CapacitiveRatio, 4)+" / "+optNum(b.CapacitiveThreshold, 2))
		applied := label(loc, "no")
		if b.ReactivePenaltyApplied {
			applied = label(loc, "yes")
		}
		kv(label(loc, "applied"), applied)
	}
	if b.PtfYekdemUsed {
		heading(label(loc, "ptf"))
		opt := func(v *int32) string {
			if v == nil {
				return "—"
			}
			return fmt.Sprint(*v)
		}
		kv(label(loc, "hours_expected"), opt(b.PtfHoursExpected))
		kv(label(loc, "hours_matched"), opt(b.PtfHoursMatched))
		kv(label(loc, "hours_missing"), opt(b.PtfHoursMissing))
		kv(label(loc, "ptf_average"), optNum(b.PtfAverage, 2)+" TL/MWh")
		kv(label(loc, "yekdem"), optNum(b.YekdemUsed, 2)+" TL/MWh")
	}
	if len(d.Members) > 0 && b.Scope != model.BillScopeAnalyzer {
		heading(label(loc, "members"))
		for _, m := range d.Members {
			pdf.CellFormat(0, 5, strings.TrimSpace(m.Name+" "+m.InstallationNumber), "", 1, "L", false, 0, "")
		}
	}
	pdf.Ln(4)
	pdf.SetFont("Go", "", 7)
	pdf.MultiCell(0, 4, label(loc, "footnote"), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("invoicepdf: %w", err)
	}
	return buf.Bytes(), nil
}

func row(pdf *fpdf.Fpdf, widths []float64, cells []string, header bool) {
	style := ""
	if header {
		style = "B"
	}
	pdf.SetFont("Go", style, 9)
	for i, c := range cells {
		align := "R"
		if i == 0 {
			align = "L"
		}
		pdf.CellFormat(widths[i], 6, c, "B", 0, align, false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Go", "", 9)
}

func scopeKey(s model.BillScope) string {
	switch s {
	case model.BillScopeCompany:
		return "company_scope"
	case model.BillScopeBuilding:
		return "building_scope"
	}
	return "analyzer"
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
