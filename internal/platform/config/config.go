// Package config loads and validates the whole application configuration from
// the environment. The process refuses to start on an invalid configuration:
// no secret has a default, and every problem is reported at once.
package config

import (
	"fmt"
	"strings"
	"time"
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
	EPIASUsername   string
	EPIASPassword   string
	MLURL           string
	MLAPIKey        string
	MLTimeout       time.Duration
	WeatherProvider string
	WeatherAPIKey   string
	MapTileURL      string
	ISolarRedirect  string
	PinnedCerts     map[string]string // host -> base64(DER)
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
