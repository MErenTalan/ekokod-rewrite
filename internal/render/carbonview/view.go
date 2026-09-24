// Package carbonview lays out a carbon report payload (R312) as a
// reportview.Document, so the carbon PDF uses the report renderer's page,
// fonts and determinism.
package carbonview

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

// Build lays out p in the given locale ("tr" unless "en").
func Build(p domain.ReportPayload, locale string) reportview.Document {
	if locale != "en" {
		locale = "tr"
	}
	l := catalogue[locale]
	num := func(v decimal.Decimal, places int32) string { return render.FormatDecimal(v, places, locale) }
	doc := reportview.Document{Locale: locale, Title: l.isoTitle}
	if p.ReportType == "ghg" {
		doc.Title = l.ghgTitle
	}
	doc.Lines = []string{l.company + ": " + p.Company, l.building + ": " + p.Building}
	if p.Address != nil && *p.Address != "" {
		doc.Lines = append(doc.Lines, l.address+": "+*p.Address)
	}
	doc.Lines = append(doc.Lines, l.period+": "+date(p.From, locale)+" – "+date(p.To, locale),
		l.generated+": "+p.GeneratedAt.Format(stamp(locale)))
	for _, g := range p.Groups {
		s := reportview.Section{Key: g.Key, Title: l.groups[g.Key], Header: []string{l.sub, "kg CO₂e"}}
		for _, line := range g.Lines {
			s.Rows = append(s.Rows, []string{l.subs[line.Sub], num(line.KgCO2e, 2)})
		}
		if len(g.Lines) == 0 {
			s.Notes = []string{l.noRecords}
		}
		s.Rows = append(s.Rows, []string{l.total, num(g.TotalKgCO2e, 2)})
		doc.Sections = append(doc.Sections, s)
	}
	doc.Notes = []string{fmt.Sprintf(l.totalLine, num(p.TotalKgCO2e, 2))}
	if p.Pending > 0 {
		doc.Notes = append(doc.Notes, fmt.Sprintf(l.pending, p.Pending))
	}
	grid := reportview.Section{Key: "grid", Title: l.gridTitle, Header: []string{l.item, l.value}}
	if f := p.Figures94; f != nil {
		src := ""
		if f.GridFactorSource != nil {
			src = *f.GridFactorSource
		}
		if f.GridFactorYear != nil {
			src = strings.TrimPrefix(src+", "+fmt.Sprint(*f.GridFactorYear), ", ")
		}
		grid.Rows = [][]string{
			{l.consumption, num(f.ConsumptionKwh, 2) + " kWh"},
			{l.generation, num(f.GenerationKwh, 2) + " kWh"},
			{l.factor, num(f.GridFactor, 3) + " " + f.GridFactorUnit},
			{l.source, src},
			{l.consumptionT, num(f.ConsumptionT, 3) + " t"},
			{l.reductionT, num(f.ProductionReductT, 3) + " t"},
			{l.netT, num(f.NetT, 3) + " t"},
		}
	} else {
		grid.Notes = []string{l.noMeter}
	}
	doc.Sections = append(doc.Sections, grid)
	return doc
}

func date(iso, locale string) string {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return t.Format(map[string]string{"tr": "02.01.2006", "en": "02/01/2006"}[locale])
}

func stamp(locale string) string {
	if locale == "en" {
		return "02/01/2006 15:04"
	}
	return "02.01.2006 15:04"
}

// Text flattens a document for assertions and plain-text previews.
func Text(doc reportview.Document) string {
	var b strings.Builder
	b.WriteString(doc.Title + "\n")
	for _, l := range doc.Lines {
		b.WriteString(l + "\n")
	}
	for _, s := range doc.Sections {
		b.WriteString(s.Title + "\n" + strings.Join(s.Header, " | ") + "\n")
		for _, r := range s.Rows {
			b.WriteString(strings.Join(r, " | ") + "\n")
		}
		for _, n := range s.Notes {
			b.WriteString(n + "\n")
		}
	}
	for _, n := range doc.Notes {
		b.WriteString(n + "\n")
	}
	return b.String()
}
