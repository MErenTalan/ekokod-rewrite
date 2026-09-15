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
	files := fake.FixtureFiles(t) // glob internal/integration/*/testdata/**/*.{json,xml,csv,txt}
	for _, f := range files {
		for _, v := range fake.Violations(f.Path, f.Body) {
			t.Errorf("%s: %s", f.Path, v)
		}
	}
}

// TestSanitiserRejects is the brief's original table plus every evasion the
// fix-round-1 review (task-4-fix1-findings.md, I1-I3) demonstrated getting
// past the previous implementation: numbers and object keys never scanned
// (I1), the OSOS/ARIL field names and Turkish spellings missing from the
// PII key set plus out-of-range coordinates (I2), and everyday formatting
// (spaced IBAN/phone, bare hostnames, URL userinfo) evading the shape rules
// (I3).
func TestSanitiserRejects(t *testing.T) {
	for name, body := range map[string]string{
		// --- brief's original acceptance cases ---
		"real token":     `{"access_token":"a1b2c3d4e5"}`,
		"jwt":            `{"x":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"}`,
		"real name":      `{"customerName":"Acme Tekstil A.Ş."}`,
		"installation":   `{"instalationNumber":"40012345"}`,
		"email":          `{"mail":"ali@firma.com.tr"}`,
		"host":           `{"url":"https://gateway.isolarcloud.eu/x"}`,
		"tckn":           `{"note":"12345678901"}`,
		"iban":           `{"iban":"TR330006100519786457841326"}`,
		"password field": `{"Password":"hunter2"}`,

		// --- I1: numbers and object keys never scanned ---
		"tckn as bare JSON number":           `{"tckn":12345678901}`,
		"installation number as JSON number": `{"instalationNumber":40012345}`,
		"ps_id as JSON number":               `{"ps_id":1234567}`,
		"tckn and email smuggled as keys":    `{"12345678901":{"ali@firma.com.tr":"x"}}`,

		// --- I2: PII key set misses spec field names / Turkish spellings,
		// coordinates not range-checked ---
		"customerAdress not Fixture-prefixed": `{"customerAdress":"Gerçek Adres 123"}`, //nolint:misspell // OSOS field spelling + Turkish word, not a typo
		"il not Fixture-prefixed":             `{"il":"Ankara"}`,
		"ilce not Fixture-prefixed":           `{"ilce":"Çankaya"}`,
		"koyMahallesi not Fixture-prefixed":   `{"koyMahallesi":"Gerçek Mahalle"}`,
		"caddesiSokagi not Fixture-prefixed":  `{"caddesiSokagi":"Gerçek Cadde"}`,
		"muhatapNo not FX-prefixed":           `{"muhatapNo":"12345"}`,
		"sayimNokTanim (company) not Fixture": `{"sayimNokTanim":"Acme Fabrika"}`,
		"koordinatX out of range":             `{"koordinatX":45.531}`,
		"koordinatY out of range":             `{"koordinatY":28.98}`,
		"company not Fixture-prefixed":        `{"company":"Acme Corp"}`,
		"musteriAdi not Fixture-prefixed":     `{"musteriAdi":"Ali Veli"}`,
		"adres not Fixture-prefixed":          `{"adres":"Gerçek Sokak No:5"}`, //nolint:misspell // "adres" is the OSOS field name under test, not a typo
		"neighborhood not Fixture-prefixed":   `{"neighborhood":"Real Neighborhood"}`,

		// --- I3: everyday formatting evades shape rules ---
		"spaced IBAN":               `{"iban":"TR33 0006 1005 1978 6457 8413 26"}`,
		"spaced local mobile":       `{"contact":"0532 123 45 67"}`,
		"formatted mobile with +90": `{"contact":"+90 (532) 123-45-67"}`,
		"bare hostname, no scheme":  `{"endpoint":"gateway.isolarcloud.eu/openapi"}`,
		"URL with userinfo":         `{"url":"https://user:pass@gateway.isolarcloud.eu/x"}`,
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

// TestSanitiserAllowsI4FalsePositiveFixture is I4's exact must-pass
// fixture: an ordinary protocol payload (result code, currency code, an
// energy reading formatted as a decimal string, an already-sanitised
// ps_id) that the pre-fix sanitiser flagged with 4 false-positive
// violations (the "code" substring in result_code/currencyCode tripping
// the old secret-key rule, and "90433" inside "1890433.5" tripping the old
// unanchored phone rule).
func TestSanitiserAllowsI4FalsePositiveFixture(t *testing.T) {
	require.Empty(t, fake.Violations("ok.json", []byte(
		`{"result_code":"1","currencyCode":"TRY","energy":"1890433.5","ps_id":"FX1901234"}`)))
}

// TestSanitiserAllowsValidCoordinates proves the coordinate range check
// (I2) does not reject values inside the allowed fixture bounds.
func TestSanitiserAllowsValidCoordinates(t *testing.T) {
	require.Empty(t, fake.Violations("ok.json", []byte(`{"koordinatX":39.5,"koordinatY":32.5}`)))
}
