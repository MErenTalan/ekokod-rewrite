package carbonview_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/carbonview"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func payload(kind string) domain.ReportPayload {
	src, year := "Türkiye Ulusal Envanteri", int16(2025)
	addr := "Merkez Cad. 1"
	p := domain.ReportPayload{ReportType: kind, Company: "Acme Enerji", Building: "Merkez", Address: &addr,
		From: "2026-01-01", To: "2026-06-30", GeneratedAt: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		TotalKgCO2e: d("1234.5678"), Pending: 2,
		Figures94: &domain.Payload94{ConsumptionKwh: d("10000"), GenerationKwh: d("2000"), GridFactor: d("0.469"),
			GridFactorUnit: "kg CO2e/kWh", GridFactorSource: &src, GridFactorYear: &year,
			ConsumptionT: d("4.69"), ProductionReductT: d("0.938"), NetT: d("3.752")}}
	if kind == "ghg" {
		p.Groups = []domain.PayloadGroup{
			{Key: "scope_1", TotalKgCO2e: d("1000"), Lines: []domain.PayloadLine{{Sub: "sub_space_heating", KgCO2e: d("1000")}}},
			{Key: "scope_2", TotalKgCO2e: d("234.5678"), Lines: []domain.PayloadLine{{Sub: "sub_grid_electricity", KgCO2e: d("234.5678")}}},
			{Key: "scope_3", TotalKgCO2e: d("0")},
		}
	} else {
		for _, c := range domain.ISOCategories {
			p.Groups = append(p.Groups, domain.PayloadGroup{Key: c, TotalKgCO2e: d("0")})
		}
		p.Groups[5] = domain.PayloadGroup{Key: "category_6", TotalKgCO2e: d("1234.5678"),
			Lines: []domain.PayloadLine{{Sub: "sub_waste_disposal", KgCO2e: d("1234.5678")}}}
	}
	return p
}

func TestGHGDocumentTurkish(t *testing.T) {
	doc := carbonview.Build(payload("ghg"), "tr")
	require.Equal(t, "GHG Protokolü Karbon Ayak İzi Raporu", doc.Title)
	text := carbonview.Text(doc)
	for _, want := range []string{"Acme Enerji", "Merkez", "Merkez Cad. 1", "01.01.2026 – 30.06.2026",
		"Kapsam 1", "Kapsam 2", "Kapsam 3", "Ortam Isıtması", "1.000,00", "234,57", "Toplam: 1.234,57 kg CO₂e",
		"2 kayıt onay bekliyor", "0,469 kg CO2e/kWh", "Türkiye Ulusal Envanteri, 2025", "4,690 t", "0,938 t", "3,752 t"} {
		require.True(t, strings.Contains(text, want), "missing %q in:\n%s", want, text)
	}
}

func TestISODocumentEnglishHasSixCategories(t *testing.T) {
	doc := carbonview.Build(payload("iso"), "en")
	require.Equal(t, "ISO 14064 Carbon Footprint Report", doc.Title)
	text := carbonview.Text(doc)
	for _, want := range []string{"Category 1", "Category 6", "Waste disposal", "1,234.57", "2 records await approval", "2025"} {
		require.True(t, strings.Contains(text, want), "missing %q in:\n%s", want, text)
	}
	groups := 0
	for _, s := range doc.Sections {
		if strings.HasPrefix(s.Key, "category_") {
			groups++
		}
	}
	require.Equal(t, 6, groups, "every ISO category, zeros included")
}

func TestNoMeterFiguresSaysSo(t *testing.T) {
	p := payload("ghg")
	p.Figures94 = nil
	p.Pending = 0
	text := carbonview.Text(carbonview.Build(p, "tr"))
	require.Contains(t, text, "Sayaç verisinden otomatik kayıt yok")
	require.NotContains(t, text, "onay bekliyor")
}
