package fake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

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

	// nameKeyPattern matches keys whose values must start "Fixture ":
	// customer/company/plant/title/address/street/district/neighbourhood
	// names. Built from 06-integrations.md's OSOS (customerAdress, il,
	// ilce, koyMahallesi, caddesiSokagi, sayimNokTanim) and ARIL
	// (Title, Address) field tables, plus the Turkish-spelling stems I2
	// found evading the original English-only pattern.
	//
	// fix-round-2 R1 controller ruling: the OSOS province key is matched as
	// a WHOLE key, `^il$` (case-insensitive), not the old unanchored `il$`
	// suffix — that suffix matched ANY key ending in "il", so a compliant
	// `{"email":"someone@example.com"}` fixture was rejected as if "email"
	// were the Turkish province field. `^il$` still catches `{"il":"Ankara"}`
	// and no longer touches "email"/"mail"/"contactEmail" (those still get
	// the independent e-mail-domain shape check via shapeViolations
	// regardless of which switch branch fires — see TestSanitiserRejects's
	// "email" case, which requires an @example.com address, not this rule).
	//
	// R1 also asked whether any OTHER stem has the same collision for a
	// plausible non-PII protocol key. Checked every 06-integrations.md field
	// table (OSOS, GridBox, ARIL, PM5340, iSolarCloud, EPİAŞ): the one real
	// hit is "tanim"/"tanım" — OSOS's mapping table lists BOTH
	// `sayimNokTanim` (→ counterparty_no/metering_point_name, a name-shaped
	// field, correctly PII) and `tesisatTurTanim` (→ installation_kind, a
	// tariff/installation-CATEGORY field in the same table row as
	// tarifeTipi/tarifeTuru, which nameKeyPattern never flags) — both keys
	// END in "Tanim", so anchoring the stem (the `^il$` fix) cannot tell
	// them apart: it would either flag both or neither. Resolved the same
	// way installationKeys resolves ps_id/ps_key/device_sn — an exact
	// (case-insensitive) whole-key entry, `^sayimnoktanim$`, for the one
	// field documented as PII-shaped, instead of a generic "tanim" stem, so
	// `tesisatTurTanim` (and tarifeTipi/tarifeTuru) pass through unflagged.
	// See TestSanitiserAllowsNonPIIProtocolKeys.
	//
	// The other hypothetical collision this ruling named — "name" vs.
	// "username"/"deviceName"/"unitName" — was checked against every field
	// table too: none of those exact spellings appear as a Provider-field
	// column entry anywhere in 06-integrations.md (ARIL's auth body uses
	// `UserCode`, not "username"; PM5340 uses `deviceId`/`deviceIp`, never
	// `deviceName`; no provider table has a `unitName` field), so "name" is
	// left unanchored — the only field table hit it has today is the
	// genuinely-PII `customerName` (OSOS). No anchor needed for "name" per
	// the field tables as they stand today; a later adapter task that
	// introduces a real non-PII "...name..." key should anchor it the same
	// way, with its own must-pass case.
	nameKeyPattern = regexp.MustCompile(`(?i)adres|adress|address|company|customer|musteri|müşteri|firma|unvan|mahalle|sokak|cadde|^il$|ilce|ilçe|neighbo|muhatap|^sayimnoktanim$|name|title|street|district`) //nolint:misspell // "adres" is the OSOS provider's own (Turkish) key spelling, not an English typo

	// installationKeys are exact (case-insensitive) identifier keys whose
	// values must be in FX placeholder form: installation/wiring/
	// subscription numbers, meter serials, ps_id/ps_key/device_sn — plus
	// muhatapNo, which I2 carves out of nameKeyPattern above (its stem
	// "muhatap" would otherwise classify it as a name) because its
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
	case nameKeyPattern.MatchString(key):
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
