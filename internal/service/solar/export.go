package solar

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var exportLabels = map[string][4]string{
	"tr": {"Üretim", "Tarih", "Üretim (kWh)", "Kaynak"},
	"en": {"Production", "Date", "Production (kWh)", "Basis"},
}

var exportLayout = map[string]string{"hour": "2006-01-02 15:00", "day": "2006-01-02", "month": "2006-01"}

// ExportProduction is R284's series as a workbook; the file name carries the range.
func (s *Service) ExportProduction(ctx context.Context, sc store.Scope, plantID uuid.UUID, gran string, from, to time.Time, locale string) ([]byte, string, error) {
	series, err := s.ProductionSeries(ctx, sc, plantID, gran, from, to)
	if err != nil {
		return nil, "", err
	}
	l, ok := exportLabels[locale]
	if !ok {
		l = exportLabels["tr"]
	}
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", l[0]); err != nil {
		return nil, "", err
	}
	if err := f.SetSheetRow(l[0], "A1", &[]any{l[1], l[2], l[3]}); err != nil {
		return nil, "", err
	}
	for i, p := range series.Points {
		row := []any{p.Ts.In(istanbul).Format(exportLayout[gran]), nil, ""}
		if p.ProductionKwh != nil {
			row[1] = p.ProductionKwh.InexactFloat64() // drawing only: the value is already computed
		}
		if p.Basis != nil {
			row[2] = *p.Basis
		}
		if err := f.SetSheetRow(l[0], fmt.Sprintf("A%d", i+2), &row); err != nil {
			return nil, "", err
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("uretim-%s-%s.xlsx", from.In(istanbul).Format("2006-01-02"), to.In(istanbul).Format("2006-01-02"))
	return buf.Bytes(), name, nil
}
