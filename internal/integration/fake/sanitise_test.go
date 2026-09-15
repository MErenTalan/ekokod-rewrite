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

// TestSanitiserWholeKeyProvinceMatch is fix-round-2's R1 controller ruling:
// the OSOS province key `il` must be matched as a WHOLE key (`^il$`,
// case-insensitive), never a suffix — the old unanchored `il$` stem matched
// any key ENDING in "il", so a compliant `{"email":"..."}` fixture was
// rejected as if "email" were the Turkish province field. Must-pass cases
// prove the fix; the must-reject case proves the real `il` field is still
// caught.
func TestSanitiserWholeKeyProvinceMatch(t *testing.T) {
	for name, body := range map[string]string{
		"email key, valid @example.com address":        `{"email":"someone@example.com"}`,
		"mail key, valid @example.com address":         `{"mail":"another@example.com"}`,
		"contactEmail key, valid @example.com address": `{"contactEmail":"x@example.com"}`,
	} {
		require.Empty(t, fake.Violations("ok.json", []byte(body)), name)
	}

	require.NotEmpty(t, fake.Violations("probe.json", []byte(`{"il":"Ankara"}`)), "il")
}

// TestSanitiserWordTokenisedKeyMatching is fix-round-3's controller ruling
// test table in full: PII key matching decides on whole WORDS
// (tokeniseKey/isNameLikePII/piiWords in sanitise.go), never substrings and
// never a single exact whole-key anchor — both of which earlier rounds
// tried, and each had a false result in the opposite direction:
//
//   - fix-round-1's unanchored `il$` SUBSTRING stem rejected any key ending
//     in "il", including the compliant "email"/"mail"/"contactEmail".
//   - fix-round-2's `^il$`/`^sayimnoktanim$` EXACT WHOLE-KEY anchors fixed
//     that, but an exact whole-key anchor stops matching the instant the
//     key is spelled with an extra separator/case/suffix: `ilAdi`,
//     `il_adi`, `musteriIl`, `adresIl` (never equal "il"), and
//     `sayimNoktaTanimi`/`SAYIM_NOK_TANIM`/`sayim-nokta-tanimi` (never
//     byte-equal to "sayimnoktanim" after lower-casing) all wrongly passed.
//
// Word tokenisation catches every spelling of the must-reject cases while
// still passing "tesisatTurTanim" (a bare "tanim" word, no "sayim" word —
// not PII on its own) and the other non-PII protocol/English keys.
func TestSanitiserWordTokenisedKeyMatching(t *testing.T) {
	mustReject := map[string]string{
		"il":                 `{"il":"Ankara"}`,
		"ilAdi":              `{"ilAdi":"Ankara"}`,
		"il_adi":             `{"il_adi":"Ankara"}`,
		"musteriIl":          `{"musteriIl":"Ankara"}`,
		"adresIl":            `{"adresIl":"Ankara"}`,
		"sayimNokTanim":      `{"sayimNokTanim":"Acme Fabrika"}`,
		"sayimNoktaTanimi":   `{"sayimNoktaTanimi":"Acme Fabrika"}`,
		"SAYIM_NOK_TANIM":    `{"SAYIM_NOK_TANIM":"Acme Fabrika"}`,
		"sayim-nokta-tanimi": `{"sayim-nokta-tanimi":"Acme Fabrika"}`,
		"customerAdress":     `{"customerAdress":"Gerçek Adres 123"}`, //nolint:misspell // OSOS field spelling, not a typo
		"koyMahallesi":       `{"koyMahallesi":"Gerçek Mahalle"}`,
		"caddesiSokagi":      `{"caddesiSokagi":"Gerçek Cadde"}`,
		"İlçe":               `{"İlçe":"Çankaya"}`,
	}
	for name, body := range mustReject {
		require.NotEmpty(t, fake.Violations("probe.json", []byte(body)), name)
	}

	mustPass := map[string]string{
		"email":           `{"email":"someone@example.com"}`,
		"mail":            `{"mail":"another@example.com"}`,
		"contactEmail":    `{"contactEmail":"x@example.com"}`,
		"tesisatTurTanim": `{"tesisatTurTanim":"Sanayi"}`,
		"deviceName":      `{"deviceName":"Inverter-1"}`,
		"unitName":        `{"unitName":"Unit-7"}`,
		"result_code":     `{"result_code":"0"}`,
		"currencyCode":    `{"currencyCode":"TRY"}`,
		"detail":          `{"detail":"ok"}`,
		"utilization":     `{"utilization":"72.3"}`,
	}
	for name, body := range mustPass {
		require.Empty(t, fake.Violations("ok.json", []byte(body)), name)
	}
}

// TestSanitiserAcronymBoundaryTokenisation is fix-round-4's controller
// ruling: tokeniseKey's lower→upper boundary rule alone never fires
// inside a run of consecutive uppercase letters, so a leading acronym
// fuses with the word that follows it instead of tokenising separately —
// "XMLCustomerID" tokenised to the single fused word "xmlcustomer" plus
// "id", which never matches the "customer" PII word (false negative).
// The added acronym rule splits an uppercase letter off as the start of a
// new word when the letter immediately after it is lowercase and the
// letter immediately before it is also uppercase, so "XML" + "Customer"
// (and "ID" + "No") tokenise as separate words. Must-reject cases prove
// the acronym-prefixed PII words are now caught; must-pass cases prove
// acronym-prefixed non-PII words (and a bare acronym-only word, "IDNo")
// still pass.
func TestSanitiserAcronymBoundaryTokenisation(t *testing.T) {
	mustReject := map[string]string{
		"XMLCustomerID":   `{"XMLCustomerID":"Acme Fabrika"}`,
		"APICustomerName": `{"APICustomerName":"Acme Fabrika"}`,
		"HTTPAdres":       `{"HTTPAdres":"Gerçek Adres 123"}`, //nolint:misspell // OSOS field spelling, not a typo
		"IDMusteri":       `{"IDMusteri":"Acme Fabrika"}`,
	}
	for name, body := range mustReject {
		require.NotEmpty(t, fake.Violations("probe.json", []byte(body)), name)
	}

	mustPass := map[string]string{
		"XMLVersion": `{"XMLVersion":"1.0"}`,
		"HTTPStatus": `{"HTTPStatus":"200"}`,
		"IDNo":       `{"IDNo":"FX0000001"}`,
		"URLPath":    `{"URLPath":"/x"}`,
	}
	for name, body := range mustPass {
		require.Empty(t, fake.Violations("ok.json", []byte(body)), name)
	}
}

// TestSanitiserAllowsNonPIIProtocolKeys is R1's second check: no OTHER
// nameKeyPattern stem should reject a plausible non-PII protocol key from
// 06-integrations.md's field tables. The one real hit found was "tanim":
// OSOS's mapping table lists BOTH `sayimNokTanim` (a name-shaped,
// genuinely-PII field — still must-reject, see TestSanitiserRejects) and
// `tesisatTurTanim` (an installation/tariff-KIND classification field, in
// the same table row as tarifeTipi/tarifeTuru — never PII), and both keys
// end in "Tanim", so the generic "tanim" stem could not tell them apart.
// Resolved with an exact `^sayimnoktanim$` entry instead of the generic
// stem, so these classification fields now pass unflagged.
func TestSanitiserAllowsNonPIIProtocolKeys(t *testing.T) {
	require.Empty(t, fake.Violations("ok.json", []byte(
		`{"tesisatTurTanim":"Sanayi","tarifeTipi":"TT","tarifeTuru":"Tek Terimli"}`)))
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
