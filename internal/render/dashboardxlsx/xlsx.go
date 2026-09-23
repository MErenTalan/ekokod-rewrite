// Package dashboardxlsx renders the invoice dashboard of 01 §7.10 as a
// workbook: the invoice rows with their per-building subtotals, and a summary
// sheet with the netting figures. It is given the same aggregate the screen
// renders (R239), so the file and the page cannot disagree.
package dashboardxlsx

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
)

type labels struct {
	billsSheet, summarySheet string
	columns                  []string
	subtotal, total          string
	period, currency         string
	consumption, production  string
	net, status, invoice     string
	efficiency, unavailable  string
	netConsumption           string
	netProduction            string
	plants, plantsNote       string
}

var catalogue = map[string]labels{
	"tr": {
		billsSheet: "Faturalar", summarySheet: "Özet",
		columns:  []string{"Dönem", "Bina", "Sayaç", "Tesisat no", "ETSO", "Tüketim (kWh)", "Üretim (kWh)", "Birim fiyat (TL/kWh)", "Fatura"},
		subtotal: "Ara toplam", total: "Toplam", period: "Dönem", currency: "Para birimi",
		consumption: "Toplam tüketim (kWh)", production: "Toplam üretim (kWh)", net: "Net (kWh)", status: "Durum",
		invoice: "Toplam fatura", efficiency: "Verimlilik (%)", unavailable: "Hesaplanamadı",
		netConsumption: "Net tüketim", netProduction: "Net üretim",
		plants:     "GES santralleri",
		plantsNote: "Santral üretimi için veri kaynağı henüz yok; santral satırları F9 ile gelecek.",
	},
	"en": {
		billsSheet: "Invoices", summarySheet: "Summary",
		columns:  []string{"Period", "Building", "Meter", "Installation no", "ETSO", "Consumption (kWh)", "Production (kWh)", "Unit price (TL/kWh)", "Invoice"},
		subtotal: "Subtotal", total: "Total", period: "Period", currency: "Currency",
		consumption: "Total consumption (kWh)", production: "Total production (kWh)", net: "Net (kWh)", status: "Status",
		invoice: "Total invoice", efficiency: "Efficiency (%)", unavailable: "Not available",
		netConsumption: "Net consumption", netProduction: "Net production",
		plants:     "Solar plants (GES)",
		plantsNote: "No data source for plant production yet; plant rows arrive with F9.",
	},
}

// Render writes the workbook. Numbers are written as decimal strings so no
// value passes through float, exactly as the hourly export does.
func Render(res billing.DashboardResult, period, locale string) ([]byte, error) {
	l, ok := catalogue[locale]
	if !ok {
		l = catalogue["tr"]
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", l.billsSheet); err != nil {
		return nil, fmt.Errorf("dashboardxlsx: %w", err)
	}
	if err := writeBills(f, l, res, period); err != nil {
		return nil, err
	}
	if err := writeSummary(f, l, res, period); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("dashboardxlsx: %w", err)
	}
	return buf.Bytes(), nil
}

func writeBills(f *excelize.File, l labels, res billing.DashboardResult, period string) error {
	rows := [][]any{toAny(l.columns)}
	for _, b := range res.Buildings {
		for _, r := range b.Rows {
			price := ""
			if r.ConsumptionPrice != nil {
				price = r.ConsumptionPrice.String()
			}
			rows = append(rows, []any{r.PeriodKey, r.BuildingName, r.AnalyzerName, r.InstallationNumber, r.EtsoCode,
				r.Consumption.String(), r.Production.String(), price, r.Invoice.String()})
		}
		rows = append(rows, []any{l.subtotal + " — " + b.BuildingName, "", "", "", "",
			b.TotalConsumption.String(), b.TotalProduction.String(), "", b.TotalInvoice.String()})
		// 02 §6.11: the building invoice prices the aggregate once, so it can
		// differ from the rows above it. Show it; never reconcile it away.
		if b.BuildingBill != nil {
			rows = append(rows, []any{l.columns[1] + " — " + b.BuildingName, "", "", "", "",
				b.BuildingBill.Consumption.String(), b.BuildingBill.Production.String(), "", b.BuildingBill.Invoice.String()})
		}
	}
	for _, n := range res.Netting {
		rows = append(rows, []any{fmt.Sprintf("%s (%s)", l.total, n.Currency), "", "", "", "",
			n.TotalConsumption.String(), n.TotalProduction.String(), "", n.TotalInvoice.String()})
	}
	return write(f, l.billsSheet, rows)
}

func writeSummary(f *excelize.File, l labels, res billing.DashboardResult, period string) error {
	if _, err := f.NewSheet(l.summarySheet); err != nil {
		return fmt.Errorf("dashboardxlsx: %w", err)
	}
	rows := [][]any{{l.period, period}, {}}
	for _, n := range res.Netting {
		status := l.netConsumption
		if n.NetStatus == billing.NetStatusProduction {
			status = l.netProduction
		}
		efficiency := l.unavailable
		if n.EfficiencyPct != nil {
			efficiency = n.EfficiencyPct.String()
		}
		rows = append(rows,
			[]any{l.currency, string(n.Currency)},
			[]any{l.consumption, n.TotalConsumption.String()},
			[]any{l.production, n.TotalProduction.String()},
			[]any{l.net, n.Net.String()},
			[]any{l.status, status},
			[]any{l.invoice, n.TotalInvoice.String()},
			[]any{l.efficiency, efficiency},
			[]any{},
		)
	}
	// R235: the section 01 §7.10 describes has no data source before F9. The
	// export names it and says so rather than leaving a silent hole.
	rows = append(rows, []any{l.plants, l.plantsNote})
	return write(f, l.summarySheet, rows)
}

func toAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func write(f *excelize.File, sheet string, rows [][]any) error {
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return fmt.Errorf("dashboardxlsx: %w", err)
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return fmt.Errorf("dashboardxlsx: %w", err)
		}
	}
	return nil
}
