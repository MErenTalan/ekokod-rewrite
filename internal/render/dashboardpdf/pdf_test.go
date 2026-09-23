package dashboardpdf_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/dashboardpdf"
)

var update = flag.Bool("update", false, "rewrite testdata/dashboard.pdf.sha256")

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func document() dashboardpdf.Document {
	b1 := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	b2 := uuid.MustParse("66666666-7777-8888-9999-aaaaaaaaaaaa")
	price := d("3.12")
	row := func(b uuid.UUID, building, analyzer, kwh, cost string) billing.DashboardRow {
		id := uuid.MustParse("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
		return billing.DashboardRow{BillID: id, BuildingID: &b, AnalyzerID: &id, BuildingName: building,
			AnalyzerName: analyzer, InstallationNumber: "40001", EtsoCode: "40Z0000000001A", PeriodKey: "2026-08",
			Consumption: d(kwh), Production: d("0"), ConsumptionPrice: &price, Invoice: d(cost), Currency: model.CurrencyTRY}
	}
	res := billing.BuildDashboard([]billing.DashboardRow{
		row(b1, "Bina A", "Sayaç 1", "100", "250.50"),
		row(b1, "Bina A", "Sayaç 2", "50", "125.25"),
		row(b2, "ĞÜŞİÖÇ ğüşıöç Binası", "Sayaç 3", "200", "500"),
	}, nil, nil)
	return dashboardpdf.Document{Result: res, Period: "2026-08", CompanyName: "Ekokod Test A.Ş.",
		GeneratedAt: time.Date(2026, 9, 1, 6, 0, 0, 0, time.UTC)}
}

func TestRenderIsByteStable(t *testing.T) {
	for _, uncompressed := range []bool{false, true} {
		doc := document()
		doc.Uncompressed = uncompressed
		a, err := dashboardpdf.Render(doc)
		require.NoError(t, err)
		b, err := dashboardpdf.Render(doc)
		require.NoError(t, err)
		require.True(t, bytes.Equal(a, b), "uncompressed=%v", uncompressed)
		require.True(t, bytes.HasPrefix(a, []byte("%PDF-")))
	}
}

func TestRenderBothLocales(t *testing.T) {
	for _, locale := range []string{"tr", "en", ""} {
		doc := document()
		doc.Locale = locale
		out, err := dashboardpdf.Render(doc)
		require.NoError(t, err, locale)
		require.Greater(t, len(out), 1000, locale)
	}
}

func TestAMonthWithMoreInvoicesPrintsMore(t *testing.T) {
	// A weak but honest content check: the renderer draws the rows it is
	// given, so dropping a building must change the document.
	full, err := dashboardpdf.Render(document())
	require.NoError(t, err)
	doc := document()
	doc.Result.Buildings = doc.Result.Buildings[:1]
	partial, err := dashboardpdf.Render(doc)
	require.NoError(t, err)
	require.NotEqual(t, len(full), len(partial))
}

func TestAnEmptyMonthStillRenders(t *testing.T) {
	doc := document()
	doc.Result = billing.DashboardResult{}
	out, err := dashboardpdf.Render(doc)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(out, []byte("%PDF-")))
}

func TestRenderGoldenHash(t *testing.T) {
	out, err := dashboardpdf.Render(document())
	require.NoError(t, err)
	sum := sha256.Sum256(out)
	got := hex.EncodeToString(sum[:])
	path := "testdata/dashboard.pdf.sha256"
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(want)), got)
}
