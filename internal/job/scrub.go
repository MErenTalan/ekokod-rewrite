package job

import "fmt"

// scrubParseErr reports a redis URL parse failure without ever including the
// raw input: goredis.ParseURL forwards net/url's parse error unchanged, and
// url.Error.Error() embeds its whole argument — including any password —
// verbatim. We cannot reason about what a URL we cannot even parse might
// contain, so the underlying error text is dropped entirely rather than
// risk leaking a credential. See internal/store/redis/scrub.go and
// internal/store/postgres/scrub.go for the same defect class elsewhere.
func scrubParseErr(op string) error {
	return fmt.Errorf("%s: redis url is not valid (details withheld)", op)
}
