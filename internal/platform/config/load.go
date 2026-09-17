package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

type loader struct {
	lookup   func(string) (string, bool)
	errs     []error
	resolved []Resolved
}

func (l *loader) fail(name string, err error) {
	l.errs = append(l.errs, fmt.Errorf("%s: %w", name, err))
}

func (l *loader) record(name, value string, secret bool, fromEnv bool) {
	source := "default"
	if fromEnv {
		source = "env"
	}
	l.resolved = append(l.resolved, Resolved{Name: name, Value: value, Secret: secret, Source: source})
}

// raw returns the raw value and whether it came from the environment.
func (l *loader) raw(name string) (string, bool) {
	v, ok := l.lookup(name)
	return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
}

func (l *loader) str(name, def string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		v = def
	}
	l.record(name, v, false, fromEnv)
	return v
}

func (l *loader) required(name string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set"))
	}
	l.record(name, v, false, fromEnv)
	return v
}

// httpsURL reads an optional URL that defaults to def
// when unset, and fails when the resolved value's scheme is not https —
// TLS verification is never disabled anywhere in this codebase (06 §1 rule
// 7), and a provider base URL is exactly the kind of value an operator
// could otherwise silently downgrade to plain http.
func (l *loader) httpsURL(name, def string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		v = def
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" {
		l.fail(name, errors.New("must be an https URL"))
		l.record(name, v, false, fromEnv)
		return v
	}
	l.record(name, v, false, fromEnv)
	return v
}

// dsnDisplay renders a DSN for display, always carrying the secret mask
// glyph so every Resolved row flagged Secret is visibly masked, even when
// the particular DSN happens to carry no embedded credential.
func dsnDisplay(v string) string {
	return maskGlyph + " " + redactDSN(v)
}

func (l *loader) requiredDSN(name string) string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set"))
		l.record(name, maskSecret(""), true, false)
		return ""
	}
	if _, err := url.Parse(v); err != nil {
		// Do not wrap the url.Parse error: url.Error.Error() embeds the raw
		// input verbatim, which would print the DSN password in clear.
		l.fail(name, errors.New("is not a valid URL"))
		l.record(name, maskGlyph+" (invalid — value withheld)", true, true)
		return v
	}
	l.record(name, dsnDisplay(v), true, true)
	return v
}

// secret reads a required secret. Secrets never have defaults and are masked.
func (l *loader) secret(name string, minLen int) []byte {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, errors.New("is required but not set (secrets have no default)"))
		l.record(name, maskSecret(""), true, false)
		return nil
	}
	if len(v) < minLen {
		l.fail(name, fmt.Errorf("must be at least %d characters", minLen))
	}
	l.record(name, maskSecret(v), true, true)
	return []byte(v)
}

func (l *loader) optionalSecret(name string) []byte {
	v, fromEnv := l.raw(name)
	l.record(name, maskSecret(v), true, fromEnv)
	if !fromEnv {
		return nil
	}
	return []byte(v)
}

// key reads a required base64 key of an exact byte length.
func (l *loader) key(name string, wantBytes int) []byte {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.fail(name, fmt.Errorf("is required but not set (must be base64 of %d bytes)", wantBytes))
		l.record(name, maskSecret(""), true, false)
		return nil
	}
	key, err := parseKey(v, wantBytes)
	if err != nil {
		l.fail(name, err)
	}
	l.record(name, maskSecret(v), true, true)
	return key
}

func (l *loader) intVal(name string, def int) int {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.Itoa(def), false, false)
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail(name, fmt.Errorf("must be an integer, got %q", v))
		n = def
	}
	l.record(name, v, false, true)
	return n
}

// positiveInt is intVal with a "> 0" constraint. It exists because zero is a
// legitimate value for several counts here (EKOKOD_DB_MIN_CONNS,
// EKOKOD_JOB_MAX_RETRIES, and the Redis database indices — see
// nonNegativeInt), so the constraint cannot live in intVal itself and must be
// opted into per field.
func (l *loader) positiveInt(name string, def int) int {
	n := l.intVal(name, def)
	if n <= 0 {
		l.fail(name, errors.New("must be greater than zero"))
		return def
	}
	return n
}

// nonNegativeInt is intVal with a ">= 0" constraint, for the Redis logical
// database indices. positiveInt would be wrong — database 0 is a real
// database and the documented default for the cache — but a negative is not
// merely odd, it is silently wrong: go-redis issues SELECT on connect only
// under `if c.opt.DB > 0` (redis.go:840-841), so a negative fails that guard
// exactly as zero does and the connection simply lands on database 0. A
// typo'd EKOKOD_REDIS_QUEUE_DB=-1 would therefore collapse the job queue onto
// the cache's own database with no error at all, defeating the separation
// this project relies on (see internal/store/redis/redis.go:1-3: "a cache
// flush can never drop queued work").
func (l *loader) nonNegativeInt(name string, def int) int {
	n := l.intVal(name, def)
	if n < 0 {
		l.fail(name, errors.New("must be zero or greater"))
		return def
	}
	return n
}

func (l *loader) int64Val(name string, def int64) int64 {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.FormatInt(def, 10), false, false)
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.fail(name, fmt.Errorf("must be an integer, got %q", v))
		n = def
	}
	l.record(name, v, false, true)
	return n
}

// positiveInt64 is int64Val with a "> 0" constraint.
func (l *loader) positiveInt64(name string, def int64) int64 {
	n := l.int64Val(name, def)
	if n <= 0 {
		l.fail(name, errors.New("must be greater than zero"))
		return def
	}
	return n
}

func (l *loader) boolVal(name string, def bool) bool {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strconv.FormatBool(def), false, false)
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail(name, fmt.Errorf("must be true or false, got %q", v))
		b = def
	}
	l.record(name, v, false, true)
	return b
}

func (l *loader) duration(name string, def time.Duration) time.Duration {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, def.String(), false, false)
		return def
	}
	d, err := parseDuration(v)
	if err != nil {
		l.fail(name, err)
		d = def
	}
	l.record(name, v, false, true)
	return d
}

// positiveDuration is duration with a "> 0" constraint, for the fields where a
// zero or negative value is neither "unset" nor "unlimited" but a silent
// hazard. A negative EKOKOD_JOB_TIMEOUT reaches asynq's shutdownTimeout, whose
// time.AfterFunc then closes the abort channel immediately and kills every
// in-flight task with no drain at all; a zero or negative
// EKOKOD_DB_MAX_CONN_LIFETIME expires every pooled connection the moment it is
// opened. Neither surfaces as an error at startup, so the check has to happen
// here. The constraint is opt-in rather than baked into duration() because
// optionalDuration's zero legitimately means "unlimited" and
// EKOKOD_DB_STATEMENT_TIMEOUT's zero legitimately means "no timeout".
func (l *loader) positiveDuration(name string, def time.Duration) time.Duration {
	d := l.duration(name, def)
	if d <= 0 {
		l.fail(name, errors.New("must be greater than zero"))
		return def
	}
	return d
}

// optionalDuration returns zero when unset, which callers read as "unlimited".
func (l *loader) optionalDuration(name string) time.Duration {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, "(unlimited)", false, false)
		return 0
	}
	d, err := parseDuration(v)
	if err != nil {
		l.fail(name, err)
	}
	l.record(name, v, false, true)
	return d
}

// parseDuration accepts Go durations plus a day suffix ("90d").
func parseDuration(v string) (time.Duration, error) {
	if strings.HasSuffix(v, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		if err != nil || days < 0 {
			return 0, fmt.Errorf("must be a duration such as 90d or 1h30m, got %q", v)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("must be a duration such as 30s or 1h, got %q", v)
	}
	return d, nil
}

// decimalVal reads a decimal.Decimal, defaulting to def when unset. An
// unparsable value fails and falls back to def, matching every other
// loader method's shape — money/multiplier values are decimal.Decimal
// throughout F2 (Global Constraints: "never float64"), so this is the one
// place config itself parses a decimal string rather than delegating to
// strconv.
func (l *loader) decimalVal(name string, def decimal.Decimal) decimal.Decimal {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, def.String(), false, false)
		return def
	}
	d, err := decimal.NewFromString(v)
	if err != nil {
		l.fail(name, fmt.Errorf("must be a decimal number, got %q", v))
		d = def
	}
	l.record(name, v, false, true)
	return d
}

func (l *loader) enum(name, def string, allowed ...string) string {
	v := l.str(name, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	l.fail(name, fmt.Errorf("must be one of %s, got %q", strings.Join(allowed, ", "), v))
	return def
}

func (l *loader) csv(name string, def []string) []string {
	v, fromEnv := l.raw(name)
	if !fromEnv {
		l.record(name, strings.Join(def, ","), false, false)
		return def
	}
	l.record(name, v, false, true)
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (l *loader) cronExpr(name, def string) string {
	v := l.str(name, def)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(v); err != nil {
		l.fail(name, fmt.Errorf("is not a valid cron expression: %w", err))
	}
	return v
}

func (l *loader) location(name, def string) *time.Location {
	v := l.str(name, def)
	loc, err := time.LoadLocation(v)
	if err != nil {
		l.fail(name, fmt.Errorf("is not a known timezone: %w", err))
		loc = time.UTC
	}
	return loc
}

func (l *loader) rateLimit(name, def string) RateLimit {
	v := l.str(name, def)
	rl, err := parseRateLimit(v)
	if err != nil {
		l.fail(name, err)
	}
	return rl
}

// FromEnv loads the configuration from the process environment.
func FromEnv() (*Config, error) { return Load(os.LookupEnv) }

// Load reads and validates the configuration, reporting every problem at once.
func Load(lookup func(string) (string, bool)) (*Config, error) {
	l := &loader{lookup: lookup}
	c := &Config{}

	c.Env = Environment(l.enum("EKOKOD_ENV", string(EnvProduction),
		string(EnvDevelopment), string(EnvStaging), string(EnvProduction)))
	c.LogLevel = l.enum("EKOKOD_LOG_LEVEL", "info", "debug", "info", "warn", "error")
	c.LogFormat = LogFormat(l.enum("EKOKOD_LOG_FORMAT", string(LogFormatJSON),
		string(LogFormatJSON), string(LogFormatText)))
	c.Timezone = l.location("EKOKOD_TIMEZONE", "Europe/Istanbul")
	c.DefaultLocale = l.enum("EKOKOD_DEFAULT_LOCALE", "tr", "tr", "en")

	c.HTTP = HTTP{
		Addr:           l.str("EKOKOD_HTTP_ADDR", ":8080"),
		PublicURL:      l.required("EKOKOD_PUBLIC_URL"),
		CORSOrigins:    l.csv("EKOKOD_CORS_ORIGINS", nil),
		TrustedProxies: l.csv("EKOKOD_TRUSTED_PROXIES", nil),
		RateLimitAPI:   l.rateLimit("EKOKOD_RATE_LIMIT_API", "120/min"),
		RateLimitAuth:  l.rateLimit("EKOKOD_RATE_LIMIT_AUTH", "5/15min"),
	}

	c.DB = DB{
		URL:              l.requiredDSN("EKOKOD_DB_URL"),
		MaxConns:         l.intVal("EKOKOD_DB_MAX_CONNS", 25),
		MinConns:         l.intVal("EKOKOD_DB_MIN_CONNS", 5),
		MaxConnLifetime:  l.positiveDuration("EKOKOD_DB_MAX_CONN_LIFETIME", time.Hour),
		StatementTimeout: l.duration("EKOKOD_DB_STATEMENT_TIMEOUT", 30*time.Second),
		ReadingRetention: l.optionalDuration("EKOKOD_READING_RETENTION"),
		CompressionAfter: l.duration("EKOKOD_COMPRESSION_AFTER", 90*24*time.Hour),
	}

	c.Redis = Redis{
		URL:     l.requiredDSN("EKOKOD_REDIS_URL"),
		CacheDB: l.nonNegativeInt("EKOKOD_REDIS_CACHE_DB", 0),
		QueueDB: l.nonNegativeInt("EKOKOD_REDIS_QUEUE_DB", 1),
	}

	c.Security = Security{
		EncryptionKey:           l.key("EKOKOD_ENCRYPTION_KEY", 32),
		JWTSigningKey:           l.secret("EKOKOD_JWT_SIGNING_KEY", 32),
		PasswordPepper:          l.secret("EKOKOD_PASSWORD_PEPPER", 32),
		DeviceFingerprintSecret: l.secret("EKOKOD_DEVICE_FINGERPRINT_SECRET", 32),
		AccessTokenTTL:          l.positiveDuration("EKOKOD_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:         l.positiveDuration("EKOKOD_REFRESH_TOKEN_TTL", 24*time.Hour),
		RefreshTokenRememberTTL: l.positiveDuration("EKOKOD_REFRESH_TOKEN_REMEMBER_TTL", 720*time.Hour),
		BcryptCost:              l.intVal("EKOKOD_BCRYPT_COST", 12),
		PasswordHistorySize:     l.intVal("EKOKOD_PASSWORD_HISTORY_SIZE", 5),
		LegacyEncryptionKey:     l.optionalSecret("EKOKOD_LEGACY_ENCRYPTION_KEY"),
	}
	if c.Security.RefreshTokenRememberTTL < c.Security.RefreshTokenTTL {
		l.fail("EKOKOD_REFRESH_TOKEN_REMEMBER_TTL", errors.New("must not be shorter than EKOKOD_REFRESH_TOKEN_TTL"))
	}
	if c.Security.BcryptCost < 12 {
		l.fail("EKOKOD_BCRYPT_COST", errors.New("must be at least 12"))
	}

	// EKOKOD_READING_RETENTION is the one remaining unbounded knob, left
	// that way on purpose: nothing consumes it yet, and what a negative
	// retention window should mean is the consuming phase's decision, not
	// this one's. EKOKOD_JOB_MAX_RETRIES got that same treatment in F1;
	// F2 is the consuming phase (internal/ingest, internal/worker), so it
	// now uses nonNegativeInt — asynq's MaxRetry is a count, and a negative
	// one is not "unlimited" or "unset", it is a config mistake (see
	// nonNegativeInt's own doc for the parallel EKOKOD_REDIS_*_DB rationale;
	// zero stays legal, it is job.TaskOptions' own documented "no retries").
	c.Worker = Worker{
		Concurrency: l.positiveInt("EKOKOD_WORKER_CONCURRENCY", 10),
		MaxRetries:  l.nonNegativeInt("EKOKOD_JOB_MAX_RETRIES", 5),
		Timeout:     l.positiveDuration("EKOKOD_JOB_TIMEOUT", 30*time.Minute),
	}
	c.Scheduler = Scheduler{Enabled: l.boolVal("EKOKOD_SCHEDULER_ENABLED", true)}
	c.Schedule = Schedule{
		Ingestion:      l.cronExpr("EKOKOD_SCHEDULE_INGESTION", "0 3 * * *"),
		EPIAS:          l.cronExpr("EKOKOD_SCHEDULE_EPIAS", "0 14 * * *"),
		Alarms:         l.cronExpr("EKOKOD_SCHEDULE_ALARMS", "0 * * * *"),
		Billing:        l.cronExpr("EKOKOD_SCHEDULE_BILLING", "0 5 * * *"),
		Forecast:       l.cronExpr("EKOKOD_SCHEDULE_FORECAST", "0 4 * * *"),
		Carbon:         l.cronExpr("EKOKOD_SCHEDULE_CARBON", "30 4 * * *"),
		ReportsMonthly: l.cronExpr("EKOKOD_SCHEDULE_REPORTS_MONTHLY", "0 6 2 * *"),
		ReportsYearly:  l.cronExpr("EKOKOD_SCHEDULE_REPORTS_YEARLY", "0 7 3 1 *"),
		Demo:           l.cronExpr("EKOKOD_SCHEDULE_DEMO", "15 * * * *"),
	}

	c.Storage = Storage{
		Root:      l.required("EKOKOD_STORAGE_ROOT"),
		UploadMax: l.positiveInt64("EKOKOD_UPLOAD_MAX_BYTES", 31457280),
		AllowedTypes: l.csv("EKOKOD_UPLOAD_ALLOWED_TYPES", []string{
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.ms-excel",
			"text/csv",
			"image/png",
			"image/jpeg",
		}),
	}

	c.External = External{
		EPIASUsername:   l.required("EKOKOD_EPIAS_USERNAME"),
		EPIASPassword:   string(l.secret("EKOKOD_EPIAS_PASSWORD", 1)),
		EPIASCASURL:     l.httpsURL("EKOKOD_EPIAS_CAS_URL", "https://giris.epias.com.tr/cas/v1/tickets"),
		EPIASBaseURL:    l.httpsURL("EKOKOD_EPIAS_BASE_URL", "https://seffaflik.epias.com.tr/electricity-service"),
		MLURL:           l.str("EKOKOD_ML_URL", "http://ml:8000"),
		MLAPIKey:        string(l.secret("EKOKOD_ML_API_KEY", 1)),
		MLTimeout:       l.positiveDuration("EKOKOD_ML_TIMEOUT", 60*time.Second),
		WeatherProvider: l.str("EKOKOD_WEATHER_PROVIDER", ""),
		MapTileURL:      l.str("EKOKOD_MAP_TILE_URL", ""),
		ISolarRedirect:  l.str("EKOKOD_ISOLAR_REDIRECT_URL", ""),
	}
	if c.External.WeatherProvider != "" {
		c.External.WeatherAPIKey = string(l.secret("EKOKOD_WEATHER_API_KEY", 1))
	} else {
		l.record("EKOKOD_WEATHER_API_KEY", maskSecret(""), true, false)
	}
	if raw, ok := l.raw("EKOKOD_PINNED_CERTS"); ok {
		certs, err := parsePinnedCerts(raw)
		if err != nil {
			l.fail("EKOKOD_PINNED_CERTS", err)
		}
		c.External.PinnedCerts = certs
		l.record("EKOKOD_PINNED_CERTS", fmt.Sprintf("%d pinned host(s)", len(certs)), false, true)
	} else {
		l.record("EKOKOD_PINNED_CERTS", "(none)", false, false)
	}

	c.Features = Features{
		SelfRegistration: l.boolVal("EKOKOD_FEATURE_SELF_REGISTRATION", false),
		PricingPage:      l.boolVal("EKOKOD_FEATURE_PRICING_PAGE", false),
		SMSAlarms:        l.boolVal("EKOKOD_FEATURE_SMS_ALARMS", false),
		Analytics:        l.boolVal("EKOKOD_FEATURE_ANALYTICS", false),
		Tracing:          l.boolVal("EKOKOD_FEATURE_TRACING", false),
	}

	c.Ingest = Ingest{
		SanityMultiple:  l.decimalVal("EKOKOD_INGEST_SANITY_MULTIPLE", decimal.NewFromInt(10)),
		FutureTolerance: l.positiveDuration("EKOKOD_INGEST_FUTURE_TOLERANCE", 15*time.Minute),
		InitialLookback: l.positiveDuration("EKOKOD_INGEST_INITIAL_LOOKBACK", 720*time.Hour),
	}
	if c.Ingest.SanityMultiple.LessThanOrEqual(decimal.NewFromInt(1)) {
		l.fail("EKOKOD_INGEST_SANITY_MULTIPLE", errors.New("must be greater than 1"))
	}

	// R73: the ingest enqueue gate and consumption.Refresher's
	// single global lock TTL (wired into RefreshDeps.LockTTL by
	// worker/wiring.go). R100(5): 30m default.
	c.ConsumptionRefreshEnabled = l.boolVal("EKOKOD_CONSUMPTION_REFRESH_ENABLED", true)
	c.ConsumptionRefreshLockTTL = l.positiveDuration("EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL", 30*time.Minute)

	c.resolved = l.resolved
	if len(l.errs) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  %w", errors.Join(l.errs...))
	}
	return c, nil
}
