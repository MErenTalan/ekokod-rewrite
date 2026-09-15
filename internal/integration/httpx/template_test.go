package httpx_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
)

// TestExpandSubstitutesPathAndQuery is the brief's worked example verbatim:
// a path placeholder is url.PathEscape'd, a query placeholder is
// url.QueryEscape'd — different escaping rules for the same character
// class ('/' -> %2F either way, but a space is %20 in the path and '+' in
// the query).
func TestExpandSubstitutesPathAndQuery(t *testing.T) {
	u, err := httpx.Expand("https://h/{wiringNo}/x?start={startDate}", map[string]httpx.Param{
		"wiringNo":  {Value: "A/B"},
		"startDate": {Value: "01/09/2026 00:00:00"},
	})
	require.NoError(t, err)
	require.Contains(t, u.EscapedPath(), "A%2FB")
	require.Equal(t, "start=01%2F09%2F2026+00%3A00%3A00", u.RawQuery)
}

func TestExpandRefusesMissingUnusedAndLeftoverPlaceholders(t *testing.T) {
	t.Run("missing param", func(t *testing.T) {
		_, err := httpx.Expand("https://h/{wiringNo}", map[string]httpx.Param{})
		require.Error(t, err)
	})

	t.Run("unused param", func(t *testing.T) {
		_, err := httpx.Expand("https://h/x", map[string]httpx.Param{
			"wiringNo": {Value: "A"},
		})
		require.Error(t, err)
	})

	t.Run("leftover placeholder syntax", func(t *testing.T) {
		// "{unclosed" has no matching '}', so the placeholder regex never
		// matches it and it survives substitution verbatim.
		_, err := httpx.Expand("https://h/{unclosed", map[string]httpx.Param{})
		require.Error(t, err)
	})

	t.Run("nested braces leave the outer pair unresolved", func(t *testing.T) {
		_, err := httpx.Expand("https://h/{{inner}}", map[string]httpx.Param{
			"inner": {Value: "x"},
		})
		require.Error(t, err)
	})

	t.Run("error never contains the param value", func(t *testing.T) {
		_, err := httpx.Expand("https://h/x", map[string]httpx.Param{
			"secretParam": {Value: "FIXTURE-SECRET-VALUE", Secret: true},
		})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "FIXTURE-SECRET-VALUE")
	})
}
