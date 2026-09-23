// Package reportpdf renders a stored report payload as an A4 PDF, section
// by section from internal/render/reportview, in either locale. It is
// deterministic: identical Documents give identical bytes.
package reportpdf

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

// Document is everything a report PDF prints.
type Document struct {
	Payload       report.Payload
	CompanyName   string
	BuildingNames []string
	Locale        string // "tr" (default) | "en"
	// GeneratedAt stamps the file; an input so two renders are byte-identical.
	GeneratedAt time.Time
	// Uncompressed disables stream compression.
	Uncompressed bool
}

// drawer is what the layout asks of a page; tests record it.
type drawer interface {
	title(title string, lines []string)
	// heading keeps with what follows: a chart needs its whole height.
	heading(text string, keep float64)
	table(header []string, rows [][]string)
	chart(c reportview.Chart)
	note(text string)
}

// draw lays out a document; it is the whole content decision.
func draw(doc reportview.Document, w drawer) {
	w.title(doc.Title, doc.Lines)
	for _, n := range doc.Notes {
		w.note(n)
	}
	for _, s := range doc.Sections {
		keep := 20.0
		if s.Chart != nil {
			keep = chartHeight + 30
		}
		w.heading(s.Title, keep)
		if s.Chart != nil {
			w.chart(*s.Chart)
		}
		w.table(s.Header, s.Rows)
		for _, n := range s.Notes {
			w.note(n)
		}
	}
}

// Render builds the PDF.
func Render(d Document) ([]byte, error) {
	view := reportview.Build(d.Payload, d.CompanyName, d.BuildingNames, d.Locale)
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(!d.Uncompressed)
	pdf.SetCatalogSort(true)
	pdf.SetCreationDate(d.GeneratedAt)
	pdf.SetModificationDate(d.GeneratedAt)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	pdf.SetTitle(view.Title+" "+d.Payload.Period, true)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AddPage()
	draw(view, &page{pdf: pdf, locale: view.Locale})
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("reportpdf: %w", err)
	}
	return buf.Bytes(), nil
}

const (
	contentWidth = 180.0
	chartHeight  = 55.0
)

type page struct {
	pdf    *fpdf.Fpdf
	locale string
}

func (p *page) title(title string, lines []string) {
	p.pdf.SetFont("Go", "B", 15)
	p.pdf.MultiCell(contentWidth, 7, title, "", "L", false)
	p.pdf.SetFont("Go", "", 10)
	for _, l := range lines {
		p.pdf.MultiCell(contentWidth, 5.5, l, "", "L", false)
	}
	p.pdf.Ln(2)
}

func (p *page) heading(text string, keep float64) {
	p.needs(keep)
	p.pdf.Ln(2)
	p.pdf.SetFont("Go", "B", 11)
	p.pdf.MultiCell(contentWidth, 7, text, "B", "L", false)
	p.pdf.Ln(1)
}

func (p *page) note(text string) {
	p.pdf.SetFont("Go", "", 8)
	p.pdf.MultiCell(contentWidth, 4.5, text, "", "L", false)
}

// table prints a two-column label/value table with wrapping values, or a
// wider one in equal columns.
func (p *page) table(header []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := len(rows[0])
	if cols <= 3 && len(header) == 0 {
		widths := []float64{95, contentWidth - 95}
		if cols == 3 {
			widths = []float64{80, 60, contentWidth - 140}
		}
		p.pdf.SetFont("Go", "", 9)
		for _, r := range rows {
			p.needs(6)
			for i, c := range r {
				if i == len(r)-1 {
					p.pdf.MultiCell(widths[i], 5.5, c, "", "L", false)
					continue
				}
				p.pdf.CellFormat(widths[i], 5.5, c, "", 0, "L", false, 0, "")
			}
		}
		return
	}
	w := contentWidth / float64(max(cols, len(header)))
	p.pdf.SetFont("Go", "B", 8)
	for i, h := range header {
		// Headers align with their column's values, which are right-aligned.
		p.pdf.CellFormat(w, 5.5, h, "B", 0, align(i), false, 0, "")
	}
	p.pdf.Ln(-1)
	p.pdf.SetFont("Go", "", 8)
	for _, r := range rows {
		p.needs(5)
		for i, c := range r {
			p.pdf.CellFormat(w, 5, c, "", 0, align(i), false, 0, "")
		}
		p.pdf.Ln(-1)
	}
}

func align(col int) string {
	if col == 0 {
		return "L"
	}
	return "R"
}

// Brand green for the current series, muted grey for the prior (07 §5).
var seriesColours = [][3]int{{5, 150, 105}, {156, 163, 175}, {4, 120, 87}}

// chart draws grouped bars. Heights are decimal ratios converted to a float
// only at the drawing call: no money or energy arithmetic in float (Global
// Constraints).
func (p *page) chart(c reportview.Chart) {
	if len(c.Categories) == 0 {
		return
	}
	p.needs(chartHeight + 16)
	top := p.pdf.GetY() + 4
	left := p.pdf.GetX() + 12
	width := contentWidth - 12
	peak := decimal.Zero
	for _, s := range c.Series {
		for _, v := range s.Values {
			if v != nil && v.GreaterThan(peak) {
				peak = *v
			}
		}
	}
	p.pdf.SetFont("Go", "", 7)
	p.pdf.Text(left-12, top-1, c.Unit)
	// The scale: the tallest bar's value at the top of the axis.
	p.pdf.Text(left-12, top+3, render.FormatDecimal(peak, 0, p.locale))
	p.pdf.Line(left, top, left, top+chartHeight)
	base := top + chartHeight
	p.pdf.SetDrawColor(120, 120, 120)
	p.pdf.Line(left, base, left+width, base)
	group := width / float64(len(c.Categories))
	bar := group * 0.8 / float64(len(c.Series))
	for i, cat := range c.Categories {
		x := left + group*float64(i) + group*0.1
		for si, s := range c.Series {
			v := s.Values[i]
			if v == nil || peak.IsZero() || !v.IsPositive() {
				continue // a gap, never a zero-height bar pretending to be data
			}
			h := v.Div(peak).Mul(decimal.NewFromFloat(chartHeight)).InexactFloat64()
			col := seriesColours[si%len(seriesColours)]
			p.pdf.SetFillColor(col[0], col[1], col[2])
			p.pdf.Rect(x+bar*float64(si), base-h, bar, h, "F")
		}
		label := cat
		if len(c.Categories) > 6 && len([]rune(label)) > 3 {
			label = string([]rune(label)[:3]) // every month the same way
		}
		p.pdf.Text(x, base+4, label)
	}
	legendY := base + 9
	lx := left
	for si, s := range c.Series {
		col := seriesColours[si%len(seriesColours)]
		p.pdf.SetFillColor(col[0], col[1], col[2])
		p.pdf.Rect(lx, legendY-2.5, 3, 3, "F")
		p.pdf.Text(lx+4, legendY, s.Name)
		lx += 40
	}
	p.pdf.SetY(legendY + 3)
}

// needs starts a new page when fewer than h mm remain.
func (p *page) needs(h float64) {
	_, pageH := p.pdf.GetPageSize()
	_, _, _, bottom := p.pdf.GetMargins()
	if p.pdf.GetY()+h > pageH-bottom-14 {
		p.pdf.AddPage()
	}
}
