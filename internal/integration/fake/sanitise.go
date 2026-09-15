package fake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// Rule patterns are derived from the F2 Task 4 brief's sanitisation table
// (provenance/rationale there) plus the fix-round-1 findings
// (.superpowers/sdd/2026-09-15-f2-integration-layer/task-4-fix1-findings.md,
// ruling R31): the guard must be STRICTER than the brief's literal patterns
// on real leaks and must NOT flag non-secret protocol fields. Where the two
// disagree, R31 (and the findings below) win.
var (
	fxFormPattern = regexp.MustCompile(`^FX[0-9_]+$`)

	jwtPattern  = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`)
	tcknPattern = regexp.MustCompile(`\b[1-9]\d{10}\b`)
	ibanPattern = regexp.MustCompile(`TR\d{24}`)
	// phoneNormalisedPattern is matched against the WHOLE normalised value
	// (normaliseStripPattern below), never a substring (I4/I3): the brief's
	// original `\+?90\s?\d{3}` matched inside plain decimal strings like
	// "1890433.5" (the digits "90433" appear mid-number). Anchoring to the
	// full value and requiring the mobile "5xxxxxxxxx" shape after an
	// optional 0/90/+90 prefix fixes that false positive while still
	// catching every spaced/punctuated form I3 found evading the old shape
	// rules.
	phoneNormalisedPattern = regexp.MustCompile(`^(\+?90|0)?5\d{9}$`)
	emailPattern           = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	urlPattern             = regexp.MustCompile(`https?://[^\s"']+`)
	// bareHostnamePattern finds domain-shaped tokens even with no scheme
	// (I3: "gateway.isolarcloud.eu/openapi" has no "https://"). The final
	// label must be 2+ letters, so it never matches a dotted-decimal IP
	// like 127.0.0.1 or a plain decimal number like "1890433.5" (I4).
	bareHostnamePattern = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)
	// normaliseStripPattern removes the separators a real IBAN/phone is
	// commonly typed or copy-pasted with (I3): spaces, dashes, dots and
	// parentheses.
	normaliseStripPattern = regexp.MustCompile(`[\s\-.()]`)

	// secretKeyPattern matches only credential-shaped keys (I4/R31): a bare
	// "code" or "key" substring is deliberately EXCLUDED so protocol fields
	// like result_code/currencyCode do not false-positive. ps_key/device_sn
	// etc. are handled as exact installationKeys entries below instead, so
	// dropping the bare "key" substring here does not miss them.
	//
	// "auth" is checked separately (authKeyPattern/authorKeyPattern) because
	// Go's RE2 engine does not support the negative lookahead the brief's
	// literal `auth(?!or)` needs to match "auth"/"authToken" while not
	// matching "author"/"authorName"; isSecretKey below implements the same
	// exclusion without lookahead.
	secretKeyPattern = regexp.MustCompile(`(?i)token|secret|pass|tgt|api_?key|_key$|cookie|session`)
	authKeyPattern   = regexp.MustCompile(`(?i)auth`)
	authorKeyPattern = regexp.MustCompile(`(?i)author`)

	// piiWords is the PII key WORD set (fix-round-3 controller ruling): a
	// key is name/address-shaped PII if tokeniseKey(key) produces a word
	// that is a member of this set — never by substring match. Built from
	// 06-integrations.md's OSOS (customerAdress, il, ilce, koyMahallesi,
	// caddesiSokagi → province/district/neighbourhood/street; muhatapNo,
	// sayimNokTanim) and ARIL (Title, Address) field tables, plus the
	// round-1/round-2 Turkish-spelling stems (I2, R1).
	//
	// fix-round-2 R1 found that substring/suffix stems (`il$`) and later a
	// single exact-whole-key anchor (`^sayimnoktanim$`) both fail: a suffix
	// stem catches unrelated keys that happen to END in the stem
	// ("email"/"mail" end in "il"; the pre-fix-round-2 bug), while an exact
	// whole-key anchor stops matching the instant the key is spelled with
	// ANY extra separator or suffix — `SAYIM_NOK_TANIM` (underscores) and
	// `sayimNoktaTanimi` (the "-ta"/"-i" suffix forms) both lower to a
	// string that is not byte-identical to "sayimnoktanim", so the anchor
	// wrongly let them through (fix-round-3's regression). Word
	// tokenisation (tokeniseKey below) fixes both failure modes at once:
	// "email"/"mail" never produce an "il" WORD (there is no camelCase/
	// separator boundary before "il" in either), so they are not caught by
	// the "il" entry below, while `SAYIM_NOK_TANIM`/`sayimNoktaTanimi`/
	// `sayim-nokta-tanimi` all tokenise to a "sayim"(-ish) word plus a
	// "tanim"/"tanimi" word regardless of case/separator spelling, so
	// hasSayimTanimCombo (below) still catches every spelling. "tanim" is
	// NOT in this set on its own, so a word-for-word `tesisatTurTanim` (→
	// installation_kind, a tariff/installation-CATEGORY field in the same
	// OSOS table row as tarifeTipi/tarifeTuru, never PII) does not trip the
	// sayim/tanim combo and is not otherwise a member of piiWords.
	//
	// "name" is deliberately NOT a member: 06-integrations.md's only
	// Provider-field hit for it is the genuinely-PII `customerName`, and
	// that key is still caught via its "customer" word. A bare "name" word
	// alone (`deviceName`, `unitName` — neither is a Provider-field column
	// entry anywhere in the doc) must pass; see TestSanitiserWordTokenisedKeyMatching.
	piiWords = map[string]bool{
		"il":           true,
		"ilce":         true,
		"mahalle":      true,
		"koy":          true,
		"cadde":        true,
		"caddesi":      true,
		"sokak":        true,
		"sokagi":       true,
		"adres":        true, //nolint:misspell // OSOS's own (Turkish) key spelling, not an English typo
		"adress":       true, //nolint:misspell // OSOS's own (Turkish/misspelled) key spelling, not an English typo
		"address":      true,
		"company":      true,
		"customer":     true,
		"musteri":      true, // covers müşteri too: foldWord maps ü→u, ş→s
		"firma":        true,
		"unvan":        true,
		"neighborhood": true,
		"muhatap":      true,
		"title":        true,
		"street":       true,
		"district":     true,
	}

	// installationKeys are exact (case-insensitive) identifier keys whose
	// values must be in FX placeholder form: installation/wiring/
	// subscription numbers, meter serials, ps_id/ps_key/device_sn — plus
	// muhatapNo, which I2 carves out of piiWords above (its word "muhatap"
	// would otherwise classify it as a name) because its
	// canonical field is counterparty_no, a numeric identifier, not a name.
	installationKeys = map[string]bool{
		"instalationnumber":  true,
		"installationnumber": true,
		"wiringno":           true,
		"subscriptionserno":  true,
		"ownerserno":         true,
		"ps_id":              true,
		"ps_key":             true,
		"device_sn":          true,
		"meterserial":        true,
		"meternumber":        true,
		"muhatapno":          true,
	}

	// coordXKeyPattern / coordYKeyPattern identify OSOS's koordinatX/
	// koordinatY (mapped to latitude/longitude per 06-integrations.md
	// §2 "Flow" step 2) and their canonical English names, each
	// range-checked (I2) against the allowed fixture bounds.
	coordXKeyPattern = regexp.MustCompile(`(?i)^koordinatx$|^latitude$|^lat$`)
	coordYKeyPattern = regexp.MustCompile(`(?i)^koordinaty$|^longitude$|^lon$|^lng$`)

	// coordLatMin/Max and coordLonMin/Max are the allowed fixture bounds
	// (I2), as exact decimals — no float64 anywhere in this package (see
	// coordinateViolations).
	coordLatMin = decimal.RequireFromString("39.0")
	coordLatMax = decimal.RequireFromString("39.999999")
	coordLonMin = decimal.RequireFromString("32.0")
	coordLonMax = decimal.RequireFromString("32.999999")

	// allowedHosts are the only hostnames a fixture's URL (scheme-bearing
	// or bare) may reference.
	allowedHosts = map[string]bool{
		"127.0.0.1":       true,
		"example.invalid": true,
	}
)

// tokeniseKey splits key into lowercase, Turkish-folded WORDS (fix-round-3
// controller ruling, extended by fix-round-4's acronym rule) on:
//   - camelCase/PascalCase boundaries (a lowercase letter immediately
//     followed by an uppercase letter);
//   - acronym boundaries (fix-round-4): an uppercase letter that is
//     immediately followed by a lowercase letter, when the letter BEFORE
//     it is also uppercase — the standard rule for splitting a leading
//     acronym run off the word that follows it. Without this rule the
//     lower→upper rule above never fires inside an all-uppercase run, so
//     an acronym fuses with the next word instead of tokenising
//     separately: "XMLCustomerID" → ["xmlcustomer", "id"] (one fused word
//     that never matches the "customer" PII word) instead of the correct
//     ["xml", "customer", "id"]. With the rule, "XMLCustomer" splits into
//     "XML" + "Customer" (boundary before the "C": the "L" before it is
//     upper, the "C" is upper, the "u" after it is lower) and "IDNo"
//     splits into "ID" + "No" (same shape, boundary before the "N").
//     "XMLVersion", "HTTPStatus", "IDNo" itself and "URLPath" still pass
//     as non-PII, since none of "xml"/"version"/"http"/"status"/"id"/
//     "no"/"url"/"path" is a PII word;
//   - digit boundaries (a letter immediately followed by a digit, or a
//     digit immediately followed by a letter);
//   - any run of one or more non-alphanumeric separators (`_`, `-`, `.`,
//     whitespace, or anything else that is not a letter or digit).
//
// Boundary detection runs on the ORIGINAL runes, before folding: folding
// lower-cases Turkish letters (İ→i, ı→i, ...), and doing that first would
// erase the very upper/lower distinction camelCase and acronym splitting
// depend on. Each extracted word is folded (see foldWord) only after its
// boundaries are decided, so "İlçe" (one Titlecase word, no internal
// boundary) still becomes the single folded word "ilce", while
// "sayimNoktaTanimi" becomes three words ("sayim", "nokta", "tanimi").
func tokeniseKey(key string) []string {
	runes := []rune(key)
	var words []string
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			words = append(words, foldWord(string(runes[start:end])))
		}
		start = -1
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		prev := runes[i-1]
		boundary := unicode.IsDigit(r) != unicode.IsDigit(prev) ||
			(unicode.IsLower(prev) && unicode.IsUpper(r))
		if !boundary && unicode.IsUpper(prev) && unicode.IsUpper(r) &&
			i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			boundary = true
		}
		if boundary {
			flush(i)
			start = i
		}
	}
	flush(len(runes))
	return words
}

// foldWord lower-cases a single extracted word, mapping Turkish letters to
// their closest ASCII equivalent (ı/İ→i, ş/Ş→s, ç/Ç→c, ğ/Ğ→g, ö/Ö→o,
// ü/Ü→u) so a key spelled with or without Turkish diacritics tokenises to
// the same word (e.g. "müşteri" and "musteri" both fold to "musteri").
func foldWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ı', 'İ':
			b.WriteRune('i')
		case 'ş', 'Ş':
			b.WriteRune('s')
		case 'ç', 'Ç':
			b.WriteRune('c')
		case 'ğ', 'Ğ':
			b.WriteRune('g')
		case 'ö', 'Ö':
			b.WriteRune('o')
		case 'ü', 'Ü':
			b.WriteRune('u')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// hasSayimTanimCombo reports whether words (already tokenised/folded)
// contain the "sayim nokta tanim(i)" counting-point-description
// combination (fix-round-3 controller ruling): a "sayim"/"sayimnok" word
// together with a "tanim"/"tanimi" word — however many words separate
// them, and regardless of case/separator spelling — OR a single fused
// word that itself starts with "sayimnok" and contains "tanim" (the
// no-separator-at-all spelling, e.g. "sayimnoktanim" as one token). A
// bare "tanim" word with no accompanying "sayim" word is NOT PII on its
// own (see piiWords' doc comment: "tesisatTurTanim").
func hasSayimTanimCombo(words []string) bool {
	hasSayim, hasTanim := false, false
	for _, w := range words {
		if w == "sayim" || w == "sayimnok" {
			hasSayim = true
		}
		if w == "tanim" || w == "tanimi" {
			hasTanim = true
		}
		if strings.HasPrefix(w, "sayimnok") && strings.Contains(w, "tanim") {
			return true
		}
	}
	return hasSayim && hasTanim
}

// isNameLikePII reports whether key is name/company/address-shaped PII
// per the fix-round-3 word-tokenised ruling: true if tokeniseKey(key)
// yields any word in piiWords, or the sayim/tanim combination
// (hasSayimTanimCombo) — decided on whole words, never substrings, so a
// word ENDING in a PII stem (e.g. "il" inside "email"/"mail") never
// matches, and only a genuine PII WORD does.
func isNameLikePII(key string) bool {
	words := tokeniseKey(key)
	if hasSayimTanimCombo(words) {
		return true
	}
	for _, w := range words {
		if piiWords[w] {
			return true
		}
	}
	return false
}

// Violations reports every sanitisation rule broken by the fixture at path
// with the given body.
//
// JSON fixtures (.json) get the full key-conditioned walk (I1: object keys
// and json.Number leaves are checked, not just string leaves).
//
// Non-JSON fixtures (.xml/.csv/.txt — M5) get only the key-independent
// shape rules (JWT/TCKN/IBAN/phone/e-mail/host) run over the raw text.
// Key-conditioned rules (install/secret/name placeholders) are JSON-only in
// this round: a robust, cheap XML element/attribute-name extractor was
// judged not worth its own risk of false negatives/positives for this
// round. See the Task 4 report, "what M5 covers".
func Violations(path string, body []byte) []string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return jsonViolations(path, body)
	default:
		return shapeViolations("raw text", string(body))
	}
}

func jsonViolations(path string, body []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return []string{fmt.Sprintf("%s: invalid JSON: %v", path, err)}
	}

	var out []string
	walkSanitise(doc, "", &out)
	return out
}

// walkSanitise recurses through a decoded JSON document. I1: it checks
// object KEYS (not just values — a TCKN or e-mail can be smuggled in as a
// map key, e.g. {"12345678901":{"ali@x.com":"y"}}) and json.Number leaves
// (not just strings — {"tckn":12345678901} has no string leaf at all) in
// addition to ordinary string leaves.
func walkSanitise(node any, key string, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			*out = append(*out, shapeViolations(fmt.Sprintf("map key %q", k), k)...)
			walkSanitise(val, k, out)
		}
	case []any:
		for _, item := range v {
			walkSanitise(item, key, out)
		}
	case string:
		*out = append(*out, checkStringValue(key, v)...)
	case json.Number:
		*out = append(*out, checkStringValue(key, v.String())...)
	}
}

// checkStringValue applies the key-conditioned placeholder rule for key
// (installation identifier, credential/token, or name/address — first
// match wins, in that priority order, since a key like "muhatapNo" matches
// both the installation list and the name pattern's "muhatap" stem: I2
// requires the FX rule to win there) and then the coordinate-range rule and
// the key-independent shape rules that apply to every value regardless of
// its key.
func checkStringValue(key, value string) []string {
	var violations []string
	lowerKey := strings.ToLower(key)

	switch {
	case installationKeys[lowerKey]:
		if !fxFormPattern.MatchString(value) {
			violations = append(violations, fmt.Sprintf("key %q: value %q is not in FX placeholder form", key, value))
		}
	case isSecretKey(key):
		if !strings.HasPrefix(value, "FIXTURE-") && !strings.HasPrefix(value, "TGT-FIXTURE-") {
			violations = append(violations, fmt.Sprintf("key %q: value %q is not in FIXTURE- placeholder form", key, value))
		}
	case isNameLikePII(key):
		if !strings.HasPrefix(value, "Fixture ") {
			violations = append(violations, fmt.Sprintf("key %q: value %q does not start with \"Fixture \"", key, value))
		}
	}

	violations = append(violations, coordinateViolations(key, lowerKey, value)...)
	violations = append(violations, shapeViolations(fmt.Sprintf("key %q", key), value)...)
	return violations
}

// isSecretKey reports whether key is credential-shaped (I4/R31): the
// generic secretKeyPattern, or "auth" without "author" — the RE2-compatible
// equivalent of the brief's `auth(?!or)`.
func isSecretKey(key string) bool {
	if secretKeyPattern.MatchString(key) {
		return true
	}
	return authKeyPattern.MatchString(key) && !authorKeyPattern.MatchString(key)
}

// coordinateViolations range-checks OSOS's koordinatX/koordinatY (and their
// canonical latitude/longitude names) against the allowed fixture bounds
// (I2: "koordinatX/Y out of the allowed range ... reject out-of-range").
//
// fix-round-2 (controller, merge-with-Task-1 finding): uses
// github.com/shopspring/decimal, never strconv.ParseFloat/float64 — Task 1's
// merged arch guard (TestIntegrationTreesDoNotParseFloats / depguard's
// no-float-money rule) forbids float parsing anywhere under
// internal/integration/..., non-test files included, so this exact-decimal
// comparison replaces the prior float64 bounds check with the same accepted/
// rejected cases.
func coordinateViolations(key, lowerKey, value string) []string {
	var lo, hi decimal.Decimal
	switch {
	case coordXKeyPattern.MatchString(lowerKey):
		lo, hi = coordLatMin, coordLatMax
	case coordYKeyPattern.MatchString(lowerKey):
		lo, hi = coordLonMin, coordLonMax
	default:
		return nil
	}

	d, err := decimal.NewFromString(value)
	if err != nil {
		return []string{fmt.Sprintf("key %q: coordinate value %q is not numeric", key, value)}
	}
	if d.LessThan(lo) || d.GreaterThan(hi) {
		return []string{fmt.Sprintf("key %q: coordinate value %s is outside the allowed fixture range [%s, %s]", key, d.String(), lo.String(), hi.String())}
	}
	return nil
}

// shapeViolations applies the key-independent shape rules — JWT, TCKN,
// IBAN, phone, e-mail domain, URL/bare host — to text identified by label
// in messages. I4: a value already in FX placeholder form is exempt (it is
// by construction a fixture placeholder, not a leak) so it cannot trip a
// shape rule on its own digits/letters.
func shapeViolations(label, text string) []string {
	if fxFormPattern.MatchString(text) {
		return nil
	}

	var violations []string
	// I3: normalise a COPY before the IBAN/TCKN/phone checks so everyday
	// formatting (spaces, dashes, dots, parentheses) cannot evade them; the
	// JWT/e-mail/URL checks below intentionally use the original text,
	// since normalising would corrupt those shapes.
	normalised := normaliseStripPattern.ReplaceAllString(text, "")

	if jwtPattern.MatchString(text) {
		violations = append(violations, fmt.Sprintf("%s: value looks like a JWT", label))
	}
	if tcknPattern.MatchString(normalised) {
		violations = append(violations, fmt.Sprintf("%s: value looks like a TCKN", label))
	}
	if ibanPattern.MatchString(normalised) {
		violations = append(violations, fmt.Sprintf("%s: value looks like an IBAN", label))
	}
	if phoneNormalisedPattern.MatchString(normalised) {
		violations = append(violations, fmt.Sprintf("%s: value looks like a phone number", label))
	}
	for _, email := range emailPattern.FindAllString(text, -1) {
		if !strings.HasSuffix(strings.ToLower(email), "@example.com") {
			violations = append(violations, fmt.Sprintf("%s: e-mail %q is not at example.com", label, email))
		}
	}
	for _, raw := range urlPattern.FindAllString(text, -1) {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if host := strings.ToLower(u.Hostname()); !allowedHosts[host] {
			violations = append(violations, fmt.Sprintf("%s: URL host %q is not allowed", label, host))
		}
		if u.User != nil {
			violations = append(violations, fmt.Sprintf("%s: URL contains userinfo", label))
		}
	}
	// I3: bare hostnames (no "http(s)://" scheme) such as
	// "gateway.isolarcloud.eu/openapi". E-mail matches are stripped first
	// so a compliant "...@example.com" address's own domain is not
	// re-flagged here: the bare/URL host allow-list is 127.0.0.1 and
	// example.invalid — example.com is allowed only as an e-mail domain,
	// checked above.
	withoutEmails := emailPattern.ReplaceAllString(text, "")
	for _, host := range bareHostnamePattern.FindAllString(withoutEmails, -1) {
		if !allowedHosts[strings.ToLower(host)] {
			violations = append(violations, fmt.Sprintf("%s: value contains a disallowed host %q", label, host))
		}
	}

	return violations
}
