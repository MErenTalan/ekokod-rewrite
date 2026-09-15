// Package worker is the single wiring point for the background worker
// process: it constructs every F2 dependency from configuration and
// returns the job.Handlers internal/cli/worker.go registers on the asynq
// mux, and Task 17's acceptance test exercises directly.
//
// Build wires the whole F2 graph in one pass: the shared Redis lock, the
// httpx pool, the EPİAŞ client, internal/marketdata.Syncer
// (Handlers.Prices), the encryption cipher, every tenant repository, the
// meter-adapter registry (osos/gridbox/aril/pm5340), the iSolarCloud
// client, internal/credentials.Service, the PM5340 generation accumulator
// hook, internal/ingest.Service (Handlers.Ingestion) and
// internal/ingest/backfill.Backfiller (Handlers.Backfill). Every resource
// Build opens before a later step fails is closed on that step's error
// path (see closers/closeAll below), and again, idempotently, by
// Built.Close on the success path — TestWorkerBuildClosesEarlierResourcesOnLateFailure
// proves this by forcing crypto.NewCipher to fail after redis and the job
// client are already open.
package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/backfill"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/aril"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/epias"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/gridbox"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/pm5340"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
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
// signing key it is derived from. Build passes this into
// credentials.Deps.StateKey.
func StateKey(jwtSigningKey []byte) []byte {
	mac := hmac.New(sha256.New, jwtSigningKey)
	mac.Write([]byte(isolarStateKeyInfo))
	return mac.Sum(nil)
}

// RedirectURI resolves the isolar OAuth callback URL: an explicit override
// (EKOKOD_ISOLAR_REDIRECT_URL / cfg.External.ISolarRedirect) if set, else
// cfg.HTTP.PublicURL with any trailing slash trimmed, plus
// isolarCallbackPath. Build passes this into credentials.Deps.RedirectURI.
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
	// Idempotent: safe to call more than once (see
	// TestWorkerBuildClosesCleanlyTwice).
	Close func()
}

// verifierResolver adapts *integration.Registry and the iSolarCloud client
// to credentials.VerifierResolver. Every meter-provider Verify comes from
// its registered integration.Adapter — integration.Adapter already embeds
// Verify(ctx, integration.Credentials) error (the exact shape of
// credentials.Verifier), so registry.Source's result is returned as-is,
// with no per-provider adapter type. isolar is never registered in the
// meter-adapter registry (it is not a MeterDataSource/Adapter — it feeds
// plant/production data, not meter readings — see internal/integration/isolar's
// package doc), so ProviderISolar is routed to the iSolarCloud client
// directly; *isolar.Client.Verify has the same Verifier shape too.
type verifierResolver struct {
	registry *integration.Registry
	isolar   *isolar.Client
}

func (v verifierResolver) Verifier(p integration.Provider) (credentials.Verifier, error) {
	if p == integration.ProviderISolar {
		return v.isolar, nil
	}
	return v.registry.Source(p)
}

// Build constructs every F2 dependency from configuration: it is the
// single wiring point; internal/cli/worker.go calls it and Task 17's
// acceptance test calls it. Every resource opened before a later
// construction step fails is closed on that step's own error path via
// closeAll, in reverse-open order — see the closers slice below.
func Build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (Built, error) {
	adminMarketDataRepo := admin.NewMarketDataRepository(pool)
	adminJournalRepo := admin.NewJournalRepository(pool)

	// closers accumulates a Close for every long-lived resource Build
	// opens, in open order; closeAll runs them in reverse-open order (the
	// same order Built.Close's doc has always promised: job client, then
	// the lock's redis client). Every error path below calls closeAll()
	// before returning, so a late construction failure never leaks an
	// earlier resource — proven by
	// TestWorkerBuildClosesEarlierResourcesOnLateFailure.
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	redisClient, err := platformredis.New(ctx, cfg.Redis, log)
	if err != nil {
		return Built{}, fmt.Errorf("worker: connect redis: %w", err)
	}
	closers = append(closers, func() { _ = redisClient.Close() })
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
		closeAll()
		return Built{}, fmt.Errorf("worker: build httpx pool: %w", err)
	}

	epiasClient, err := epias.New(httpxPool, epias.Options{
		Username: cfg.External.EPIASUsername,
		Password: integration.NewSecret([]byte(cfg.External.EPIASPassword)),
		Locker:   redisLock,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build epias client: %w", err)
	}

	jobClient, err := job.NewClient(cfg.Redis)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build job client: %w", err)
	}
	closers = append(closers, func() { _ = jobClient.Close() })

	syncer := marketdata.New(epiasClient, adminMarketDataRepo, adminJournalRepo, clock.System(), log)

	cipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build encryption cipher: %w", err)
	}

	readingRepo := postgres.NewReadingRepository(pool)
	cursorRepo := postgres.NewCursorRepository(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	generationRepo := postgres.NewGenerationRepository(pool)
	providerSeriesRepo := postgres.NewProviderSeriesRepository(pool)
	integrationRepo := postgres.NewIntegrationRepository(pool, cipher)
	adminIngestionRepo := admin.NewIngestionRepository(pool)

	registry, err := integration.NewRegistry(
		osos.New(httpxPool, osos.Options{}),
		gridbox.New(httpxPool, gridbox.Options{}),
		aril.New(httpxPool, aril.Options{}),
		pm5340.New(httpxPool, pm5340.Options{}),
	)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build meter-adapter registry: %w", err)
	}

	isolarClient := isolar.New(httpxPool, isolar.Options{})

	credService, err := credentials.New(credentials.Deps{
		Integrations: integrationRepo,
		Analyzers:    analyzerRepo,
		Verifiers:    verifierResolver{registry: registry, isolar: isolarClient},
		ISolar:       isolarClient,
		Enqueuer:     jobClient,
		Locker:       redisLock,
		Nonces:       redisLock, // *lock.Redis implements lock.Consumer too, R3
		Clock:        clock.System(),
		StateKey:     StateKey(cfg.Security.JWTSigningKey),
		RedirectURI:  RedirectURI(cfg),
		MaxRetry:     cfg.Worker.MaxRetries,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build credential service: %w", err)
	}

	ingestSvc, err := ingest.New(ingest.Deps{
		Analyzers:      analyzerRepo,
		Readings:       readingRepo,
		Cursors:        cursorRepo,
		Anomalies:      anomalyRepo,
		Ops:            opsRepo,
		ProviderSeries: providerSeriesRepo,
		AdminIngestion: adminIngestionRepo,
		AdminJournal:   adminJournalRepo,
		Credentials:    credService,
		Sources:        registry,
		Enqueuer:       jobClient,
		Hooks: map[model.IntegrationProvider][]ingest.PostPersistHook{
			model.IntegrationProviderPM5340: {generation.New(readingRepo, generationRepo, clock.System())},
		},
		ConsumptionRefresh: nil, // R17: consumption.refresh is declared, never enqueued, in F2
		Clock:              clock.System(),
		Log:                log,
	}, ingest.Options{
		FutureTolerance: cfg.Ingest.FutureTolerance,
		SanityMultiple:  cfg.Ingest.SanityMultiple,
		InitialLookback: cfg.Ingest.InitialLookback,
		MaxRetry:        cfg.Worker.MaxRetries,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("worker: build ingest service: %w", err)
	}

	backfiller := backfill.New(backfill.Deps{
		Analyzers:   analyzerRepo,
		Ops:         opsRepo,
		Credentials: credService,
		Sources:     registry,
		Enqueuer:    jobClient,
		Clock:       clock.System(),
		MaxRetry:    cfg.Worker.MaxRetries,
	})

	return Built{
		Handlers: &job.Handlers{Log: log, Ingestion: ingestSvc, Backfill: backfiller, Prices: syncer},
		Close:    closeAll,
	}, nil
}
