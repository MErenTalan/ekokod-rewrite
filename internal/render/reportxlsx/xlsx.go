// Package reportxlsx renders a stored report payload as a workbook: the
// report sheet carries every section of internal/render/reportview, and a
// data sheet feeds one native chart per chart section (R271).
package reportxlsx

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

// SheetName is the report sheet's name in a locale.
func SheetName(locale string) string {
	if locale == "en" {
		return "Report"
	}
	return "Rapor"
}

// DataSheetName is the chart data sheet's name in a locale.
func DataSheetName(locale string) string {
	if locale == "en" {
		return "Chart data"
	}
	return "Grafik verisi"
}

// Render builds the workbook.
func Render(p report.Payload, companyName string, buildingNames []string, locale string) ([]byte, error) {
	doc := reportview.Build(p, companyName, buildingNames, locale)
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	main, data := SheetName(doc.Locale), DataSheetName(doc.Locale)
	if err := f.SetSheetName("Sheet1", main); err != nil {
		return nil, fmt.Errorf("reportxlsx: %w", err)
	}
	if _, err := f.NewSheet(data); err != nil {
		return nil, fmt.Errorf("reportxlsx: %w", err)
	}
	w := &writer{f: f, main: main, data: data}
	w.line(doc.Title)
	for _, l := range doc.Lines {
		w.line(l)
	}
	for _, n := range doc.Notes {
		w.line(n)
	}
	for _, s := range doc.Sections {
		w.row++
		start := w.row
		w.line(s.Title)
		if len(s.Header) > 0 {
			w.cells(s.Header)
		}
		for _, r := range s.Rows {
			w.cells(r)
		}
		for _, n := range s.Notes {
			w.line(n)
		}
		if s.Chart != nil {
			if err := w.chart(s.Title, *s.Chart, start); err != nil {
				return nil, err
			}
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	_ = f.SetColWidth(main, "A", "A", 48)
	_ = f.SetColWidth(main, "B", "E", 24)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("reportxlsx: %w", err)
	}
	return buf.Bytes(), nil
}

type writer struct {
	f            *excelize.File
	main, data   string
	row, dataRow int
	err          error
}

func (w *writer) line(text string) { w.cells([]string{text}) }

func (w *writer) cells(values []string) {
	if w.err != nil {
		return
	}
	w.row++
	row := make([]any, len(values))
	for i, v := range values {
		row[i] = v
	}
	cell, _ := excelize.CoordinatesToCellName(1, w.row)
	if err := w.f.SetSheetRow(w.main, cell, &row); err != nil {
		w.err = fmt.Errorf("reportxlsx: %w", err)
	}
}

// chart writes the series as numbers on the data sheet and anchors a
// clustered column chart beside the section. The float conversion happens
// only here, for the cell a chart reads; the report sheet keeps the exact
// formatted decimals.
func (w *writer) chart(title string, c reportview.Chart, anchorRow int) error {
	w.dataRow++
	head := w.dataRow
	header := []any{title}
	for _, s := range c.Series {
		header = append(header, s.Name)
	}
	cell, _ := excelize.CoordinatesToCellName(1, head)
	if err := w.f.SetSheetRow(w.data, cell, &header); err != nil {
		return fmt.Errorf("reportxlsx: %w", err)
	}
	for i, cat := range c.Categories {
		w.dataRow++
		row := []any{cat}
		for _, s := range c.Series {
			if v := s.Values[i]; v != nil {
				row = append(row, v.InexactFloat64())
			} else {
				row = append(row, nil) // a gap, not a zero
			}
		}
		cell, _ := excelize.CoordinatesToCellName(1, w.dataRow)
		if err := w.f.SetSheetRow(w.data, cell, &row); err != nil {
			return fmt.Errorf("reportxlsx: %w", err)
		}
	}
	first, last := head+1, w.dataRow
	ref := func(col, from, to int) string {
		a, _ := excelize.CoordinatesToCellName(col, from, true)
		b, _ := excelize.CoordinatesToCellName(col, to, true)
		return fmt.Sprintf("'%s'!%s:%s", w.data, a, b)
	}
	var series []excelize.ChartSeries
	for i := range c.Series {
		name, _ := excelize.CoordinatesToCellName(i+2, head, true)
		series = append(series, excelize.ChartSeries{
			Name: fmt.Sprintf("'%s'!%s", w.data, name), Categories: ref(1, first, last), Values: ref(i+2, first, last),
		})
	}
	w.dataRow++ // a blank row between blocks
	anchor, _ := excelize.CoordinatesToCellName(7, anchorRow)
	return w.f.AddChart(w.main, anchor, &excelize.Chart{
		Type: excelize.Col, Series: series,
		Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: title}}},
		YAxis:     excelize.ChartAxis{Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: c.Unit}}}},
		Legend:    excelize.ChartLegend{Position: "bottom"},
		Dimension: excelize.ChartDimension{Width: 560, Height: 280},
	})
}
