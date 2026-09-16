package invoicepdf_test

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"os"
	"regexp"
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
	d := invoicepdf.Document{Bill: b, Lines: lines, CompanyName: "Ekokod Test A.Ş.", BuildingName: "ĞÜŞİÖÇ ğüşıöç Binası"}
	if scope != model.BillScopeAnalyzer {
		d.Members = []invoicepdf.Member{{AnalyzerID: id, Name: "Sayaç 1", InstallationNumber: "T-1"}, {AnalyzerID: id, Name: "Sayaç 2", InstallationNumber: "T-2"}}
	}
	if scope == model.BillScopeCompany {
		d.BuildingName = ""
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
