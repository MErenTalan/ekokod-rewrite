// Package worker is the single wiring point for the background worker
// process: it constructs every dependency from configuration and returns
// the job.Handlers internal/cli/worker.go registers on the asynq mux.
//
// Build wires the whole graph in one pass: the shared Redis lock, the
// httpx pool, the EPİAŞ client, internal/marketdata.Syncer
// (Handlers.Prices), the encryption cipher, every tenant repository, the
// meter-adapter registry (osos/gridbox/aril/pm5340), the iSolarCloud
// client, internal/credentials.Service, the PM5340 generation accumulator
// hook, internal/ingest.Service (Handlers.Ingestion),
// internal/ingest/backfill.Backfiller (Handlers.Backfill) and
// internal/service/consumption.Refresher (Handlers.ConsumptionRefresh —
// the globally-locked consumption.refresh handler, R100).
// ingestDeps.ConsumptionRefresh is wired to a small adapter over the same
// *job.Client (consumptionRefreshEnqueuer below) and threads
// cfg.ConsumptionRefreshEnabled / cfg.ConsumptionRefreshLockTTL through;
// the same adapter also backs consumption.RefreshDeps.Enqueuer, so lock
// contention re-enqueues through the identical path (R100(4)). Every
// resource Build opens before a later step fails is closed on that step's
// error path (see closers/closeAll below), and again, idempotently, by
// Built.Close on the success path — TestWorkerBuildClosesEarlierResourcesOnLateFailure
// proves this by forcing crypto.NewCipher to fail after redis and the job
// client are already open.
package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
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
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/pm5340"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	reportsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	platformredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
)

// mailDialTimeout matches apiwire's: an alarm notification must not hold a
// worker slot waiting on an unreachable SMTP host.
const mailDialTimeout = 15 * time.Second

// isolarStateKeyInfo domain-separates the isolar OAuth state signing key
// (StateKey) from the raw JWT signing key it is derived from, so the two
// can never be confused with or substituted for one another even though
// one is deterministically derived from the other.
const isolarStateKeyInfo = "isolar-oauth-state/v1"

// isolarCallbackPath is the fixed suffix RedirectURI appends to PublicURL
// when EKOKOD_ISOLAR_REDIRECT_URL is not set.
const isolarCallbackPath = "/api/v1/integrations/isolar/callback" // F6a: the callback route lives under /api/v1

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
	// Idempotent — guarded by sync.Once, not merely by the underlying
	// clients' own tolerance for a second Close call — so it is always
	// safe to call more than once (TestWorkerBuildClosesCleanlyTwice
	// calls it twice and requires no panic).
	Close func()
}

// namedCloser is one long-lived resource build() opened, tagged with a name
// so a test can assert exactly which resources a graph holds closers for,
// rather than only observing their combined effect via a side channel like
// a connection count.
type namedCloser struct {
	name  string
	close func()
}

// graph is the full dependency graph build() assembles before Build wraps
// it into Built's public two-field shape. Splitting construction out of
// Build into build() gives the same-package tests in this file access to
// pieces Built alone cannot prove: which key source feeds
// credentials.Deps.StateKey and the integration repository's cipher, which
// hook is registered for PM5340, how verifierResolver routes providers,
// and which resources are closed on every return path. Build's exported
// signature and Built are unchanged — this is purely an internal seam, no
// behaviour change.
type graph struct {
	cipher          *crypto.Cipher
	integrationRepo *postgres.IntegrationRepository
	verifiers       verifierResolver
	credentialsDeps credentials.Deps
	ingestDeps      ingest.Deps
	handlers        *job.Handlers
	closers         []namedCloser
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

// consumptionRefreshEnqueuer adapts *job.Client to BOTH
// ingest.ConsumptionRefreshEnqueuer and
// consumption.Enqueuer (R100(4)) — the two interfaces share the identical
// EnqueueConsumptionRefresh method shape by design, so this one adapter
// satisfies both with no glue: it is the enqueue seam a fetch run uses to
// start the FIRST consumption.refresh, and the seam RefreshConsumption
// itself uses to re-enqueue on lock contention. It builds a
// consumption.refresh task via job.NewConsumptionRefreshTask — using its
// OWN clock reading as "now" (R100(2): the enqueuing side's clock, not the
// handler's) — and enqueues it on the SAME job client every other
// integration task in this graph uses — no separate connection, no separate
// retry policy source.
//
// R100(2): asynq.ErrTaskIDConflict is mapped to nil here, never surfaced as
// an error. A conflict means another enqueue already reserved this exact
// one-minute debounce slot for this exact window — the designed burst-
// collapse case, not a failure — so the caller (internal/ingest's
// maybeEnqueueConsumptionRefresh) must see success and write no warning.
type consumptionRefreshEnqueuer struct {
	client   *job.Client
	maxRetry int
	clock    clock.Clock
}

func (e consumptionRefreshEnqueuer) EnqueueConsumptionRefresh(ctx context.Context, p job.ConsumptionRefreshPayload) error {
	task, err := job.NewConsumptionRefreshTask(p, e.clock.Now(), job.TaskOptions{MaxRetry: e.maxRetry})
	if err != nil {
		return err
	}
	_, err = e.client.Enqueue(ctx, task)
	if err != nil && errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// Build constructs every dependency from configuration: it is the single
// wiring point; internal/cli/worker.go calls it. It is a thin wrapper over
// build() (below),
// which does the actual assembly — Build's only job is to turn a graph
// into Built's public two-field shape and guard Close with sync.Once so it
// is safe to call more than once.
func Build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (Built, error) {
	g, err := build(ctx, cfg, pool, log)
	if err != nil {
		return Built{}, err
	}
	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			for i := len(g.closers) - 1; i >= 0; i-- {
				g.closers[i].close()
			}
		})
	}
	return Built{Handlers: g.handlers, Close: closeAll}, nil
}

// build assembles the full dependency graph. Every resource opened
// before a later construction step fails is closed on that step's own
// error path via closeAll, in reverse-open order — see the closers slice
// below. build is unexported: Build (above) is the only production caller;
// this package's own same-package tests call build directly to inspect
// pieces (credentials.Deps, ingest.Deps, the cipher, verifierResolver, the
// named closers) that Built's public shape does not expose.
func build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (graph, error) {
	adminMarketDataRepo := admin.NewMarketDataRepository(pool)
	adminJournalRepo := admin.NewJournalRepository(pool)
	adminAggregateRepo := admin.NewAggregateRepository(pool)

	// closers accumulates a namedCloser for every long-lived resource
	// build opens, in open order; closeAll runs them in reverse-open order
	// (the same order Built.Close's doc has always promised: job client,
	// then the lock's redis client). Every error path below calls
	// closeAll() before returning, so a late construction failure never
	// leaks an earlier resource — proven by
	// TestWorkerBuildClosesEarlierResourcesOnLateFailure.
	var closers []namedCloser
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i].close()
		}
	}

	redisClient, err := platformredis.New(ctx, cfg.Redis, log)
	if err != nil {
		return graph{}, fmt.Errorf("worker: connect redis: %w", err)
	}
	closers = append(closers, namedCloser{"redis", func() { _ = redisClient.Close() }})
	// R2: this value is passed directly wherever a Locker (or Consumer) is
	// wanted — httpx.Locker is a type alias for lock.Locker, so *lock.Redis
	// already satisfies it. There is no worker.httpxLocker adapter.
	redisLock := lock.NewRedis(redisClient)

	httpxPool, err := httpx.NewPool(httpx.PoolOptions{
		PinnedCerts: cfg.External.PinnedCerts,
		Locker:      redisLock,
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build httpx pool: %w", err)
	}

	epiasClient, err := epias.New(httpxPool, epias.Options{
		CASURL:   cfg.External.EPIASCASURL,
		BaseURL:  cfg.External.EPIASBaseURL,
		Username: cfg.External.EPIASUsername,
		Password: integration.NewSecret([]byte(cfg.External.EPIASPassword)),
		Locker:   redisLock,
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build epias client: %w", err)
	}

	jobClient, err := job.NewClient(cfg.Redis)
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build job client: %w", err)
	}
	closers = append(closers, namedCloser{"job-client", func() { _ = jobClient.Close() }})

	syncer := marketdata.New(epiasClient, adminMarketDataRepo, adminJournalRepo, clock.System(), log)

	// cipher is built from cfg.Security.EncryptionKey, never from
	// cfg.Security.JWTSigningKey — the integration repository below shares
	// this single instance, so a same-package test can seal through it and
	// open with a cipher independently built from EncryptionKey to pin the
	// key source (see wiring_internal_test.go).
	cipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build encryption cipher: %w", err)
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
		return graph{}, fmt.Errorf("worker: build meter-adapter registry: %w", err)
	}

	isolarClient := isolar.New(httpxPool, isolar.Options{})
	verifiers := verifierResolver{registry: registry, isolar: isolarClient}

	// StateKey is derived from cfg.Security.JWTSigningKey, never from
	// cfg.Security.EncryptionKey — credDeps is the exact literal passed to
	// credentials.New, so a same-package test can read StateKey back off
	// it directly (see wiring_internal_test.go).
	credDeps := credentials.Deps{
		Integrations: integrationRepo,
		Analyzers:    analyzerRepo,
		Buildings:    postgres.NewBuildingRepository(pool),
		Verifiers:    verifiers,
		ISolar:       isolarClient,
		Enqueuer:     jobClient,
		Locker:       redisLock,
		Nonces:       redisLock, // *lock.Redis implements lock.Consumer too, R3
		Clock:        clock.System(),
		StateKey:     StateKey(cfg.Security.JWTSigningKey),
		RedirectURI:  RedirectURI(cfg),
		MaxRetry:     cfg.Worker.MaxRetries,
	}
	credService, err := credentials.New(credDeps)
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build credential service: %w", err)
	}

	// consumptionRefreshEnq is the ONE consumptionRefreshEnqueuer instance
	// this graph builds (R100(4)): ingestDeps.ConsumptionRefresh below uses
	// it to enqueue the FIRST consumption.refresh from a fetch run, and
	// consumption.RefreshDeps.Enqueuer (built further down) uses the exact
	// same value to re-enqueue on lock contention — one clock, one MaxRetry
	// source, no risk of the two adapters drifting apart.
	consumptionRefreshEnq := consumptionRefreshEnqueuer{client: jobClient, maxRetry: cfg.Worker.MaxRetries, clock: clock.System()}

	// the PM5340 hook list is built once here as ingestDeps, the exact
	// literal passed to ingest.New — a same-package test asserts it holds
	// exactly one *generation.Accumulator (see wiring_internal_test.go).
	ingestDeps := ingest.Deps{
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
			// R52: the generation hook shares the SAME redisLock every other
			// Locker consumer in this graph does, and is bounded by the SAME
			// configured cfg.Ingest.FutureTolerance the ingest.Service below
			// is built with — never the package's own default.
			// wiring_internal_test.go's
			// TestBuildGraphGenerationHookUsesRedisLockAndConfiguredFutureTolerance
			// pins both.
			model.IntegrationProviderPM5340: {generation.New(readingRepo, generationRepo, clock.System(), redisLock, cfg.Ingest.FutureTolerance)},
		},
		// the enqueue seam is wired to the same jobClient every other
		// integration task in this graph enqueues through, gated by
		// cfg.ConsumptionRefreshEnabled below.
		ConsumptionRefresh: consumptionRefreshEnq,
		Clock:              clock.System(),
		Log:                log,
	}
	ingestSvc, err := ingest.New(ingestDeps, ingest.Options{
		FutureTolerance:           cfg.Ingest.FutureTolerance,
		SanityMultiple:            cfg.Ingest.SanityMultiple,
		InitialLookback:           cfg.Ingest.InitialLookback,
		MaxRetry:                  cfg.Worker.MaxRetries,
		ConsumptionRefreshEnabled: cfg.ConsumptionRefreshEnabled,
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build ingest service: %w", err)
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

	// R71/R72/R100: the refresher reuses the SAME redisLock every other
	// Locker consumer in this graph does — now under
	// R100(4)'s single global key rather than one key per view — and is
	// bounded by cfg.ConsumptionRefreshLockTTL (EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL,
	// default 30m, R100(5)). Enqueuer is the SAME consumptionRefreshEnq
	// ingestDeps.ConsumptionRefresh above uses, so a lock-contention
	// re-enqueue (R100(4)) goes through the identical adapter. Location is
	// Europe/Istanbul, loaded through the same
	// internal/integration/normalize.Istanbul every other timestamp
	// normalisation in this worker uses.
	consumptionRefresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: adminAggregateRepo,
		Locker:     redisLock,
		Enqueuer:   consumptionRefreshEnq,
		Clock:      clock.System(),
		LockTTL:    cfg.ConsumptionRefreshLockTTL,
		Location:   normalize.Istanbul,
		Log:        log,
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build consumption refresher: %w", err)
	}

	// F4 billing: invoice-grade consumption (full BillingDeps: the locker,
	// analyzers and users serve anomaly recording and resolution), the billing
	// service and its three job adapters.
	billingConsumption, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: readingRepo, Anomalies: anomalyRepo, Ops: opsRepo, Clock: clock.System(), Log: log,
		Locker: redisLock, Analyzers: analyzerRepo, Users: postgres.NewUserRepository(pool),
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build billing consumption: %w", err)
	}
	billRepo := postgres.NewBillRepository(pool)
	buildingRepo := postgres.NewBuildingRepository(pool)
	billingService, err := billingsvc.New(billingsvc.Deps{
		Consumption: billingConsumption, Buildings: buildingRepo, Analyzers: analyzerRepo,
		Tariffs: postgres.NewTariffRepository(pool), Params: postgres.NewBillingParameterRepository(pool),
		Prices: postgres.NewPriceRepository(pool), Anomalies: anomalyRepo, Bills: billRepo, Ops: opsRepo,
		Clock: clock.System(), Log: log,
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build billing service: %w", err)
	}

	// F7 alarms: the worker is the only process that fires rules — it
	// dispatches, evaluates and delivers. The API holds the same service
	// without RunDeps, so a dry run there can never send anything.
	alarmService, err := alarms.New(alarms.Deps{
		Alarms: postgres.NewAlarmRepository(pool), Analyzers: analyzerRepo, Clock: clock.System(),
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build alarm service: %w", err)
	}
	alarmService = alarmService.
		WithEvaluate(alarms.EvaluateDeps{Readings: readingRepo, Bills: billRepo}).
		WithRun(alarms.RunDeps{Ops: opsRepo, Enqueue: alarms.EnqueueNotify(jobClient, cfg.Worker.MaxRetries)}).
		WithNotify(alarms.NotifyDeps{SMTP: postgres.NewSMTPRepository(pool, cipher),
			Mail: mail.NewSMTPSender(mailDialTimeout, nil), Ops: opsRepo}).
		WithDispatch(alarms.DispatchDeps{
			Companies: admin.NewAlarmRepository(pool), Journal: admin.NewJournalRepository(pool),
			Enqueue: alarms.EnqueueEvaluate(jobClient, cfg.Worker.MaxRetries),
		})
	alarmJobs := alarms.JobAdapter{Service: alarmService, Clock: clock.System()}

	// F8b reports: the worker builds, stores and e-mails them (R264–R268).
	reportAnalytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: postgres.NewAnalyticsRepository(pool), Log: log})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build report analytics: %w", err)
	}
	reportService, err := reportsvc.New(reportsvc.Deps{
		Buildings: buildingRepo, Analyzers: analyzerRepo, Bills: billRepo, Plants: postgres.NewPlantRepository(pool),
		Solar: postgres.NewSolarTariffRepository(pool), Tariffs: postgres.NewTariffRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Analytics: postgres.NewAnalyticsRepository(pool),
		Consumption: reportAnalytics, Clock: clock.System(),
	})
	if err != nil {
		closeAll()
		return graph{}, fmt.Errorf("worker: build report service: %w", err)
	}
	reportRepo := postgres.NewReportRepository(pool)
	companyRepo := postgres.NewCompanyRepository(pool)
	reportFiles := reportsvc.Files{Root: cfg.Storage.Root, Companies: companyRepo, Buildings: buildingRepo}

	// F9 iSolar: sync, faults and forwarding (R276-R288). The worker is the
	// only process that calls iSolar on a schedule.
	solarService := solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(pool), Production: postgres.NewProductionRepository(pool),
		Totals: postgres.NewProductionTotalsRepository(pool), Faults: postgres.NewFaultRepository(pool), Ops: opsRepo,
		AdminSolar: admin.NewSolarRepository(pool), SMTP: postgres.NewSMTPRepository(pool, cipher),
		Mail: mail.NewSMTPSender(mailDialTimeout, nil), Creds: credService, ISolar: isolarClient,
		Clock: clock.System(), Enqueuer: jobClient, MaxRetry: cfg.Worker.MaxRetries,
	})

	return graph{
		cipher:          cipher,
		integrationRepo: integrationRepo,
		verifiers:       verifiers,
		credentialsDeps: credDeps,
		ingestDeps:      ingestDeps,
		handlers: &job.Handlers{
			Log:                log,
			Ingestion:          ingestSvc,
			AnalyzerRefresh:    ingestSvc,
			Demo:               seed.DemoJob{Pool: pool, Clock: clock.System()},
			Backfill:           backfiller,
			Prices:             syncer,
			ConsumptionRefresh: consumptionRefresher,
			BillingDispatch: billingsvc.Dispatcher{Billable: admin.NewBillingRepository(pool), Bills: billRepo, Enqueuer: jobClient,
				Clock: clock.System(), MaxRetry: cfg.Worker.MaxRetries},
			BillingGenerate: billingsvc.JobGenerator{Service: billingService, Ops: opsRepo, Enqueuer: jobClient,
				Clock: clock.System(), MaxRetry: cfg.Worker.MaxRetries},
			BillingRender: billingsvc.PDFRenderer{Bills: billRepo, Companies: postgres.NewCompanyRepository(pool),
				Buildings: buildingRepo, Analyzers: analyzerRepo, Root: cfg.Storage.Root},
			AlarmDispatch: alarmJobs,
			AlarmEvaluate: alarmJobs,
			AlarmNotify:   alarmJobs,
			ReportDispatch: reportsvc.Dispatcher{Billable: admin.NewBillingRepository(pool), Enqueuer: jobClient,
				Clock: clock.System(), MaxRetry: cfg.Worker.MaxRetries},
			ReportGenerate: reportsvc.Generator{Service: reportService, Reports: reportRepo, Companies: companyRepo,
				Ops: opsRepo, Files: reportFiles, Clock: clock.System()},
			ReportDeliver: reportsvc.Deliverer{Reports: reportRepo, Files: reportFiles, SMTP: postgres.NewSMTPRepository(pool, cipher),
				Mail: mail.NewSMTPSender(mailDialTimeout, nil), Ops: opsRepo, Clock: clock.System()},
			Solar: solarService,
		},
		closers: closers,
	}, nil
}
