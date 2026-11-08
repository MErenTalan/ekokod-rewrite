package invoicepdf_test

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/invoicepdf"
)

var update = flag.Bool("update", false, "rewrite testdata/analyzer_invoice.pdf.sha256")

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func document(scope model.BillScope) invoicepdf.Document {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	unit := "kWh"
	eff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := model.Bill{
		ID: id, Scope: scope, PeriodKey: "2026-01", DaysInPeriod: 31, Currency: model.CurrencyTRY,
		PeriodStart: time.Date(2026, 1, 14, 21, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 2, 14, 21, 0, 0, 0, time.UTC),
		TariffEffectiveFrom: &eff, ActiveImport: decimal.RequireFromString("12345.678"), NetConsumption: decimal.RequireFromString("12345.678"),
		InductiveKvarh: decimal.RequireFromString("800"), CapacitiveKvarh: decimal.RequireFromString("12"),
		IndexStart: json.RawMessage(`{"active_import":"1000","t1_import":null}`), IndexEnd: json.RawMessage(`{"active_import":"13345.678","t1_import":"5"}`),
		VatBase: decimal.RequireFromString("40000.10"), VatCost: decimal.RequireFromString("8000.02"), TotalCost: decimal.RequireFromString("48000.12"),
		InductiveRatio: dp("0.0648"), InductiveThreshold: dp("0.20"), CapacitiveThreshold: dp("0.15"),
		PtfYekdemUsed: true, Status: model.BillStatusIssued, ComputedAt: time.Date(2026, 2, 18, 9, 30, 0, 0, time.UTC),
	}
	expected, matched, missing := int32(744), int32(744), int32(0)
	b.PtfHoursExpected, b.PtfHoursMatched, b.PtfHoursMissing, b.PtfAverage, b.YekdemUsed = &expected, &matched, &missing, dp("2500.5"), dp("500")
	pct := "%"
	lines := []model.BillLine{
		{Code: model.BillLineEnergy, Quantity: dp("12345.678"), Unit: &unit, UnitPrice: dp("3.116667"), Amount: decimal.RequireFromString("38477.36"), SortOrder: 0},
		{Code: "tax:BTV", Unit: &pct, RatePct: dp("5"), Amount: decimal.RequireFromString("1522.74"), SortOrder: 1},
		{Code: "extra:Kapasite", Quantity: dp("1"), UnitPrice: dp("25"), Amount: decimal.RequireFromString("25"), SortOrder: 2},
		{Code: model.BillLineVat, Unit: &pct, RatePct: dp("20"), Amount: decimal.RequireFromString("8000.02"), SortOrder: 3},
	}
	name, contracted, installed := "Merkez OSB", decimal.RequireFromString("250"), decimal.RequireFromString("320")
	tar := &model.Tariff{Name: &name, UserGroup: model.UserGroupIndustrial, VoltageLevel: model.VoltageLevelMV, Term: model.TariffTermBinomial,
		PriceType: model.PriceTypeSingleTime, SupplyCompany: model.SupplyCompanyPrivate, UsePtfYekdem: true, ContractedPowerKw: &contracted}
	d := invoicepdf.Document{Bill: b, Lines: lines, CompanyName: "Ekokod Test A.Ş.", BuildingName: "ĞÜŞİÖÇ ğüşıöç Binası",
		Tariff: tar, InstalledPowerKw: &installed, Members: []invoicepdf.Member{{AnalyzerID: id, Name: "Ana pano", InstallationNumber: "4001234567"}}}
	if scope != model.BillScopeAnalyzer {
		d.Members = []invoicepdf.Member{{AnalyzerID: id, Name: "Sayaç 1", InstallationNumber: "T-1"}, {AnalyzerID: id, Name: "Sayaç 2", InstallationNumber: "T-2"}}
	}
	if scope == model.BillScopeCompany {
		d.BuildingName, d.Tariff, d.InstalledPowerKw = "", nil, nil
	}
	return d
}

func TestRenderIsByteStable(t *testing.T) {
	for _, uncompressed := range []bool{false, true} {
		d := document(model.BillScopeAnalyzer)
		d.Uncompressed = uncompressed
		a, err := invoicepdf.Render(d)
		require.NoError(t, err)
		b, err := invoicepdf.Render(d)
		require.NoError(t, err)
		require.True(t, bytes.Equal(a, b), "uncompressed=%v", uncompressed)
		require.True(t, bytes.HasPrefix(a, []byte("%PDF-")))
	}
}

// TestRenderContainsTurkishGlyphs: fpdf's ToUnicode CMap is an identity range
// (CID = code point), so the proof is the embedded subset's CIDToGIDMap: every
// Turkish letter the document draws maps to a real glyph, an undrawn one does not.
func TestRenderContainsTurkishGlyphs(t *testing.T) {
	d := document(model.BillScopeAnalyzer)
	d.Uncompressed = true
	out, err := invoicepdf.Render(d)
	require.NoError(t, err)
	maps := cidToGIDMaps(t, out)
	require.NotEmpty(t, maps, "no embedded UTF-8 font")
	glyph := func(cp int) bool {
		for _, m := range maps {
			if cp*2+1 < len(m) && (m[cp*2] != 0 || m[cp*2+1] != 0) {
				return true
			}
		}
		return false
	}
	for _, cp := range []int{0x011F, 0x015F, 0x0131, 0x0130, 0x00FC, 0x00F6, 0x00E7, 0x011E, 0x015E, 0x00DC, 0x00D6, 0x00C7} {
		require.True(t, glyph(cp), "U+%04X has no embedded glyph", cp)
	}
	require.False(t, glyph(0x2603), "positive control: an undrawn character is not in the subset")
}

func cidToGIDMaps(t *testing.T, pdf []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for _, m := range regexp.MustCompile(`/CIDToGIDMap (\d+) 0 R`).FindAllSubmatch(pdf, -1) {
		obj := []byte("\n" + string(m[1]) + " 0 obj")
		i := bytes.Index(pdf, obj)
		require.GreaterOrEqual(t, i, 0)
		j := bytes.Index(pdf[i:], []byte("stream\n")) + i + len("stream\n")
		k := bytes.Index(pdf[j:], []byte("endstream")) + j
		r, err := zlib.NewReader(bytes.NewReader(pdf[j:k]))
		require.NoError(t, err)
		raw, err := io.ReadAll(r)
		require.NoError(t, err)
		out = append(out, raw)
	}
	return out
}

func TestRenderListsMissingDays(t *testing.T) {
	d := document(model.BillScopeAnalyzer)
	d.Uncompressed = true
	d.Bill.MissingDays = []string{"2026-01-20", "2026-01-21"}
	out, err := invoicepdf.Render(d)
	require.NoError(t, err)
	base := document(model.BillScopeAnalyzer)
	base.Uncompressed = true
	plain, err := invoicepdf.Render(base)
	require.NoError(t, err)
	require.Greater(t, len(out), len(plain), "the missing-days note is drawn")
}

// prosumer is a hourly-netted residential bill with the informational sale (R531).
func prosumer() invoicepdf.Document {
	d := document(model.BillScopeAnalyzer)
	unit := "kWh"
	d.Bill.ActiveImport, d.Bill.ActiveExport, d.Bill.NetConsumption = decimal.RequireFromString("315.091"), decimal.RequireFromString("476.321"), decimal.RequireFromString("309.4")
	d.Lines = []model.BillLine{
		{Code: model.BillLineEnergy, Quantity: dp("309.4"), Unit: &unit, UnitPrice: dp("1.212443"), Amount: decimal.RequireFromString("375.13"), SortOrder: 0},
		d.Lines[3],
		{Code: model.BillLineGenerationSale, Quantity: dp("470.63"), Unit: &unit, UnitPrice: dp("0.9"), Amount: decimal.RequireFromString("423.57"), SortOrder: 5},
	}
	return d
}

func TestRenderPrintsGenerationSaleNote(t *testing.T) {
	d := prosumer()
	d.Uncompressed = true
	out, err := invoicepdf.Render(d)
	require.NoError(t, err)
	without := prosumer()
	without.Uncompressed = true
	without.Lines = without.Lines[:2]
	plain, err := invoicepdf.Render(without)
	require.NoError(t, err)
	require.Greater(t, len(out), len(plain), "the sale note is drawn")
}

func TestRenderGoldenHash(t *testing.T) {
	out, err := invoicepdf.Render(document(model.BillScopeAnalyzer))
	require.NoError(t, err)
	sum := sha256.Sum256(out)
	got := hex.EncodeToString(sum[:])
	path := "testdata/analyzer_invoice.pdf.sha256"
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(want)), got)
}

func TestRenderAllScopes(t *testing.T) {
	for _, scope := range model.BillScopes() {
		if scope != model.BillScopeAnalyzer && scope != model.BillScopeBuilding && scope != model.BillScopeCompany {
			continue
		}
		for _, locale := range []string{"tr", "en"} {
			d := document(scope)
			d.Locale, d.Uncompressed = locale, true
			d.Bill.Status = model.BillStatusFlagged
			reason := "ptf_data_missing"
			d.Bill.FlagReason = &reason
			out, err := invoicepdf.Render(d)
			require.NoError(t, err, "%s/%s", scope, locale)
			require.Greater(t, len(out), 1000)
		}
	}
}

func pageCount(t *testing.T, pdf []byte) int {
	t.Helper()
	m := regexp.MustCompile(`/Type /Pages\s*/Kids \[[^\]]*\]\s*/Count (\d+)`).FindSubmatch(pdf)
	require.NotNil(t, m, "no page tree")
	n, err := strconv.Atoi(string(m[1]))
	require.NoError(t, err)
	return n
}

// A long meter list flows onto further pages (the table repeats its header).
func TestRenderLongMemberListAddsPages(t *testing.T) {
	d := document(model.BillScopeCompany)
	d.Uncompressed = true
	out, err := invoicepdf.Render(d)
	require.NoError(t, err)
	require.Equal(t, 1, pageCount(t, out))
	for i := range 60 {
		d.Members = append(d.Members, invoicepdf.Member{Name: fmt.Sprintf("Sayaç %d", i), InstallationNumber: fmt.Sprintf("T-%d", i)})
	}
	out, err = invoicepdf.Render(d)
	require.NoError(t, err)
	require.Greater(t, pageCount(t, out), 1)
}

var samples = flag.String("samples", "", "write sample invoices into this directory for a visual review")

// TestWriteSamples renders the review set: go test -run TestWriteSamples -samples=DIR.
func TestWriteSamples(t *testing.T) {
	if *samples == "" {
		t.Skip("no -samples directory")
	}
	write := func(file string, d invoicepdf.Document) {
		out, err := invoicepdf.Render(d)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(*samples, file), out, 0o644))
	}
	b := document(model.BillScopeBuilding)
	b.Bill.MaxDemandKw = dp("210.5")
	write("building.pdf", b)

	a := document(model.BillScopeAnalyzer)
	unit := "kVArh"
	a.Bill.ReactivePenaltyApplied, a.Bill.InductiveRatio = true, dp("0.648")
	a.Lines = append(a.Lines[:1:1], model.BillLine{Code: model.BillLineReactiveInductive, Quantity: dp("800"), Unit: &unit, UnitPrice: dp("1.234567"), Amount: decimal.RequireFromString("987.65"), SortOrder: 0},
		model.BillLine{Code: model.BillLineReactiveWaiver, Amount: decimal.RequireFromString("-987.65"), SortOrder: 0})
	a.Lines = append(a.Lines, document(model.BillScopeAnalyzer).Lines[1:]...)
	write("analyzer-waived.pdf", a)

	c := document(model.BillScopeCompany)
	c.Locale = "en"
	for i := range 40 {
		c.Members = append(c.Members, invoicepdf.Member{Name: "Meter", InstallationNumber: fmt.Sprintf("T-%d", i)})
	}
	write("company-en.pdf", c)

	f := document(model.BillScopeBuilding)
	reason := "ptf_data_missing,installed_power_missing"
	f.Bill.Status, f.Bill.FlagReason = model.BillStatusFlagged, &reason
	f.Bill.MissingDays = []string{"2026-01-20", "2026-01-21", "2026-01-25"}
	f.Bill.InductiveThreshold, f.Bill.CapacitiveThreshold = nil, nil
	write("flagged.pdf", f)
	write("prosumer.pdf", prosumer())

	// F30: the utility's 16:00 reading window, energy + distribution = the all-in unit price.
	h := document(model.BillScopeBuilding)
	kwh := "kWh"
	h.Bill.PeriodStart, h.Bill.PeriodEnd = time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC), time.Date(2026, 9, 16, 13, 0, 0, 0, time.UTC)
	h.Lines = append([]model.BillLine{
		{Code: model.BillLineEnergy, Quantity: dp("17240.84"), Unit: &kwh, UnitPrice: dp("3.774401"), Amount: decimal.RequireFromString("65074.06")},
		{Code: model.BillLineDistribution, Quantity: dp("17240.84"), Unit: &kwh, UnitPrice: dp("2.4794"), Amount: decimal.RequireFromString("42746.94")},
	}, h.Lines[1:]...)
	write("reading-hours.pdf", h)
}

func TestBoundShowsTheReadingHourOnlyWhenItMatters(t *testing.T) {
	require.Equal(t, "17.07.2026", invoicepdf.Bound(time.Date(2026, 7, 16, 21, 0, 0, 0, time.UTC)))
	require.Equal(t, "17.07.2026 16:00", invoicepdf.Bound(time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)))
}

func TestUnitSummaryTextAddsEnergyAndDistribution(t *testing.T) {
	kwh := "kWh"
	lines := []model.BillLine{
		{Code: model.BillLineEnergy, Quantity: dp("17240.84"), Unit: &kwh, UnitPrice: dp("3.774401")},
		{Code: model.BillLineDistribution, Quantity: dp("17240.84"), Unit: &kwh, UnitPrice: dp("2.4794")},
	}
	require.Equal(t, "Birim fiyat: Elektrik enerji bedeli 3,774401 + Dağıtım bedeli 2,479400 = Toplam birim fiyat 6,253801 TL/kWh",
		invoicepdf.UnitSummaryText("tr", lines))
	lines[0].UnitPrice = nil
	require.Equal(t, "Birim fiyat: Elektrik enerji bedeli — + Dağıtım bedeli 2,479400 = Toplam birim fiyat — TL/kWh",
		invoicepdf.UnitSummaryText("tr", lines), "R560: unpriced PTF energy")
	require.Empty(t, invoicepdf.UnitSummaryText("tr", []model.BillLine{{Code: model.BillLineVat}}))
}
