package reportpdf

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportview"
)

// recorder is a drawer that remembers what it was asked to draw. The PDF's
// own text is glyph-encoded (UTF-8 fonts), so this is how a test sees it.
type recorder struct {
	headings []string
	rows     map[string]int
	charts   map[string]int
	notes    []string
	current  string
}

func (r *recorder) title(string, []string)      {}
func (r *recorder) heading(s string, _ float64) { r.headings = append(r.headings, s); r.current = s }
func (r *recorder) table(_ []string, rows [][]string) {
	r.rows[r.current] += len(rows)
}
func (r *recorder) chart(c reportview.Chart) { r.charts[r.current] = len(c.Categories) }
func (r *recorder) note(s string)            { r.notes = append(r.notes, s) }

func record(doc reportview.Document) *recorder {
	r := &recorder{rows: map[string]int{}, charts: map[string]int{}}
	draw(doc, r)
	return r
}

func titles(doc reportview.Document) []string {
	var out []string
	for _, s := range doc.Sections {
		out = append(out, s.Title)
	}
	return out
}

func TestMonthlyPDFHasEverySection(t *testing.T) {
	for _, locale := range []string{"tr", "en"} {
		doc := reportview.Build(monthlyPayload(), "Ekokod A.Ş.", []string{"Merkez"}, locale)
		r := record(doc)
		require.Equal(t, titles(doc), r.headings, locale)
		require.Len(t, doc.Sections, 4)
		for _, s := range doc.Sections {
			require.Equal(t, len(s.Rows), r.rows[s.Title], "every row of %q is drawn", s.Title)
			if s.Chart != nil {
				require.Equal(t, 12, r.charts[s.Title], "%q draws its chart", s.Title)
			}
		}
	}
}

func TestYearlyPDFHasEverySection(t *testing.T) {
	for _, locale := range []string{"tr", "en"} {
		doc := reportview.Build(yearlyPayload(), "Ekokod A.Ş.", []string{"Merkez"}, locale)
		r := record(doc)
		require.Equal(t, titles(doc), r.headings, locale)
		require.Len(t, doc.Sections, 8)
		for _, s := range doc.Sections {
			require.Equal(t, len(s.Rows), r.rows[s.Title], "every row of %q is drawn", s.Title)
			if s.Chart != nil {
				require.Positive(t, r.charts[s.Title], "%q draws its chart", s.Title)
			}
		}
		require.NotEmpty(t, r.notes, "the carbon factor note is printed")
	}
}
