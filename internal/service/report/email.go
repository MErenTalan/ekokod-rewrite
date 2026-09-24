package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render"
)

type emailLabels struct {
	monthly, yearly, consumption, production, bill, noData, footer string
	months                                                         [12]string
}

var emailCatalogue = map[string]emailLabels{
	"tr": {monthly: "Aylık Enerji Raporu", yearly: "Yıllık Enerji Raporu", consumption: "Toplam tüketim",
		production: "Toplam üretim", bill: "Toplam fatura", noData: "veri yok",
		footer: "Bu rapor otomatik olarak oluşturulmuştur. PDF ve Excel dosyaları ektedir.",
		months: [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}},
	"en": {monthly: "Monthly Energy Report", yearly: "Yearly Energy Report", consumption: "Total consumption",
		production: "Total production", bill: "Total bill", noData: "no data",
		footer: "This report was generated automatically. The PDF and Excel files are attached.",
		months: [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}},
}

// html/template escapes every value: a building is named by its tenant.
var emailTemplate = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html><body style="font-family:Arial,sans-serif;color:#1f2a27">
<h1 style="font-size:20px">{{.Buildings}}</h1>
<h2 style="font-size:16px;font-weight:normal">{{.Title}} — {{.Period}}</h2>
<table style="border-collapse:collapse">
{{range .Rows}}<tr><td style="padding:4px 16px 4px 0">{{.Label}}</td><td style="padding:4px 0;text-align:right"><strong>{{.Value}}</strong></td></tr>
{{end}}</table>
<p style="font-size:12px;color:#5a6b65">{{.Footer}}</p>
</body></html>`))

type emailRow struct{ Label, Value string }

type emailView struct {
	Buildings, Title, Period, Footer string
	Rows                             []emailRow
}

func labelsFor(locale string) (emailLabels, string) {
	if locale != "en" {
		locale = "tr"
	}
	return emailCatalogue[locale], locale
}

// view is the figures the subject, the HTML and the text share (R272).
func view(p domain.Payload, buildings []string, locale string) emailView {
	l, loc := labelsFor(locale)
	kwh := func(v *decimal.Decimal) string {
		if v == nil {
			return l.noData
		}
		return render.FormatDecimal(*v, 2, loc) + " kWh"
	}
	money := func(list []domain.MoneyFigure) []emailRow {
		if len(list) == 0 {
			return []emailRow{{l.bill, l.noData}}
		}
		var rows []emailRow
		for _, m := range list {
			rows = append(rows, emailRow{l.bill, render.FormatMoney(m.Value, loc) + " " + m.Currency})
		}
		return rows
	}
	// A line break in a building name would end the Subject header early.
	names := strings.Join(buildings, ", ")
	names = strings.NewReplacer("\r", " ", "\n", " ").Replace(names)
	v := emailView{Buildings: names, Footer: l.footer}
	switch {
	case p.Monthly != nil:
		m := p.Monthly
		v.Title, v.Period = l.monthly, fmt.Sprintf("%s %d", l.months[m.Month-1], m.Year)
		v.Rows = append([]emailRow{{l.consumption, kwh(m.Consumption.Value)}, {l.production, kwh(m.Production.Value)}}, money(m.Bill)...)
	case p.Yearly != nil:
		y := p.Yearly
		v.Title, v.Period = l.yearly, fmt.Sprint(y.Year)
		v.Rows = append([]emailRow{{l.consumption, kwh(y.Consumption.Value)}, {l.production, kwh(y.Production.Value)}}, money(y.Bill)...)
	}
	return v
}

// EmailContent is the stored subject and HTML body of a report (R272).
func EmailContent(p domain.Payload, buildings []string, locale string) (subject, html string, err error) {
	v := view(p, buildings, locale)
	var b bytes.Buffer
	if err := emailTemplate.Execute(&b, v); err != nil {
		return "", "", err
	}
	return v.Buildings + " - " + v.Title + " - " + v.Period, b.String(), nil
}

// EmailText is the plain-text alternative of the same figures.
func EmailText(p domain.Payload, buildings []string, locale string) string {
	v := view(p, buildings, locale)
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s — %s\n\n", v.Buildings, v.Title, v.Period)
	for _, r := range v.Rows {
		fmt.Fprintf(&b, "%s: %s\n", r.Label, r.Value)
	}
	b.WriteString("\n" + v.Footer + "\n")
	return b.String()
}
