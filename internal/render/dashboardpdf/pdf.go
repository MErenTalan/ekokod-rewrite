// Package dashboardpdf renders the invoice dashboard of 01 §7.10 as one
// landscape A4 document, from the same aggregate the screen renders (R239).
package dashboardpdf

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

// Document is everything the export prints.
type Document struct {
	Result      billing.DashboardResult
	Period      string
	CompanyName string
	Locale      string // "tr" (default) | "en"
	// GeneratedAt stamps the document; it is an input so two renders of the
	// same month are byte-identical.
	GeneratedAt time.Time
	// Uncompressed disables stream compression (tests inspect the streams).
	Uncompressed bool
}

var widths = []float64{22, 45, 38, 26, 34, 28, 26, 28, 30}

// Render is deterministic: identical Documents give identical bytes.
func Render(d Document) ([]byte, error) {
	loc := d.Locale
	if loc != "en" {
		loc = "tr"
	}
	l := catalogue[loc]
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetCompression(!d.Uncompressed)
	pdf.SetCatalogSort(true)
	pdf.SetCreationDate(d.GeneratedAt)
	pdf.SetModificationDate(d.GeneratedAt)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	pdf.SetTitle(l.title+" "+d.Period, true)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	money := func(v decimal.Decimal) string { return render.FormatMoney(v, loc) }
	num := func(v decimal.Decimal) string { return render.FormatDecimal(v, 2, loc) }

	pdf.SetFont("Go", "B", 14)
	pdf.CellFormat(0, 8, l.title, "", 1, "L", false, 0, "")
	pdf.SetFont("Go", "", 10)
	pdf.CellFormat(0, 6, d.CompanyName+" — "+l.period+": "+d.Period, "", 1, "L", false, 0, "")
	pdf.Ln(2)

	for _, b := range d.Result.Buildings {
		pdf.SetFont("Go", "B", 11)
		pdf.CellFormat(0, 7, b.BuildingName, "", 1, "L", false, 0, "")
		header(pdf, l)
		pdf.SetFont("Go", "", 9)
		for _, r := range b.Rows {
			price := "—"
			if r.ConsumptionPrice != nil {
				price = render.FormatDecimal(*r.ConsumptionPrice, 6, loc)
			}
			row(pdf, []string{r.PeriodKey, r.BuildingName, r.AnalyzerName, r.InstallationNumber, r.EtsoCode,
				num(r.Consumption), num(r.Production), price, money(r.Invoice)})
		}
		pdf.SetFont("Go", "B", 9)
		row(pdf, []string{l.subtotal, b.BuildingName, "", "", "", num(b.TotalConsumption), num(b.TotalProduction), "", money(b.TotalInvoice)})
		if b.BuildingBill != nil {
			// 02 §6.11: the building invoice prices the aggregate once, so it
			// may differ from the rows above. Both are printed.
			row(pdf, []string{l.buildingBill, b.BuildingName, "", "", "", num(b.BuildingBill.Consumption),
				num(b.BuildingBill.Production), "", money(b.BuildingBill.Invoice)})
			if b.DivergesFromRows {
				pdf.SetFont("Go", "", 8)
				pdf.CellFormat(0, 5, l.divergence, "", 1, "L", false, 0, "")
			}
		}
		pdf.Ln(3)
	}

	pdf.SetFont("Go", "B", 11)
	pdf.CellFormat(0, 7, l.netting, "", 1, "L", false, 0, "")
	pdf.SetFont("Go", "", 9)
	for _, n := range d.Result.Netting {
		status := l.netConsumption
		if n.NetStatus == billing.NetStatusProduction {
			status = l.netProduction
		}
		efficiency := l.unavailable
		if n.EfficiencyPct != nil {
			efficiency = render.FormatDecimal(*n.EfficiencyPct, 2, loc) + " %"
		}
		pdf.CellFormat(0, 6, fmt.Sprintf("%s — %s %s · %s %s · %s %s · %s %s · %s",
			n.Currency, l.consumption, num(n.TotalConsumption), l.production, num(n.TotalProduction),
			l.net, num(n.Net), l.invoice, money(n.TotalInvoice), status), "", 1, "L", false, 0, "")
		pdf.CellFormat(0, 6, l.efficiency+": "+efficiency, "", 1, "L", false, 0, "")
	}

	// R235: the plant section of §7.10 has no data source before F9. Saying so
	// is the house style for anything the data cannot answer (R165).
	pdf.Ln(2)
	pdf.SetFont("Go", "B", 10)
	pdf.CellFormat(0, 6, l.plants, "", 1, "L", false, 0, "")
	pdf.SetFont("Go", "", 9)
	pdf.CellFormat(0, 6, l.plantsNote, "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("dashboardpdf: %w", err)
	}
	return buf.Bytes(), nil
}

func header(pdf *fpdf.Fpdf, l labels) {
	pdf.SetFont("Go", "B", 9)
	for i, c := range l.columns {
		pdf.CellFormat(widths[i], 6, c, "B", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)
}

func row(pdf *fpdf.Fpdf, cells []string) {
	for i, c := range cells {
		align := "L"
		if i >= 5 {
			align = "R"
		}
		pdf.CellFormat(widths[i], 5.5, c, "", 0, align, false, 0, "")
	}
	pdf.Ln(-1)
}
