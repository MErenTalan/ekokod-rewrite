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
