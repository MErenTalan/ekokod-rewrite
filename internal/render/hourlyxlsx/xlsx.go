// Package hourlyxlsx renders a PTF bill's priced hours as a one-sheet XLSX.
package hourlyxlsx

import (
	"bytes"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

var headers = map[string][]string{
	"tr": {"Saat (İstanbul)", "Tüketim (kWh)", "PTF (TL/MWh)", "YEKDEM (TL/MWh)", "KBK", "Birim fiyat (TL/kWh)", "Tutar (TL)"},
	"en": {"Hour (Istanbul)", "Consumption (kWh)", "PTF (TL/MWh)", "YEKDEM (TL/MWh)", "KBK", "Unit price (TL/kWh)", "Cost (TL)"},
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Render writes one row per hour plus a totals row; numbers are written as
// decimal strings so no value passes through float.
func Render(b model.Bill, rows []model.BillHourlyDetail, locale string) ([]byte, error) {
	if locale != "en" {
		locale = "tr"
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := b.PeriodKey
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("hourlyxlsx: %w", err)
	}
	set := func(col, row int, v any) error {
		cell, err := excelize.CoordinatesToCellName(col, row)
		if err != nil {
			return err
		}
		return f.SetCellValue(sheet, cell, v)
	}
	for i, h := range headers[locale] {
		if err := set(i+1, 1, h); err != nil {
			return nil, fmt.Errorf("hourlyxlsx: %w", err)
		}
	}
	kwh, cost := decimal.Zero, decimal.Zero
	for i, r := range rows {
		vals := []any{r.Ts.In(istanbul).Format("2006-01-02 15:04"), r.Consumption.String(), r.PTF.String(), r.Yekdem.String(),
			r.Kbk.String(), r.UnitPrice.String(), r.Cost.StringFixed(4)}
		for c, v := range vals {
			if err := set(c+1, i+2, v); err != nil {
				return nil, fmt.Errorf("hourlyxlsx: %w", err)
			}
		}
		kwh, cost = kwh.Add(r.Consumption), cost.Add(r.Cost)
	}
	total := map[string]string{"tr": "Toplam", "en": "Total"}[locale]
	last := len(rows) + 2
	for c, v := range map[int]any{1: total, 2: kwh.String(), 7: cost.StringFixed(4)} {
		if err := set(c, last, v); err != nil {
			return nil, fmt.Errorf("hourlyxlsx: %w", err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("hourlyxlsx: %w", err)
	}
	return buf.Bytes(), nil
}
