package fake

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Rule patterns are copied verbatim from the F2 Task 4 brief's sanitisation
// table. See that brief for the provenance and rationale of each one.
var (
	fxFormPattern = regexp.MustCompile(`^FX[0-9_]+$`)

	jwtPattern   = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.`)
	tcknPattern  = regexp.MustCompile(`\b[1-9]\d{10}\b`)
	ibanPattern  = regexp.MustCompile(`TR\d{24}`)
	phonePattern = regexp.MustCompile(`\+?90\s?\d{3}`)
	urlPattern   = regexp.MustCompile(`https?://[^\s"']+`)
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

	// secretKeyPattern matches a key whose value must be a FIXTURE- or
	// TGT-FIXTURE- placeholder: credentials, tokens, passwords, TGTs and
	// codes.
	secretKeyPattern = regexp.MustCompile(`(?i)pass|secret|token|key|tgt|code|auth`)

	// nameKeyPattern matches a key whose value must start "Fixture ":
	// customer/company/plant/title names and address/street/district/
	// neighbourhood fields.
	nameKeyPattern = regexp.MustCompile(`(?i)name|title|address|street|district|neighbourhood`)

	// installationKeys are the exact installation/wiring/subscription/meter
	// identifier keys the brief enumerates, matched case-insensitively.
	// Their values must be in FX placeholder form.
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
	}

	// allowedHosts are the only hostnames a fixture's URL may reference.
	allowedHosts = map[string]bool{
		"127.0.0.1":       true,
		"example.invalid": true,
	}
)

// Violations reports every sanitisation rule broken by the fixture at path
// with the given JSON body. See the F2 Task 4 brief for the full rule set;
// path is used only to label a JSON-parse failure.
func Violations(path string, body []byte) []string {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return []string{fmt.Sprintf("%s: invalid JSON: %v", path, err)}
	}

	var out []string
	walkSanitise(doc, "", &out)
	return out
}

func walkSanitise(node any, key string, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			walkSanitise(val, k, out)
		}
	case []any:
		for _, item := range v {
			walkSanitise(item, key, out)
		}
	case string:
		*out = append(*out, checkStringValue(key, v)...)
	}
}

// checkStringValue applies the key-conditioned placeholder rule for key
// (installation identifier, credential/token, or name/address — first match
// wins, in that priority order, since a key like "ps_key" matches both the
// installation list and the generic secret-key pattern) and then the
// key-independent shape rules (JWT, TCKN, IBAN, phone, e-mail, URL host)
// that apply to every string value regardless of its key.
func checkStringValue(key, value string) []string {
	var violations []string
	lowerKey := strings.ToLower(key)

	switch {
	case installationKeys[lowerKey]:
		if !fxFormPattern.MatchString(value) {
			violations = append(violations, fmt.Sprintf("key %q: value %q is not in FX placeholder form", key, value))
		}
	case secretKeyPattern.MatchString(key):
		if !strings.HasPrefix(value, "FIXTURE-") && !strings.HasPrefix(value, "TGT-FIXTURE-") {
			violations = append(violations, fmt.Sprintf("key %q: value %q is not in FIXTURE- placeholder form", key, value))
		}
	case nameKeyPattern.MatchString(key):
		if !strings.HasPrefix(value, "Fixture ") {
			violations = append(violations, fmt.Sprintf("key %q: value %q does not start with \"Fixture \"", key, value))
		}
	}

	if jwtPattern.MatchString(value) {
		violations = append(violations, fmt.Sprintf("key %q: value looks like a JWT", key))
	}
	if tcknPattern.MatchString(value) {
		violations = append(violations, fmt.Sprintf("key %q: value looks like a TCKN", key))
	}
	if ibanPattern.MatchString(value) {
		violations = append(violations, fmt.Sprintf("key %q: value looks like an IBAN", key))
	}
	if phonePattern.MatchString(value) {
		violations = append(violations, fmt.Sprintf("key %q: value looks like a phone number", key))
	}
	for _, email := range emailPattern.FindAllString(value, -1) {
		if !strings.HasSuffix(email, "@example.com") {
			violations = append(violations, fmt.Sprintf("key %q: e-mail %q is not at example.com", key, email))
		}
	}
	for _, raw := range urlPattern.FindAllString(value, -1) {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if host := u.Hostname(); !allowedHosts[host] {
			violations = append(violations, fmt.Sprintf("key %q: URL host %q is not allowed", key, host))
		}
	}

	return violations
}
