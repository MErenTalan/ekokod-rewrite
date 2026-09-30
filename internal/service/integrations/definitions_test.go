package integrations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
)

// The live GridBox definition used the legacy form's last_index/indexes keys;
// the adapter reads last_endex/endexes, so every fetch failed. Save must refuse it.
func TestValidEndpointsRequiresProviderKeys(t *testing.T) {
	legacyForm := json.RawMessage(`{"token":"https://x/t","last_success_date":"https://x/l/{wiringNo}",
		"last_index":"https://x/e/{wiringNo}","indexes":"https://x/i","load_profiles":"https://x/p"}`)
	err := validEndpoints(model.IntegrationProviderGridbox, legacyForm)
	var pe *perr.Error
	require.ErrorAs(t, err, &pe)
	require.Equal(t, []string{"required"}, pe.Params["endpoints.last_endex"])
	require.Equal(t, []string{"required"}, pe.Params["endpoints.endexes"])

	ok := json.RawMessage(`{"token":"https://x/t","last_success_date":"https://x/l","last_endex":"https://x/e",
		"endexes":"https://x/i","load_profiles":"https://x/p"}`)
	require.NoError(t, validEndpoints(model.IntegrationProviderGridbox, ok))
	require.NoError(t, validEndpoints(model.IntegrationProviderPM5340, json.RawMessage(`{}`)))
}
