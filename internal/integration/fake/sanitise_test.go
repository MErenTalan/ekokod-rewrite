package fake_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
)

// TestFixturesAreSanitised walks every adapter's testdata. It passes
// vacuously on an empty tree today; Task 17's matrix guard proves the tree
// is populated. The rule set itself is proven by TestSanitiserRejects.
func TestFixturesAreSanitised(t *testing.T) {
	files := fake.FixtureFiles(t) // glob internal/integration/*/testdata/**/*.json
	for _, f := range files {
		for _, v := range fake.Violations(f.Path, f.Body) {
			t.Errorf("%s: %s", f.Path, v)
		}
	}
}

func TestSanitiserRejects(t *testing.T) {
	for name, body := range map[string]string{
		"real token":     `{"access_token":"a1b2c3d4e5"}`,
		"jwt":            `{"x":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"}`,
		"real name":      `{"customerName":"Acme Tekstil A.Ş."}`,
		"installation":   `{"instalationNumber":"40012345"}`,
		"email":          `{"mail":"ali@firma.com.tr"}`,
		"host":           `{"url":"https://gateway.isolarcloud.eu/x"}`,
		"tckn":           `{"note":"12345678901"}`,
		"iban":           `{"iban":"TR330006100519786457841326"}`,
		"password field": `{"Password":"hunter2"}`,
	} {
		require.NotEmpty(t, fake.Violations("probe.json", []byte(body)), name)
	}
	require.Empty(t, fake.Violations("ok.json", []byte(
		`{"customerName":"Fixture Customer 1","instalationNumber":"FX0000001","access_token":"FIXTURE-TOKEN-1","url":"https://127.0.0.1/x"}`)))
}

// TestSanitiserRejectsPhoneNumber proves the phone rule from the brief's
// table, which TestSanitiserRejects above (copied verbatim from the brief)
// does not itself exercise.
func TestSanitiserRejectsPhoneNumber(t *testing.T) {
	require.NotEmpty(t, fake.Violations("probe.json", []byte(`{"contact":"+90 532 1234567"}`)))
}

// TestSanitiserAllowsFXUnderscoreForm proves the second allowed FX shape
// from the brief's table (underscores, not just digits) is not itself
// rejected.
func TestSanitiserAllowsFXUnderscoreForm(t *testing.T) {
	require.Empty(t, fake.Violations("ok.json", []byte(`{"ps_id":"FX1001_14_1_1"}`)))
}
