package httpx

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// placeholderPattern matches one balanced "{name}" placeholder. It
// deliberately excludes '{' and '}' from the captured name, so a malformed
// or nested form like "{{foo}}" leaves the outer braces unmatched — Expand
// treats any '{' or '}' surviving substitution as a leftover placeholder
// error (see below), rather than silently emitting it into the URL.
var placeholderPattern = regexp.MustCompile(`\{([^{}]*)\}`)

// Expand substitutes every "{name}" placeholder in template with the
// matching Params entry: a value is url.PathEscape'd if its placeholder
// occurs before the template's first '?' OR the template has no '?' at
// all, and url.QueryEscape'd only when the placeholder occurs at or after
// an actual '?' — never raw-concatenated. A template with no query string
// is entirely a path, so every one of its placeholders must use path
// escaping (fix for I2: a naive "qIdx >= 0 && start < qIdx" test is false
// for every placeholder whenever qIdx is -1, silently query-escaping path
// segments — e.g. turning "A B" into "A+B" instead of "A%20B" in a
// template like "/{a}/x" that never has a '?' at all). A placeholder
// naming a param not in params, a params entry never referenced by the
// template, or any '{'/'}' surviving substitution (an unmatched or
// malformed brace) is an error and Expand returns before any request would
// be built.
//
// Errors here never include a param's VALUE (only its NAME, a fixed
// endpoint-template identifier, never secret material) and never include
// the substituted URL itself.
func Expand(template string, params map[string]Param) (*url.URL, error) {
	qIdx := strings.IndexByte(template, '?')

	matches := placeholderPattern.FindAllStringSubmatchIndex(template, -1)
	used := make(map[string]bool, len(matches))

	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		nameStart, nameEnd := m[2], m[3]
		name := template[nameStart:nameEnd]

		p, ok := params[name]
		if !ok {
			return nil, fmt.Errorf("httpx: template references unknown param %q", name)
		}
		used[name] = true

		b.WriteString(template[last:start])
		if qIdx < 0 || start < qIdx {
			b.WriteString(url.PathEscape(p.Value))
		} else {
			b.WriteString(url.QueryEscape(p.Value))
		}
		last = end
	}
	b.WriteString(template[last:])
	expanded := b.String()

	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	for _, name := range names {
		if !used[name] {
			return nil, fmt.Errorf("httpx: unused param %q", name)
		}
	}

	if strings.ContainsAny(expanded, "{}") {
		return nil, fmt.Errorf("httpx: template has an unresolved placeholder")
	}

	u, err := url.Parse(expanded)
	if err != nil {
		return nil, fmt.Errorf("httpx: expanded template is not a valid URL")
	}

	// M8: Expand requires an absolute http/https URL with a host. A
	// relative or non-HTTP(S) template (e.g. a typo'd scheme, or a
	// template with no scheme at all) is a configuration error, caught
	// here rather than surfacing later as an obscure transport failure —
	// and, per this package's rule, without ever echoing the URL itself.
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("httpx: template must expand to an absolute http/https URL with a host")
	}

	return u, nil
}
