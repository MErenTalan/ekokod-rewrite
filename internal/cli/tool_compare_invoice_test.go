package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
)

const realFixture = "../../testdata/real-invoices/icmal_row_ck_anonymised.json"

func TestCompareInvoiceMatchesAnonymisedIcmalRowExactly(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, cli.Execute(context.Background(), []string{"tool", "compare-invoice", "--fixture", realFixture}, &out))
	require.Contains(t, out.String(), "820028.45")
	require.NotContains(t, out.String(), "UNEXPLAINED")
}

func TestCompareInvoiceExitsNonZeroOnUnexplainedDiff(t *testing.T) {
	raw, err := os.ReadFile(realFixture)
	require.NoError(t, err)
	var f map[string]any
	require.NoError(t, json.Unmarshal(raw, &f))
	lines := f["expected"].(map[string]any)["lines"].([]any)
	lines[0].(map[string]any)["amount"] = "552557.30"
	changed, err := json.Marshal(f)
	require.NoError(t, err)

	var out bytes.Buffer
	err = cli.CompareInvoice(changed, &out)
	require.ErrorIs(t, err, cli.ErrUnexplainedDifference)
	require.Contains(t, out.String(), "UNEXPLAINED")

	f["explanations"] = map[string]any{"energy": "supplier rounded the unit price"}
	explained, err := json.Marshal(f)
	require.NoError(t, err)
	require.NoError(t, cli.CompareInvoice(explained, &bytes.Buffer{}))
}
