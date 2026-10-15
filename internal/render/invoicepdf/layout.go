package invoicepdf

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
	"github.com/shopspring/decimal"
)

// Page geometry, mm.
const (
	pageW     = 210.0
	pageH     = 297.0
	margin    = 12.0
	contentW  = pageW - 2*margin
	footerTop = 283.0
	breakAt   = 18.0 // auto page break margin: keeps rows off the footer
	gap       = 4.0
)

type rgb [3]int

// The web design system's emerald palette (web/src/styles/tokens.css, light).
var (
	brand       = rgb{4, 120, 87}
	brandTint   = rgb{209, 250, 229}
	ink         = rgb{11, 42, 34}
	muted       = rgb{75, 93, 87}
	subtle      = rgb{122, 142, 135}
	rule        = rgb{220, 231, 226}
	wash        = rgb{246, 250, 248}
	headerFill  = rgb{236, 243, 239}
	white       = rgb{255, 255, 255}
	danger      = rgb{185, 28, 28}
	dangerTint  = rgb{254, 226, 226}
	warning     = rgb{180, 83, 9}
	warningTint = rgb{254, 243, 199}
	success     = rgb{21, 128, 61}
	successTint = rgb{220, 252, 231}
)

// painter wraps fpdf with the invoice's drawing vocabulary.
type painter struct {
	f   *fpdf.Fpdf
	loc string
}

func (p *painter) fill(c rgb)   { p.f.SetFillColor(c[0], c[1], c[2]) }
func (p *painter) color(c rgb)  { p.f.SetTextColor(c[0], c[1], c[2]) }
func (p *painter) stroke(c rgb) { p.f.SetDrawColor(c[0], c[1], c[2]) }

// font: style "" | "B" | "M" (Go Medium, its own family).
func (p *painter) font(style string, size float64) {
	if style == "M" {
		p.f.SetFont("GoMedium", "", size)
		return
	}
	p.f.SetFont("Go", style, size)
}

func (p *painter) upper(s string) string {
	if p.loc == "tr" {
		return strings.ToUpperSpecial(unicode.TurkishCase, s)
	}
	return strings.ToUpper(s)
}

// fit shortens s with an ellipsis until it fits w (cell margins included).
func (p *painter) fit(s string, w float64) string {
	room := w - 2*p.f.GetCellMargin()
	if p.f.GetStringWidth(s) <= room {
		return s
	}
	for s != "" {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
		if p.f.GetStringWidth(s+"…") <= room {
			return s + "…"
		}
	}
	return ""
}

func (p *painter) text(x, y, w, h float64, s, align string) {
	p.f.SetXY(x, y)
	p.f.CellFormat(w, h, p.fit(s, w), "", 0, align, false, 0, "")
}

// ensure starts a new page when h does not fit above the footer.
func (p *painter) ensure(h float64) bool {
	if p.f.GetY()+h > pageH-breakAt {
		p.f.AddPage()
		return true
	}
	return false
}

// section prints a small upper-case heading and moves below it; it keeps
// the heading with a table header and its first row.
func (p *painter) section(title string) {
	p.ensure(9 + 6.5 + rowH)
	y := p.f.GetY() + 3
	p.font("B", 8)
	p.color(brand)
	p.text(margin-p.f.GetCellMargin(), y, contentW, 5, p.upper(title), "L")
	p.f.SetY(y + 6)
}

type chip struct {
	text     string
	fg, tint rgb
}

// card draws a bordered box with an upper-case title and an optional chip.
func (p *painter) card(x, y, w, h float64, title string, c *chip) {
	p.stroke(rule)
	p.f.SetLineWidth(0.3)
	p.fill(white)
	p.f.RoundedRect(x, y, w, h, 2, "1234", "FD")
	p.font("B", 7.5)
	p.color(subtle)
	p.text(x+2.5, y+2.5, w/2, 5, p.upper(title), "L")
	if c == nil {
		return
	}
	p.font("B", 7)
	cw := p.f.GetStringWidth(c.text) + 6
	p.fill(c.tint)
	p.f.RoundedRect(x+w-3.5-cw, y+2.6, cw, 4.8, 2.4, "1234", "F")
	p.color(c.fg)
	p.text(x+w-3.5-cw, y+2.6, cw, 4.8, c.text, "C")
}

const kvRowH = 5.0

// kv prints key/value rows inside a card starting at y.
func (p *painter) kv(x, y, w, keyW float64, rows [][2]string) {
	for i, r := range rows {
		ry := y + float64(i)*kvRowH
		p.font("", 8)
		p.color(muted)
		p.text(x, ry, keyW, kvRowH, r[0], "L")
		p.font("M", 8.5)
		p.color(ink)
		p.text(x+keyW, ry, w-keyW, kvRowH, r[1], "L")
	}
}

func cardHeight(rows int) float64 { return 10 + float64(rows)*kvRowH + 2.5 }

// bar is a ratio gauge with a marker at the limit; over the limit it turns red.
func (p *painter) bar(x, y, w float64, ratio, limit *decimal.Decimal) {
	const h = 2.2
	p.fill(rule)
	p.f.RoundedRect(x, y, w, h, h/2, "1234", "F")
	if ratio == nil {
		return
	}
	r, _ := ratio.Float64()
	scale := r * 1.15
	lim := -1.0
	if limit != nil {
		lim, _ = limit.Float64()
		scale = max(scale, lim*2)
	}
	scale = max(scale, 0.05)
	fw := min(r/scale, 1) * w
	switch {
	case lim < 0: // no limit to judge against
		p.fill(subtle)
	case r > lim:
		p.fill(danger)
	default:
		p.fill(brand)
	}
	if fw >= h {
		p.f.RoundedRect(x, y, fw, h, h/2, "1234", "F")
	} else if fw > 0 {
		p.f.Rect(x, y, fw, h, "F")
	}
	if lim >= 0 {
		mx := x + lim/scale*w
		p.stroke(ink)
		p.f.SetLineWidth(0.45)
		p.f.Line(mx, y-1, mx, y+h+1)
	}
}

// table draws rows with a repeated header after a page break.
type table struct {
	p      *painter
	widths []float64
	aligns []string
	head   []string
	row    int
}

const rowH = 6.4

func (t *table) header() {
	p := t.p
	y := p.f.GetY()
	p.fill(headerFill)
	p.f.Rect(margin, y, contentW, 6.5, "F")
	p.font("B", 7)
	p.color(muted)
	x := margin
	for i, h := range t.head {
		p.text(x, y+0.4, t.widths[i], 6, p.upper(h), t.aligns[i])
		x += t.widths[i]
	}
	p.f.SetY(y + 6.5)
	t.row = 0
}

// add prints one row; colours, when given, set each cell's text colour.
func (t *table) add(cells []string, colours ...rgb) {
	p := t.p
	if p.ensure(rowH) {
		t.header()
	}
	y := p.f.GetY()
	if t.row%2 == 1 {
		p.fill(wash)
		p.f.Rect(margin, y, contentW, rowH, "F")
	}
	x := margin
	for i, c := range cells {
		p.font("", 8.5)
		p.color(ink)
		if i < len(colours) {
			p.color(colours[i])
		}
		p.text(x, y, t.widths[i], rowH, c, t.aligns[i])
		x += t.widths[i]
	}
	p.f.SetY(y + rowH)
	t.row++
}

func (t *table) end() {
	p := t.p
	y := p.f.GetY()
	p.stroke(rule)
	p.f.SetLineWidth(0.3)
	p.f.Line(margin, y, margin+contentW, y)
}
