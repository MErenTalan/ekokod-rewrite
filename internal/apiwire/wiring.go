// Package apiwire is the API process's single wiring point (R181): it builds
// every repository, service and the /api/v1 router from configuration.
package apiwire

import (
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
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/analysis"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/calendar"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/integrations"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/tenancy"
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
		Params: postgres.NewBillingParameterRepository(pool), Clock: opts.Clock, Log: log,
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
		Service: billingService, Enqueuer: enqueuer, MaxRetry: cfg.Worker.MaxRetries, Location: istanbul,
		Renderer: billingsvc.PDFRenderer{Bills: billRepo, Companies: postgres.NewCompanyRepository(pool),
			Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool), Root: cfg.Storage.Root},
	}

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

	clientIP := middleware.ClientIP(cfg.HTTP.TrustedProxies)
	handlers := &v1.Handlers{
		Calendar: calendarService, Credentials: credentialService,
		Definitions: integrations.Definitions{Integrations: postgres.NewIntegrationRepository(pool, cipher), Catalogue: admin.NewCatalogueRepository(pool)}, Auth: authService, Tenancy: tenancyService, Assets: assetService, Analysis: analysisService,
		Tariffs: tariffService, Billing: billingService, BillRequests: billRequests, Clock: opts.Clock, Log: log, ClientIP: clientIP}
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
		Verifiers: verifiers, ISolar: isolarTokens, Enqueuer: enq, Locker: redisLock, Nonces: redisLock, Clock: opts.Clock,
		StateKey: worker.StateKey(cfg.Security.JWTSigningKey), RedirectURI: worker.RedirectURI(cfg), MaxRetry: cfg.Worker.MaxRetries,
	})
	if err != nil {
		return nil, fmt.Errorf("apiwire: build credential service: %w", err)
	}
	return svc, nil
}
