// Package config loads and validates the whole application configuration from
// the environment. The process refuses to start on an invalid configuration:
// no secret has a default, and every problem is reported at once.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Environment is the deployment environment.
type Environment string

// The deployment environments Config.Env may resolve to.
const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// LogFormat selects the slog handler.
type LogFormat string

// The log formats Config.LogFormat may resolve to.
const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// RateLimit is a token bucket expressed as "<limit>/<window>", e.g. "120/min".
type RateLimit struct {
	Limit  int
	Window time.Duration
}

// Config is the fully resolved configuration.
type Config struct {
	Env           Environment
	LogLevel      string
	LogFormat     LogFormat
	Timezone      *time.Location
	DefaultLocale string

	HTTP      HTTP
	DB        DB
	Redis     Redis
	Security  Security
	Worker    Worker
	Scheduler Scheduler
	Schedule  Schedule
	Storage   Storage
	External  External
	Features  Features
	Ingest    Ingest

	// ConsumptionRefreshEnabled gates internal/ingest's consumption.refresh
	// enqueue call site (R73).
	// EKOKOD_CONSUMPTION_REFRESH_ENABLED, default true.
	ConsumptionRefreshEnabled bool
	// ConsumptionRefreshLockTTL is wired into
	// internal/service/consumption.RefreshDeps.LockTTL by worker/wiring.go
	// (R100(5)): how long the SINGLE global
	// "consumption.refresh" lock consumption.Refresher holds is allowed to
	// run before another worker could, in principle, acquire the same key
	// (there is no lease renewal — see RefreshDeps' doc). 30 minutes
	// comfortably covers a platform-wide yearly recompute.
	// EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL, default 30m, must be > 0.
	ConsumptionRefreshLockTTL time.Duration

	resolved []Resolved
}

// HTTP configures the API's listen address and edge behaviour.
type HTTP struct {
	Addr           string
	PublicURL      string
	CORSOrigins    []string
	TrustedProxies []string
	RateLimitAPI   RateLimit
	RateLimitAuth  RateLimit
}

// DB configures the Postgres connection pool and retention policy.
type DB struct {
	URL              string
	MaxConns         int
	MinConns         int
	MaxConnLifetime  time.Duration
	StatementTimeout time.Duration
	ReadingRetention time.Duration // zero means unlimited
	CompressionAfter time.Duration
}

// Redis configures the shared Redis connection used for caching and queues.
type Redis struct {
	URL     string
	CacheDB int
	QueueDB int
}

// Security configures the secrets and parameters that protect
// authentication, encryption and password storage.
type Security struct {
	EncryptionKey           []byte // 32 bytes, AES-256-GCM
	JWTSigningKey           []byte
	PasswordPepper          []byte
	DeviceFingerprintSecret []byte
	AccessTokenTTL          time.Duration
	RefreshTokenTTL         time.Duration
	BcryptCost              int
	PasswordHistorySize     int
	LegacyEncryptionKey     []byte // migration only; may be empty
}

// Worker configures the background job worker.
type Worker struct {
	Concurrency int
	MaxRetries  int
	Timeout     time.Duration
}

// Scheduler configures the cron-driven leader-elected scheduler.
type Scheduler struct {
	Enabled bool
}

// Schedule holds cron expressions, all evaluated in Config.Timezone.
type Schedule struct {
	Ingestion      string
	EPIAS          string
	Alarms         string
	Billing        string
	Forecast       string
	Carbon         string
	ReportsMonthly string
	ReportsYearly  string
}

// Storage configures where and how uploaded files are stored.
type Storage struct {
	Root         string
	UploadMax    int64
	AllowedTypes []string
}

// External configures the third-party integrations the platform calls out to.
type External struct {
	EPIASUsername string
	EPIASPassword string
	// EPIASCASURL and EPIASBaseURL are the EPİAŞ CAS
	// ticket endpoint and electricity-service base URL internal/worker
	// passes to epias.New. Both default to the real production URLs
	// (epias.New's own defaultCASURL/defaultBaseURL) and must be https —
	// this is the only seam through which a test can point a REAL
	// worker.Build-constructed EPİAŞ client at a fake.NewTLSServer instead
	// of substituting Handlers.Prices at the test layer.
	EPIASCASURL     string
	EPIASBaseURL    string
	MLURL           string
	MLAPIKey        string
	MLTimeout       time.Duration
	WeatherProvider string
	WeatherAPIKey   string
	MapTileURL      string
	ISolarRedirect  string
	PinnedCerts     map[string]string // host -> base64(DER)
}

// Ingest configures F2's ingestion pipeline (internal/ingest.Options).
type Ingest struct {
	// SanityMultiple is R13's configurable sanity multiple: a register jump
	// beyond this many times a point's typical interval consumption is
	// rejected. EKOKOD_INGEST_SANITY_MULTIPLE, default "10", must be > 1 —
	// a multiple at or below 1 would reject every reading whose value is
	// merely equal to (or moderately above) the typical one, which is
	// ordinary data, not a sanity violation.
	SanityMultiple decimal.Decimal
	// FutureTolerance bounds how far into the future a reading's timestamp
	// may sit before it is rejected. EKOKOD_INGEST_FUTURE_TOLERANCE,
	// default 15m, must be > 0.
	FutureTolerance time.Duration
	// InitialLookback is how far back a first-ever fetch (no stored
	// cursor) starts, for an analyzer and kind. EKOKOD_INGEST_INITIAL_LOOKBACK,
	// default 720h (30 days), must be > 0.
	InitialLookback time.Duration
}

// Features toggles optional platform functionality.
type Features struct {
	SelfRegistration bool
	PricingPage      bool
	SMSAlarms        bool
	Analytics        bool
	Tracing          bool
}

// Resolved returns every configuration variable with its resolved value,
// secrets masked, for `ekokod config:check`.
func (c *Config) Resolved() []Resolved { return c.resolved }

// String renders the configuration with every secret masked. It is safe to log.
func (c *Config) String() string {
	var b strings.Builder
	for _, r := range c.resolved {
		fmt.Fprintf(&b, "%s=%s (%s)\n", r.Name, r.Value, r.Source)
	}
	return b.String()
}
