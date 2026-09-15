//go:build integration

package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// graphTestConfig is workerTestConfig's same-package twin (see
// wiring_integration_test.go, package worker_test): it loads a full, valid
// *config.Config via config.Load with a map lookup, never config.FromEnv.
// Duplicated rather than shared — the two test files live in different
// packages (worker vs worker_test) and each is independently buildable
// under -tags=integration, the same tradeoff connectedClients already
// documents for internal/scheduler.
func graphTestConfig(t *testing.T, dsn string, redisCfg config.Redis) *config.Config {
	t.Helper()
	env := map[string]string{
		"EKOKOD_PUBLIC_URL":                "https://ekokod.example.com",
		"EKOKOD_DB_URL":                    dsn,
		"EKOKOD_REDIS_URL":                 redisCfg.URL,
		"EKOKOD_REDIS_CACHE_DB":            fmt.Sprint(redisCfg.CacheDB),
		"EKOKOD_REDIS_QUEUE_DB":            fmt.Sprint(redisCfg.QueueDB),
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              t.TempDir(),
		"EKOKOD_EPIAS_USERNAME":            "epias-user",
		"EKOKOD_EPIAS_PASSWORD":            "epias-pass",
		"EKOKOD_ML_API_KEY":                "ml-api-key",
	}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	require.NoError(t, err)
	return cfg
}

// unusedButValidDSN is a syntactically valid, never-dialled DSN: build()
// takes pool directly (already opened by testfixtures.NewIsolatedDB), so
// cfg.DB.URL's only consumer is config.Load's url.Parse check.
const unusedButValidDSN = "postgres://user:pass@localhost:5432/unused"

// buildGraph is build() plus t.Cleanup of the returned graph's own
// closers — every test in this file calls it once and inspects the graph
// directly (the whole point of the build()/Build() split: graph exposes
// credentialsDeps, ingestDeps, cipher and integrationRepo, none of which
// Built's public two-field shape carries).
func buildGraph(t *testing.T, pool *pgxpool.Pool, redisCfg config.Redis) graph {
	t.Helper()
	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	g, err := build(context.Background(), cfg, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		for i := len(g.closers) - 1; i >= 0; i-- {
			g.closers[i].close()
		}
	})
	return g
}

// TestBuildGraphKeySourcesArePinned is the fix-round-1 C1 test: it proves
// credentials.Deps.StateKey is derived from cfg.Security.JWTSigningKey, not
// cfg.Security.EncryptionKey — a mistake that would have stayed green under
// the acceptance test alone, because both keys are 32+ bytes and StateKey
// never surfaces in any handler-registration assertion.
func TestBuildGraphKeySourcesArePinned(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	g := buildGraph(t, pool, redisCfg)

	require.Equal(t, StateKey(cfg.Security.JWTSigningKey), g.credentialsDeps.StateKey,
		"credentials.Deps.StateKey must be derived from the JWT signing key")
	require.NotEqual(t, StateKey(cfg.Security.EncryptionKey), g.credentialsDeps.StateKey,
		"credentials.Deps.StateKey must never be derived from the encryption key")
}

// TestBuildGraphIntegrationRepoCipherComesFromEncryptionKey is the
// fix-round-1 C2 test: it proves build()'s integration repository seals
// with a cipher built from cfg.Security.EncryptionKey, not (as the
// reviewer's proof (c2) reproduces below) cfg.Security.JWTSigningKey[:32]
// — a second, independently correct-looking 32-byte key that would also
// satisfy crypto.NewCipher without erroring, so nothing before this test
// would ever notice the swap.
//
// It seals a secret through g.integrationRepo (the graph's real, single
// cipher instance) and opens it with a SECOND repository built from a
// cipher independently constructed from cfg.Security.EncryptionKey: they
// must agree, because OpenSecret only succeeds against the exact key (and
// AAD) Seal used.
func TestBuildGraphIntegrationRepoCipherComesFromEncryptionKey(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 9101)
	var definitionID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ($1, $2) returning id`,
		"osos", "default").Scan(&definitionID))

	const plainSecret = "graph-cipher-round-trip-secret"
	created, err := g.integrationRepo.UpsertCredential(ctx, tenant.Scope, model.IntegrationCredential{
		CompanyID: tenant.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte(plainSecret), nil)
	require.NoError(t, err)

	cfg := graphTestConfig(t, unusedButValidDSN, redisCfg)
	independentCipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	require.NoError(t, err)

	secondRepo := postgres.NewIntegrationRepository(pool, independentCipher)
	secret, _, err := secondRepo.OpenSecret(ctx, tenant.Scope, created.ID)
	require.NoError(t, err, "a cipher built from cfg.Security.EncryptionKey must open what the graph's repo sealed")
	require.Equal(t, plainSecret, string(secret))
}

// TestBuildGraphRegistersOnePM5340GenerationHook is the fix-round-1 I1
// test: it proves ingest.Deps.Hooks[model.IntegrationProviderPM5340] holds
// exactly one *generation.Accumulator, not an empty hook list that would
// silently stop PM5340 readings from ever updating cumulative generation.
func TestBuildGraphRegistersOnePM5340GenerationHook(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	hooks := g.ingestDeps.Hooks[model.IntegrationProviderPM5340]
	require.Len(t, hooks, 1, "PM5340 must have exactly one post-persist hook")
	_, ok := hooks[0].(*generation.Accumulator)
	require.True(t, ok, "the PM5340 hook must be a *generation.Accumulator")
}

// TestBuildGraphOpensNamedClosersForRedisAndJobClient is the fix-round-1 I4
// test: it proves build() actually attaches a closer for the job client
// (not only the redis client the connected-clients metric can observe), by
// asserting the closers slice's names directly, in open order.
func TestBuildGraphOpensNamedClosersForRedisAndJobClient(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	redisCfg := testfixtures.RedisConfig(t)
	g := buildGraph(t, pool, redisCfg)

	names := make([]string, len(g.closers))
	for i, c := range g.closers {
		names[i] = c.name
	}
	require.Equal(t, []string{"redis", "job-client"}, names)
}
