package fake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	// Known trade-off: the "il$" stem (the OSOS `il` field) also matches
	// any key ending "il" such as "mail"/"email" — accepted as directed by
	// I2's literal stem list; a value under such a key still gets the
	// independent e-mail shape check via shapeViolations regardless of
	// which branch fires.
	nameKeyPattern = regexp.MustCompile(`(?i)adres|adress|address|company|customer|musteri|müşteri|firma|unvan|mahalle|sokak|cadde|il$|ilce|ilçe|neighbo|muhatap|tanim|tanım|name|title|street|district`) //nolint:misspell // "adres" is the OSOS provider's own (Turkish) key spelling, not an English typo

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
func coordinateViolations(key, lowerKey, value string) []string {
	var lo, hi float64
	switch {
	case coordXKeyPattern.MatchString(lowerKey):
		lo, hi = 39.0, 39.999999
	case coordYKeyPattern.MatchString(lowerKey):
		lo, hi = 32.0, 32.999999
	default:
		return nil
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return []string{fmt.Sprintf("key %q: coordinate value %q is not numeric", key, value)}
	}
	if f < lo || f > hi {
		return []string{fmt.Sprintf("key %q: coordinate value %v is outside the allowed fixture range [%v, %v]", key, f, lo, hi)}
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
