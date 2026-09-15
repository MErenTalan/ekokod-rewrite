package aril

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// TestARILDataClientSerializeKey is fix round 1 finding I1:
// provider-defaults.md's `aril` row marks "Serialise per company: yes" for
// the whole provider. dataClientConfig must carry a non-empty SerializeKey
// scoped to the company, distinct from authClient's own
// "aril:auth:<company>" key (see dataClientConfig's doc in source.go for why
// the two are kept separate).
//
// Mutation proof (fix round 1): deleting dataClientConfig's SerializeKey
// line makes this FAIL with an empty string where "aril:<company>" was
// expected; changing the key to a bare "aril" (dropping the company id)
// makes the require.Equal assertion FAIL because the recorded key no longer
// matches "aril:<company>" — both recorded in task-8-report.md's "Fix round
// 1" section.
func TestARILDataClientSerializeKey(t *testing.T) {
	companyID := uuid.MustParse("00000000-0000-0000-0000-0000000000c0")
	creds := integration.Credentials{CompanyID: companyID}
	cfg := dataClientConfig(creds)

	require.Equal(t, "aril:"+companyID.String(), cfg.SerializeKey)
	require.Equal(t, "aril:"+companyID.String(), cfg.LimiterKey, "LimiterKey and SerializeKey share the same per-company string")
	require.NotEqual(t, "aril:auth:"+companyID.String(), cfg.SerializeKey, "data and auth serialise keys must stay distinct")
}
