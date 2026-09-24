// Package apiwire is the API process's single wiring point (R181): it builds
// every repository, service and the /api/v1 router from configuration.
package apiwire

import (
	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/aril"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/gridbox"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/pm5340"
	isweather "github.com/MErenTalan/ekokod-rewrite/internal/integration/weather"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/analysis"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/calendar"
	carbonsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/financial"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/integrations"
	isosvc "github.com/MErenTalan/ekokod-rewrite/internal/service/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/jobs"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/ops"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/publicforms"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
	reportsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/tenancy"
	weathersvc "github.com/MErenTalan/ekokod-rewrite/internal/service/weather"
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	platformredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
	"github.com/MErenTalan/ekokod-rewrite/internal/worker"
)

// Options are test seams; production passes none.
type Options struct {
	Clock clock.Clock
	Mail  mail.Sender
	// RedisPrefix namespaces rate-limit and idempotency keys.
	RedisPrefix string
	// Async overrides how the auth service runs off-request work.
	Async func(func())
	// Enqueuer replaces the asynq client.
	Enqueuer Enqueuer
	// Verifiers and ISolar replace the provider clients the credential service drives.
	Verifiers credentials.VerifierResolver
	ISolar    credentials.ISolarTokens
	// Solar replaces the iSolar client behind the plant link routes (F9).
	Solar solar.Adapter
	// Weather replaces the weather provider (R291).
	Weather weathersvc.Provider
}

// Enqueuer is the job client the services enqueue through.
type Enqueuer = assets.Enqueuer

// Built is the API surface and its teardown.
type Built struct {
	V1    http.Handler
	Auth  *authsvc.Service
	Close func()
}

// Build wires the /api/v1 handler.
func Build(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger, opts Options) (Built, error) {
	if opts.Clock == nil {
		opts.Clock = clock.System()
	}
	if opts.Mail == nil {
		opts.Mail = mail.NewSMTPSender(15*time.Second, nil)
	}
	redisClient, err := platformredis.New(ctx, cfg.Redis, log)
	if err != nil {
		return Built{}, fmt.Errorf("apiwire: connect redis: %w", err)
	}
	closeAll := func() { _ = redisClient.Close() }

	// F12a Q-H6: config validated the uuid; an empty one leaves the forms unconfigured.
	formsCompany, _ := uuid.Parse(cfg.PublicForms.CompanyID)
	cipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build cipher: %w", err)
	}
	auditRepo := postgres.NewAuditRepository(pool)
	authService, err := authsvc.New(authsvc.Deps{
		Users: postgres.NewUserRepository(pool), Sessions: postgres.NewSessionRepository(pool),
		Resets: postgres.NewPasswordResetRepository(pool), Companies: postgres.NewCompanyRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), SMTP: postgres.NewSMTPRepository(pool, cipher),
		Ops: postgres.NewOpsRepository(pool), Audit: auditRepo, AdminAuth: admin.NewAuthRepository(pool),
		Mail:              opts.Mail,
		Hasher:            auth.Hasher{Pepper: cfg.Security.PasswordPepper, Cost: cfg.Security.BcryptCost},
		Tokens:            auth.Tokens{Key: cfg.Security.JWTSigningKey, TTL: cfg.Security.AccessTokenTTL, Now: opts.Clock.Now},
		FingerprintSecret: cfg.Security.DeviceFingerprintSecret,
		RefreshTTL:        cfg.Security.RefreshTokenTTL, RememberTTL: cfg.Security.RefreshTokenRememberTTL,
		HistorySize: cfg.Security.PasswordHistorySize, PublicURL: cfg.HTTP.PublicURL,
		Clock: opts.Clock, Log: log, Async: opts.Async,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build auth service: %w", err)
	}

	tenancyService, err := tenancy.New(tenancy.Deps{
		Companies: postgres.NewCompanyRepository(pool), AdminTenant: admin.NewTenantRepository(pool),
		Users: postgres.NewUserRepository(pool), Sessions: postgres.NewSessionRepository(pool),
		Analyzers: postgres.NewAnalyzerRepository(pool), SMTP: postgres.NewSMTPRepository(pool, cipher),
		Mail: opts.Mail, Hasher: auth.Hasher{Pepper: cfg.Security.PasswordPepper, Cost: cfg.Security.BcryptCost}, Clock: opts.Clock,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build tenancy service: %w", err)
	}

	jobClient, err := job.NewClient(cfg.Redis)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build job client: %w", err)
	}
	closeRedis := closeAll
	closeAll = func() { _ = jobClient.Close(); closeRedis() }
	enqueuer := Enqueuer(jobClient)
	if opts.Enqueuer != nil {
		enqueuer = opts.Enqueuer
	}
	// R192: the inspector reads the same queue the job client writes to.
	redisOpt, err := job.RedisOpt(cfg.Redis)
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: redis options for the job inspector: %w", err)
	}
	inspector := asynq.NewInspector(redisOpt)
	closeJobs := closeAll
	closeAll = func() { _ = inspector.Close(); closeJobs() }
	// A regeneration replaces a finished task holding its id; a test's own
	// enqueuer has no queue behind it, so it keeps the plain dedupe.
	var taskInspector job.TaskInspector = inspector
	if opts.Enqueuer != nil {
		taskInspector = nil
	}
	assetService, err := assets.New(assets.Deps{
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Plants: postgres.NewPlantRepository(pool), Tariffs: postgres.NewTariffRepository(pool),
		Integrations: postgres.NewIntegrationRepository(pool, cipher), Enqueuer: enqueuer,
		Sector: admin.NewSectorRepository(pool), Carbon: postgres.NewCarbonRepository(pool),
		MaxRetry: cfg.Worker.MaxRetries, Clock: opts.Clock,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build asset service: %w", err)
	}

	billingConsumption, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: postgres.NewReadingRepository(pool), Anomalies: postgres.NewAnomalyRepository(pool), Ops: postgres.NewOpsRepository(pool),
		Clock: opts.Clock, Log: log, Locker: lock.NewRedis(redisClient), Analyzers: postgres.NewAnalyzerRepository(pool),
		Users: postgres.NewUserRepository(pool),
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build billing consumption: %w", err)
	}
	analysisService, err := buildAnalysis(pool, log, opts.Clock, billingConsumption)
	if err != nil {
		closeAll()
		return Built{}, err
	}
	tariffService, err := tariffsvc.New(tariffsvc.Deps{
		Tariffs: postgres.NewTariffRepository(pool), Templates: postgres.NewTariffTemplateRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Icmal: postgres.NewIcmalRepository(pool), Prices: postgres.NewPriceRepository(pool),
		Params: postgres.NewBillingParameterRepository(pool), BulkAssignments: postgres.NewBulkAssignmentRepository(pool),
		SolarTariffs: postgres.NewSolarTariffRepository(pool), Plants: postgres.NewPlantRepository(pool),
		Catalogue: admin.NewCatalogueRepository(pool), Clock: opts.Clock, Log: log,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build tariff service: %w", err)
	}
	billRepo := postgres.NewBillRepository(pool)
	billingService, err := billingsvc.New(billingsvc.Deps{
		Consumption: billingConsumption, Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Tariffs: postgres.NewTariffRepository(pool), Params: postgres.NewBillingParameterRepository(pool),
		Prices: postgres.NewPriceRepository(pool), Anomalies: postgres.NewAnomalyRepository(pool), Bills: billRepo,
		Ops: postgres.NewOpsRepository(pool), Clock: opts.Clock, Log: log,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build billing service: %w", err)
	}
	billRequests := billingsvc.Requests{
		Service: billingService, Enqueuer: enqueuer, MaxRetry: cfg.Worker.MaxRetries, Location: istanbul, Tasks: taskInspector,
		Renderer: billingsvc.PDFRenderer{Bills: billRepo, Companies: postgres.NewCompanyRepository(pool),
			Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool), Root: cfg.Storage.Root},
	}

	// F8b reports: the API previews, enqueues and serves files; it never
	// sends mail (R266).
	reportAnalytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: postgres.NewAnalyticsRepository(pool), Log: log})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build report analytics: %w", err)
	}
	reportService, err := reportsvc.New(reportsvc.Deps{
		Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool), Bills: billRepo,
		Plants: postgres.NewPlantRepository(pool), Solar: postgres.NewSolarTariffRepository(pool),
		Tariffs: postgres.NewTariffRepository(pool), Carbon: postgres.NewCarbonRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Consumption: reportAnalytics, Clock: opts.Clock,
	})
	if err != nil {
		closeAll()
		return Built{}, fmt.Errorf("apiwire: build report service: %w", err)
	}
	reportRequests := reportsvc.Requests{Service: reportService, Reports: postgres.NewReportRepository(pool), Enqueuer: enqueuer, Tasks: taskInspector,
		Clock: opts.Clock, MaxRetry: cfg.Worker.MaxRetries,
		Files: reportsvc.Files{Root: cfg.Storage.Root, Companies: postgres.NewCompanyRepository(pool), Buildings: postgres.NewBuildingRepository(pool)}}

	calendarService, err := calendar.New(postgres.NewCalendarRepository(pool), opts.Clock)
	if err != nil {
		closeAll()
		return Built{}, err
	}
	credentialService, err := buildCredentials(cfg, pool, cipher, redisClient, enqueuer, opts)
	if err != nil {
		closeAll()
		return Built{}, err
	}

	solarAdapter := opts.Solar
	if solarAdapter == nil {
		httpxPool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: cfg.External.PinnedCerts, Locker: lock.NewRedis(redisClient)})
		if err != nil {
			closeAll()
			return Built{}, fmt.Errorf("apiwire: build iSolar pool: %w", err)
		}
		solarAdapter = isolar.New(httpxPool, isolar.Options{})
	}
	// F9: the API reads plant data and links plants; only the worker syncs on a schedule.
	solarService := solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(pool), Production: postgres.NewProductionRepository(pool),
		Totals: postgres.NewProductionTotalsRepository(pool), Faults: postgres.NewFaultRepository(pool),
		Solar: postgres.NewSolarTariffRepository(pool), Analytics: postgres.NewAnalyticsRepository(pool),
		Analyzers: postgres.NewAnalyzerRepository(pool), Bills: billRepo,
		Ops: postgres.NewOpsRepository(pool), Creds: credentialService, ISolar: solarAdapter, Clock: opts.Clock,
		Enqueuer: enqueuer, Inspector: taskInspector, MaxRetry: cfg.Worker.MaxRetries,
	})

	billRequests.Plants = solarService

	// R291: weather only when a provider is configured; air-gapped says so.
	weatherDeps := weathersvc.Deps{Cache: platformredis.NewCache(redisClient, opts.RedisPrefix+"weather:"),
		Plants: postgres.NewPlantRepository(pool), Buildings: postgres.NewBuildingRepository(pool)}
	if opts.Weather != nil {
		weatherDeps.Provider = opts.Weather
	} else if cfg.External.WeatherProvider == "open-meteo" {
		weatherPool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: cfg.External.PinnedCerts})
		if err != nil {
			closeAll()
			return Built{}, fmt.Errorf("apiwire: build weather pool: %w", err)
		}
		weatherDeps.Provider = isweather.New(weatherPool, cfg.External.WeatherBaseURL)
	}
	weatherService := weathersvc.New(weatherDeps)
	renewableService := renewable.New(renewable.Deps{Analyzers: postgres.NewAnalyzerRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Bills: billRepo, Forecasts: postgres.NewForecastRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Clock: opts.Clock})
	financialService := financial.New(financial.Deps{Bills: billRepo, Plants: postgres.NewPlantRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Solar: postgres.NewSolarTariffRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), Tariffs: postgres.NewTariffRepository(pool), Clock: opts.Clock})

	jobService, err := jobs.New(jobs.Deps{Inspector: inspector, Analyzers: postgres.NewAnalyzerRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), Ops: postgres.NewOpsRepository(pool), Reports: postgres.NewReportRepository(pool),
		Plants: postgres.NewPlantRepository(pool)})
	if err != nil {
		closeAll()
		return Built{}, err
	}

	// The API can evaluate a rule as a dry run (R223), so it gets the reads
	// the evaluation needs; the firing path itself belongs to the worker.
	alarmService, err := alarms.New(alarms.Deps{
		Alarms:    postgres.NewAlarmRepository(pool),
		Analyzers: postgres.NewAnalyzerRepository(pool),
		Clock:     opts.Clock,
	})
	if err != nil {
		closeAll()
		return Built{}, err
	}
	alarmService = alarmService.WithEvaluate(alarms.EvaluateDeps{
		Readings: postgres.NewReadingRepository(pool), Bills: billRepo,
	})

	opsService, err := ops.New(ops.Deps{
		Ops: postgres.NewOpsRepository(pool), Enqueuer: enqueuer, Clock: opts.Clock, MaxRetry: cfg.Worker.MaxRetries,
	})
	if err != nil {
		closeAll()
		return Built{}, err
	}

	clientIP := middleware.ClientIP(cfg.HTTP.TrustedProxies)
	handlers := &v1.Handlers{
		Calendar: calendarService, Credentials: credentialService, Jobs: jobService, Alarms: alarmService, Ops: opsService,
		Definitions: integrations.Definitions{Integrations: postgres.NewIntegrationRepository(pool, cipher), Catalogue: admin.NewCatalogueRepository(pool)}, Auth: authService, Tenancy: tenancyService, Assets: assetService, Analysis: analysisService,
		Tariffs: tariffService, Billing: billingService, BillRequests: billRequests, Reports: reportRequests, Solar: solarService, Weather: weatherService, Renewable: renewableService, Financial: financialService,
		ISO: isosvc.New(isosvc.Deps{ISO: postgres.NewISO50001Repository(pool), Files: postgres.NewFileRepository(pool),
			Buildings: postgres.NewBuildingRepository(pool), Clock: opts.Clock,
			Store: storage.Store{Root: cfg.Storage.Root, Max: cfg.Storage.UploadMax, Allowed: cfg.Storage.AllowedTypes}}),
		UploadMax: cfg.Storage.UploadMax,
		PublicForms: publicforms.New(publicforms.Deps{SMTP: postgres.NewSMTPRepository(pool, cipher), Mail: opts.Mail,
			CompanyID: formsCompany, To: cfg.PublicForms.To}),
		Carbon: carbonsvc.New(carbonsvc.Deps{Carbon: postgres.NewCarbonRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
			Companies: postgres.NewCompanyRepository(pool), Clock: opts.Clock}),
		Clock: opts.Clock, Log: log, ClientIP: clientIP}
	router := v1.NewRouter(handlers, middlewareFor(cfg, redisClient, authService, auditRepo, admin.NewAuditRepository(pool), clientIP, opts.RedisPrefix, log), log)
	var once sync.Once
	return Built{V1: router, Auth: authService, Close: func() { once.Do(closeAll) }}, nil
}

func middlewareFor(cfg *config.Config, rc *goredis.Client, a mw.Authenticator, tenantAudit *postgres.AuditRepository,
	platformAudit *admin.AuditRepository, clientIP func(*http.Request) string, prefix string, log *slog.Logger) v1.Middleware {
	limiter := mw.AuthLimiter{Redis: rc, Limit: cfg.HTTP.RateLimitAuth, ClientIP: clientIP, Prefix: prefix}
	idem := mw.Idempotency{Redis: rc, Prefix: prefix}
	auditor := mw.Auditor{Tenant: tenantAudit, Platform: platformAudit, ClientIP: clientIP, Log: log}
	return v1.Middleware{
		Authn:          mw.Authn(a),
		PrincipalLimit: mw.PrincipalLimit(cfg.HTTP.RateLimitAPI),
		Scope:          mw.Scope(a),
		AuthLimit:      limiter.Middleware,
		Idempotency:    idem.Middleware,
		Audit:          auditor.Middleware,
	}
}

func buildAnalysis(pool *pgxpool.Pool, log *slog.Logger, clk clock.Clock, billing *consumption.Billing) (*analysis.Service, error) {
	analytics, err := consumption.NewAnalytics(consumption.AnalyticsDeps{Analytics: postgres.NewAnalyticsRepository(pool), Log: log})
	if err != nil {
		return nil, fmt.Errorf("apiwire: build analytics: %w", err)
	}
	analyzers := postgres.NewAnalyzerRepository(pool)
	profiles, err := loadprofile.New(loadprofile.Deps{
		Calendar: postgres.NewCalendarRepository(pool), Hourly: postgres.NewAnalyticsRepository(pool), Location: istanbul, Log: log,
	})
	if err != nil {
		return nil, fmt.Errorf("apiwire: build load profile: %w", err)
	}
	svc, err := analysis.New(analysis.Deps{
		Series: analytics, Anomalies: billing, Profiles: profiles, Analyzers: analyzers, Buildings: postgres.NewBuildingRepository(pool),
		Params: postgres.NewBillingParameterRepository(pool), Tariffs: postgres.NewTariffRepository(pool), Clock: clk, Location: istanbul,
	})
	if err != nil {
		return nil, fmt.Errorf("apiwire: build analysis: %w", err)
	}
	return svc, nil
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// providerVerifiers mirrors the worker's resolver: meter adapters from the
// registry, iSolar from its own client.
type providerVerifiers struct {
	registry *integration.Registry
	isolar   *isolar.Client
}

func (v providerVerifiers) Verifier(p integration.Provider) (credentials.Verifier, error) {
	if p == integration.ProviderISolar {
		return v.isolar, nil
	}
	return v.registry.Source(p)
}

func buildCredentials(cfg *config.Config, pool *pgxpool.Pool, cipher *crypto.Cipher, rc *goredis.Client, enq Enqueuer, opts Options) (*credentials.Service, error) {
	redisLock := lock.NewRedis(rc)
	verifiers, isolarTokens := opts.Verifiers, opts.ISolar
	if verifiers == nil || isolarTokens == nil {
		httpxPool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: cfg.External.PinnedCerts, Locker: redisLock})
		if err != nil {
			return nil, fmt.Errorf("apiwire: build httpx pool: %w", err)
		}
		registry, err := integration.NewRegistry(
			osos.New(httpxPool, osos.Options{}), gridbox.New(httpxPool, gridbox.Options{}),
			aril.New(httpxPool, aril.Options{}), pm5340.New(httpxPool, pm5340.Options{}),
		)
		if err != nil {
			return nil, fmt.Errorf("apiwire: build provider registry: %w", err)
		}
		client := isolar.New(httpxPool, isolar.Options{})
		if verifiers == nil {
			verifiers = providerVerifiers{registry: registry, isolar: client}
		}
		if isolarTokens == nil {
			isolarTokens = client
		}
	}
	svc, err := credentials.New(credentials.Deps{
		Integrations: postgres.NewIntegrationRepository(pool, cipher), Analyzers: postgres.NewAnalyzerRepository(pool),
		Buildings: postgres.NewBuildingRepository(pool), Verifiers: verifiers, ISolar: isolarTokens, Enqueuer: enq, Locker: redisLock, Nonces: redisLock, Clock: opts.Clock,
		StateKey: worker.StateKey(cfg.Security.JWTSigningKey), RedirectURI: worker.RedirectURI(cfg), MaxRetry: cfg.Worker.MaxRetries,
	})
	if err != nil {
		return nil, fmt.Errorf("apiwire: build credential service: %w", err)
	}
	return svc, nil
}
