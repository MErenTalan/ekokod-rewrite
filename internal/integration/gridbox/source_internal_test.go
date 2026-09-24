package gridbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// TestGridBoxDataClientRate is fix round 1 finding I2: provider-defaults.md's
// `gridbox` row is "Rate / burst" 2 / 2 — two requests per second, burst 2.
// httpx.ClientConfig.Every is "one request per Every" (client.go), so 2
// req/s is Every: 500ms, not Every: 1s (which is only 1 req/s no matter what
// Burst says). This asserts dataClientConfig's actual ClientConfig values
// directly — no HTTP server or timing observation needed — so it fails
// immediately and unambiguously if requestEvery regresses to 1s.
//
// Mutation proof (fix round 1): reverting requestEvery to time.Second makes
// this FAIL with "0: expected 500ms, got 1s" — recorded in
// task-7-report.md's "Fix round 1" section.
func TestGridBoxDataClientRate(t *testing.T) {
	creds := integration.Credentials{CompanyID: uuid.MustParse("00000000-0000-0000-0000-0000000000c0")}
	cfg := dataClientConfig(creds)

	require.Equal(t, 500*time.Millisecond, cfg.Every, "2 req/s is one request per 500ms, not per 1s")
	require.Equal(t, 2, cfg.Burst)
	require.Equal(t, 30*time.Second, cfg.RequestTimeout)
}

// TestGridBoxDataClientSerializeKey is fix round 1 finding I1:
// provider-defaults.md's `gridbox` row marks "Serialise per company: yes"
// for the whole provider, not only the token exchange. dataClientConfig
// must carry a non-empty SerializeKey scoped to the company, distinct from
// tokenClient's own "gridbox:token:<company>" key (see dataClientConfig's
// doc in source.go for why the two are kept separate).
//
// Mutation proof (fix round 1): deleting dataClientConfig's SerializeKey
// line makes this FAIL with an empty string where "gridbox:<company>" was
// expected — recorded in task-7-report.md's "Fix round 1" section.
func TestGridBoxDataClientSerializeKey(t *testing.T) {
	companyID := uuid.MustParse("00000000-0000-0000-0000-0000000000c0")
	creds := integration.Credentials{CompanyID: companyID}
	cfg := dataClientConfig(creds)

	require.Equal(t, "gridbox:"+companyID.String(), cfg.SerializeKey)
	require.NotEqual(t, "gridbox:token:"+companyID.String(), cfg.SerializeKey, "data and token serialise keys must stay distinct")
}
