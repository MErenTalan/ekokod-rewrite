package reportpdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

var update = flag.Bool("update", false, "rewrite testdata/*.sha256")

func document(p report.Payload) Document {
	return Document{Payload: p, CompanyName: "Ekokod Test A.Ş.", BuildingNames: []string{"ĞÜŞİÖÇ Merkez"},
		GeneratedAt: time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)}
}

func TestPDFIsByteStable(t *testing.T) {
	for _, p := range []report.Payload{monthlyPayload(), yearlyPayload()} {
		for _, locale := range []string{"tr", "en"} {
			doc := document(p)
			doc.Locale = locale
			a, err := Render(doc)
			require.NoError(t, err)
			b, err := Render(doc)
			require.NoError(t, err)
			require.True(t, bytes.Equal(a, b), "%s %s", p.Type, locale)
			require.True(t, bytes.HasPrefix(a, []byte("%PDF-")))
		}
	}
}

func TestPDFGoldenHash(t *testing.T) {
	for name, p := range map[string]report.Payload{"monthly": monthlyPayload(), "yearly": yearlyPayload()} {
		out, err := Render(document(p))
		require.NoError(t, err)
		sum := sha256.Sum256(out)
		got := hex.EncodeToString(sum[:])
		path := "testdata/" + name + ".pdf.sha256"
		if *update {
			require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, strings.TrimSpace(string(want)), got, name)
	}
}

func TestRenderMissingIsNoData(t *testing.T) {
	// An empty payload (a pending report's {}) must not panic.
	out, err := Render(document(report.Payload{}))
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(out, []byte("%PDF-")))
}

// F10a R312: any reportview document renders through the same page code.
func TestRenderViewIsDeterministic(t *testing.T) {
	view := reportview.Document{Locale: "tr", Title: "Karbon", Lines: []string{"Şirket: Acme"},
		Sections: []reportview.Section{{Key: "scope_1", Title: "Kapsam 1", Header: []string{"Alt kategori", "kg CO₂e"},
			Rows: [][]string{{"Ortam Isıtması", "1.000,00"}}}}, Notes: []string{"Toplam: 1.000,00 kg CO₂e"}}
	at := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	a, err := RenderView(view, "Karbon 2026", at)
	require.NoError(t, err)
	b, err := RenderView(view, "Karbon 2026", at)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(a, []byte("%PDF-")))
	require.Equal(t, a, b)
}
