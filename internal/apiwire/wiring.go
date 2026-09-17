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
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	platformredis "github.com/MErenTalan/ekokod-rewrite/internal/store/redis"
)

// Options are test seams; production passes none.
type Options struct {
	Clock clock.Clock
	Mail  mail.Sender
	// RedisPrefix namespaces rate-limit and idempotency keys.
	RedisPrefix string
	// Async overrides how the auth service runs off-request work.
	Async func(func())
}

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
	var once sync.Once
	closeAll := func() { once.Do(func() { _ = redisClient.Close() }) }

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

	clientIP := middleware.ClientIP(cfg.HTTP.TrustedProxies)
	handlers := &v1.Handlers{Auth: authService, Clock: opts.Clock, Log: log, ClientIP: clientIP}
	router := v1.NewRouter(handlers, middlewareFor(cfg, redisClient, authService, auditRepo, admin.NewAuditRepository(pool), clientIP, opts.RedisPrefix, log), log)
	return Built{V1: router, Auth: authService, Close: closeAll}, nil
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
