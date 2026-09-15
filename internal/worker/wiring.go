// Package worker is the single wiring point for the background worker
// process: it constructs every F2 dependency from configuration and
// returns the job.Handlers internal/cli/worker.go registers on the asynq
// mux, and Task 17's acceptance test exercises directly.
//
// PART-A / PART-B SCOPE (Task 16 controller ruling). This file is Task 16
// part A: everything the task-16 brief describes EXCEPT the credential
// service (internal/credentials, Task 14) and everything that can only be
// built once it exists. internal/credentials is still an empty placeholder
// package in this worktree (Task 14 merges separately, after this task),
// so ingest.Service and backfill.Backfiller — whose Deps.Credentials is a
// REQUIRED, non-nil ingest.CredentialOpener — cannot be constructed here
// without either (a) calling a credentials.New that does not exist yet, or
// (b) hand-rolling a competing, throwaway CredentialOpener implementation
// in this package purely to satisfy the compiler, which the controller's
// scope ruling explicitly forbids ("do not create placeholders or TODO
// stubs in production code").
//
// Build therefore wires everything that does NOT transitively depend on a
// CredentialOpener: the shared Redis lock, the httpx pool, the EPİAŞ
// client and internal/marketdata.Syncer (Handlers.Prices). It does not
// build the tenant repositories (Reading/Cursor/Anomaly/Analyzer/Plant/
// Production/Ops/ProviderSeries/Integration), the meter-adapter registry,
// the PM5340 generation accumulator, the iSolarCloud client or the
// encryption cipher — every one of those exists ONLY to feed
// ingest.Service, backfill.Backfiller or credentials.Service, so building
// them here now would leave real, then-immediately-dead code for Part B to
// delete, which is worse than Part B adding fresh lines. The task-16
// report's "part B checklist" lists every line Part B adds, verbatim,
// against the variable names this file already uses, so the change stays
// small and mechanical.
package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/epias"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	platformredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
)

// isolarStateKeyInfo domain-separates the isolar OAuth state signing key
// (StateKey) from the raw JWT signing key it is derived from, so the two
// can never be confused with or substituted for one another even though
// one is deterministically derived from the other.
const isolarStateKeyInfo = "isolar-oauth-state/v1"

// isolarCallbackPath is the fixed suffix RedirectURI appends to PublicURL
// when EKOKOD_ISOLAR_REDIRECT_URL is not set.
const isolarCallbackPath = "/integrations/isolar/callback"

// StateKey derives the isolar OAuth state HMAC key from the platform's JWT
// signing key (HMAC-SHA256, domain-separated by isolarStateKeyInfo) —
// stable across calls for the same input, and never equal to the raw
// signing key it is derived from. Part B passes this into
// credentials.Deps.StateKey.
func StateKey(jwtSigningKey []byte) []byte {
	mac := hmac.New(sha256.New, jwtSigningKey)
	mac.Write([]byte(isolarStateKeyInfo))
	return mac.Sum(nil)
}

// RedirectURI resolves the isolar OAuth callback URL: an explicit override
// (EKOKOD_ISOLAR_REDIRECT_URL / cfg.External.ISolarRedirect) if set, else
// cfg.HTTP.PublicURL with any trailing slash trimmed, plus
// isolarCallbackPath. Part B passes this into credentials.Deps.RedirectURI.
func RedirectURI(cfg *config.Config) string {
	if cfg.External.ISolarRedirect != "" {
		return cfg.External.ISolarRedirect
	}
	return strings.TrimSuffix(cfg.HTTP.PublicURL, "/") + isolarCallbackPath
}

// Built is everything Build assembles.
type Built struct {
	Handlers *job.Handlers
	// Close releases every long-lived resource Build opened: the job
	// client and the lock's redis client (R2: the same *lock.Redis is
	// also the httpx Pool's and the EPİAŞ client's Locker, so there is
	// only ever one redis client to close here, not one per consumer).
	Close func()
}

// Build constructs every F2 dependency Part A owns from configuration. It
// is the single wiring point; internal/cli/worker.go calls it and Task
// 17's acceptance test calls it. See the package doc for exactly what Part
// B still adds.
func Build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (Built, error) {
	adminMarketDataRepo := admin.NewMarketDataRepository(pool)
	adminJournalRepo := admin.NewJournalRepository(pool)

	redisClient, err := platformredis.New(ctx, cfg.Redis, log)
	if err != nil {
		return Built{}, fmt.Errorf("worker: connect redis: %w", err)
	}
	// R2: this value is passed directly wherever a Locker (or Consumer) is
	// wanted — httpx.Locker is a type alias for lock.Locker (Task 2), so
	// *lock.Redis already satisfies it. There is no worker.httpxLocker
	// adapter anywhere in F2.
	redisLock := lock.NewRedis(redisClient)

	httpxPool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: cfg.External.PinnedCerts,
		Locker:      redisLock,
	})
	if err != nil {
		_ = redisClient.Close()
		return Built{}, fmt.Errorf("worker: build httpx pool: %w", err)
	}

	epiasClient, err := epias.New(httpxPool, epias.Options{
		Username: cfg.External.EPIASUsername,
		Password: integration.NewSecret([]byte(cfg.External.EPIASPassword)),
		Locker:   redisLock,
	})
	if err != nil {
		_ = redisClient.Close()
		return Built{}, fmt.Errorf("worker: build epias client: %w", err)
	}

	jobClient, err := job.NewClient(cfg.Redis)
	if err != nil {
		_ = redisClient.Close()
		return Built{}, fmt.Errorf("worker: build job client: %w", err)
	}

	syncer := marketdata.New(epiasClient, adminMarketDataRepo, adminJournalRepo, clock.System(), log)

	closeFn := func() {
		_ = jobClient.Close()
		_ = redisClient.Close()
	}

	return Built{
		Handlers: &job.Handlers{Log: log, Prices: syncer},
		Close:    closeFn,
	}, nil
}
