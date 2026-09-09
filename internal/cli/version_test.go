package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestVersionCommandPrintsJSON(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"version", "--json"}, &out)
	require.NoError(t, err)

	var got map[string]string
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Contains(t, got, "version")
	require.Contains(t, got, "commit")
	require.Contains(t, got, "date")
	require.NotEmpty(t, got["go"])
}

func TestVersionCommandPrintsText(t *testing.T) {
	var out bytes.Buffer
	err := cli.Execute(context.Background(), []string{"version"}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "ekokod")
}
