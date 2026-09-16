package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/stretchr/testify/require"
)

// valid returns a complete, minimal environment.
func valid() map[string]string {
	return map[string]string{
		"EKOKOD_PUBLIC_URL":                "https://ekokod.example.com",
		"EKOKOD_DB_URL":                    "postgres://u:p@localhost:5432/ekokod?sslmode=disable",
		"EKOKOD_REDIS_URL":                 "redis://localhost:6379/0",
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              "/var/lib/ekokod",
		"EKOKOD_EPIAS_USERNAME":            "epias-user",
		"EKOKOD_EPIAS_PASSWORD":            "epias-pass",
		"EKOKOD_ML_API_KEY":                "ml-api-key",
	}
}

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	require.Equal(t, config.EnvProduction, cfg.Env)
	require.Equal(t, "Europe/Istanbul", cfg.Timezone.String())
	require.Equal(t, "tr", cfg.DefaultLocale)
	require.Equal(t, ":8080", cfg.HTTP.Addr)
	require.Equal(t, 25, cfg.DB.MaxConns)
	require.Equal(t, 5, cfg.DB.MinConns)
	require.Equal(t, 30*time.Second, cfg.DB.StatementTimeout)
	require.Equal(t, 1, cfg.Redis.QueueDB)
	require.Equal(t, 10, cfg.Worker.Concurrency)
	require.Equal(t, 5, cfg.Worker.MaxRetries)
	require.Equal(t, 12, cfg.Security.BcryptCost)
	require.Equal(t, 15*time.Minute, cfg.Security.AccessTokenTTL)
	require.True(t, cfg.Scheduler.Enabled)
	require.Equal(t, "0 3 * * *", cfg.Schedule.Ingestion)
	require.Equal(t, config.RateLimit{Limit: 120, Window: time.Minute}, cfg.HTTP.RateLimitAPI)
	require.Equal(t, config.RateLimit{Limit: 5, Window: 15 * time.Minute}, cfg.HTTP.RateLimitAuth)
	require.False(t, cfg.Features.SelfRegistration)
	require.False(t, cfg.Features.PricingPage)
	require.False(t, cfg.Features.SMSAlarms)
}

// TestConfigRejectsMissingSecret is named in the F0 acceptance criteria.
func TestConfigRejectsMissingSecret(t *testing.T) {
	for _, name := range []string{
		"EKOKOD_ENCRYPTION_KEY",
		"EKOKOD_JWT_SIGNING_KEY",
		"EKOKOD_PASSWORD_PEPPER",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET",
		"EKOKOD_DB_URL",
		"EKOKOD_REDIS_URL",
		"EKOKOD_STORAGE_ROOT",
		"EKOKOD_PUBLIC_URL",
		"EKOKOD_EPIAS_PASSWORD",
	} {
		t.Run(name, func(t *testing.T) {
			env := valid()
			delete(env, name)
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			require.Contains(t, err.Error(), name, "the error must name the missing variable")
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	env := valid()
	delete(env, "EKOKOD_DB_URL")
	delete(env, "EKOKOD_REDIS_URL")
	env["EKOKOD_ENV"] = "banana"

	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "EKOKOD_DB_URL")
	require.Contains(t, msg, "EKOKOD_REDIS_URL")
	require.Contains(t, msg, "EKOKOD_ENV")
}

func TestEncryptionKeyMustBe32Bytes(t *testing.T) {
	env := valid()
	env["EKOKOD_ENCRYPTION_KEY"] = "c2hvcnQ=" // "short"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "32 bytes")
}

func TestResolvedMasksSecretsAndNamesSource(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	var sawSecret, sawDefault bool
	for _, r := range cfg.Resolved() {
		if r.Secret {
			sawSecret = true
			require.NotContains(t, r.Value, "password-pepper", "secret value must never be printed")
			require.NotContains(t, r.Value, "epias-pass")
			require.Contains(t, r.Value, "•")
		}
		if r.Name == "EKOKOD_HTTP_ADDR" {
			sawDefault = true
			require.Equal(t, "default", r.Source)
		}
		if r.Name == "EKOKOD_DB_URL" {
			require.Equal(t, "env", r.Source)
			require.NotContains(t, r.Value, ":p@", "DSN password must be redacted")
		}
	}
	require.True(t, sawSecret)
	require.True(t, sawDefault)
}

func TestInvalidCronIsRejected(t *testing.T) {
	env := valid()
	env["EKOKOD_SCHEDULE_BILLING"] = "not a cron"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_SCHEDULE_BILLING")
}

func TestWeatherAPIKeyRequiredWhenProviderSet(t *testing.T) {
	env := valid()
	env["EKOKOD_WEATHER_PROVIDER"] = "openweather"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_WEATHER_API_KEY")

	env["EKOKOD_WEATHER_API_KEY"] = "k"
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	require.Equal(t, "openweather", cfg.External.WeatherProvider)
}

func TestSMSFlagRequiresNothingButDefaultsOff(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)
	require.False(t, cfg.Features.SMSAlarms)
	require.False(t, strings.Contains(cfg.String(), "epias-pass"), "String() must never leak a secret")
}

// TestRequiredDSNNeverLeaksPasswordOnParseError guards against wrapping
// net/url's parse error, which embeds the raw input (including the
// password) verbatim in its Error() text.
func TestRequiredDSNNeverLeaksPasswordOnParseError(t *testing.T) {
	env := valid()
	env["EKOKOD_DB_URL"] = "postgres://user:pa ss@localhost:5432/ekokod?sslmode=disable"

	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_DB_URL")
	require.NotContains(t, err.Error(), "pa ss", "the DSN password must never appear in an error message")
}

// TestNonPositiveDurationsAndCountsAreRejected covers the fields where zero or
// a negative value is a silent hazard rather than a legitimate "unset": asynq
// aborts every in-flight task immediately on a negative shutdown timeout, and
// pgxpool expires every connection the moment it is opened on a non-positive
// MaxConnLifetime. Neither fails loudly on its own, so the loader must.
func TestNonPositiveDurationsAndCountsAreRejected(t *testing.T) {
	for name, bad := range map[string][]string{
		"EKOKOD_DB_MAX_CONN_LIFETIME": {"0s", "-1h"},
		"EKOKOD_JOB_TIMEOUT":          {"0s", "-30m"},
		"EKOKOD_ACCESS_TOKEN_TTL":     {"0s", "-15m"},
		"EKOKOD_REFRESH_TOKEN_TTL":    {"0s", "-24h"},
		"EKOKOD_ML_TIMEOUT":           {"0s", "-60s"},
		"EKOKOD_WORKER_CONCURRENCY":   {"0", "-1"},
		"EKOKOD_UPLOAD_MAX_BYTES":     {"0", "-1"},
	} {
		for _, v := range bad {
			t.Run(name+"="+v, func(t *testing.T) {
				env := valid()
				env[name] = v
				_, err := config.Load(lookupFrom(env))
				require.Error(t, err)
				require.Contains(t, err.Error(), name, "the error must name the offending variable")
				require.Contains(t, err.Error(), "greater than zero")
			})
		}
	}
}

// TestZeroStaysLegalWhereItMeansSomething guards the other half of the rule:
// the positivity constraint is opt-in per field precisely because zero is a
// meaningful value elsewhere — an unset retention is "unlimited", a zero
// statement timeout is "no timeout", and Redis database 0 is a real database.
func TestZeroStaysLegalWhereItMeansSomething(t *testing.T) {
	env := valid()
	env["EKOKOD_DB_STATEMENT_TIMEOUT"] = "0s"
	env["EKOKOD_REDIS_CACHE_DB"] = "0"
	env["EKOKOD_DB_MIN_CONNS"] = "0"
	env["EKOKOD_JOB_MAX_RETRIES"] = "0"

	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	require.Zero(t, cfg.DB.StatementTimeout)
	require.Zero(t, cfg.DB.MinConns)
	require.Zero(t, cfg.Redis.CacheDB)
	require.Zero(t, cfg.Worker.MaxRetries)
	require.Zero(t, cfg.DB.ReadingRetention, "an unset retention still means unlimited")
}

// TestNegativeRedisDatabaseIndexIsRejected covers a failure mode that looks
// like the zero case but is not: go-redis issues SELECT on connect only under
// `if c.opt.DB > 0`, so a negative index fails that guard exactly as zero does
// and the connection silently lands on database 0. A typo'd
// EKOKOD_REDIS_QUEUE_DB=-1 would therefore put the job queue on the cache's own
// database with no error, and a cache flush would drop queued work.
func TestNegativeRedisDatabaseIndexIsRejected(t *testing.T) {
	for _, name := range []string{"EKOKOD_REDIS_CACHE_DB", "EKOKOD_REDIS_QUEUE_DB"} {
		t.Run(name, func(t *testing.T) {
			env := valid()
			env[name] = "-1"
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			require.Contains(t, err.Error(), name, "the error must name the offending variable")
			require.Contains(t, err.Error(), "zero or greater")
		})
	}
}

// TestRedisDatabasesStayDistinctByDefault pins the invariant the check above
// protects: the cache and the queue must never share a logical database.
func TestRedisDatabasesStayDistinctByDefault(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)
	require.NotEqual(t, cfg.Redis.CacheDB, cfg.Redis.QueueDB,
		"a cache flush must never be able to drop queued work")
}

// TestIngestDefaults pins F2's ingest.Options defaults as read through
// config: EKOKOD_INGEST_SANITY_MULTIPLE=10, EKOKOD_INGEST_FUTURE_TOLERANCE=15m,
// EKOKOD_INGEST_INITIAL_LOOKBACK=720h (30 days).
func TestIngestDefaults(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)
	require.True(t, cfg.Ingest.SanityMultiple.Equal(decimal.NewFromInt(10)),
		"want sanity multiple 10, got %s", cfg.Ingest.SanityMultiple)
	require.Equal(t, 15*time.Minute, cfg.Ingest.FutureTolerance)
	require.Equal(t, 720*time.Hour, cfg.Ingest.InitialLookback)
}

// TestIngestSanityMultipleMustExceedOne guards R13: a multiple at or below 1
// would reject ordinary readings merely equal to (or moderately above) the
// typical one, which is not a sanity violation.
func TestIngestSanityMultipleMustExceedOne(t *testing.T) {
	for _, v := range []string{"1", "0.5", "0", "-3"} {
		t.Run(v, func(t *testing.T) {
			env := valid()
			env["EKOKOD_INGEST_SANITY_MULTIPLE"] = v
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			require.Contains(t, err.Error(), "EKOKOD_INGEST_SANITY_MULTIPLE")
			require.Contains(t, err.Error(), "greater than 1")
		})
	}
}

// TestJobMaxRetriesRejectsNegative pins F2's tightened EKOKOD_JOB_MAX_RETRIES
// rule (F1 left it unbounded; F2 is the consuming phase — see load.go): zero
// stays legal ("no retries", job.TaskOptions' own documented meaning), but a
// negative retry count is a config mistake, not a real asynq.MaxRetry value.
func TestJobMaxRetriesRejectsNegative(t *testing.T) {
	env := valid()
	env["EKOKOD_JOB_MAX_RETRIES"] = "-1"
	_, err := config.Load(lookupFrom(env))
	require.Error(t, err)
	require.Contains(t, err.Error(), "EKOKOD_JOB_MAX_RETRIES")
	require.Contains(t, err.Error(), "zero or greater")
}

// TestConsumptionRefreshDefaults pins R100(5)'s defaults: enabled by
// default (an operator who never heard of this knob still gets the R73
// promise), and a 30-minute lock TTL (long enough for a platform-wide
// yearly recompute, final review A I-3(d)).
func TestConsumptionRefreshDefaults(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)
	require.True(t, cfg.ConsumptionRefreshEnabled)
	require.Equal(t, 30*time.Minute, cfg.ConsumptionRefreshLockTTL)
}

// TestConsumptionRefreshEnabledCanBeDisabled pins the operator escape hatch
// the ingest enqueue gate reads.
func TestConsumptionRefreshEnabledCanBeDisabled(t *testing.T) {
	env := valid()
	env["EKOKOD_CONSUMPTION_REFRESH_ENABLED"] = "false"
	cfg, err := config.Load(lookupFrom(env))
	require.NoError(t, err)
	require.False(t, cfg.ConsumptionRefreshEnabled)
}

// TestConsumptionRefreshLockTTLRejectsNonPositive guards the same hazard
// EKOKOD_DB_MAX_CONN_LIFETIME's positiveDuration protects against: a zero or
// negative lock TTL is not a real lease.
func TestConsumptionRefreshLockTTLRejectsNonPositive(t *testing.T) {
	for _, v := range []string{"0", "-1m"} {
		t.Run(v, func(t *testing.T) {
			env := valid()
			env["EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL"] = v
			_, err := config.Load(lookupFrom(env))
			require.Error(t, err)
			require.Contains(t, err.Error(), "EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL")
		})
	}
}

// TestConfigCheckListsConsumptionRefreshVariables guards `ekokod
// config:check`'s coverage of the two Task 11b knobs, exactly like
// TestConfigCheckListsIngestVariables does for EKOKOD_INGEST_*.
func TestConfigCheckListsConsumptionRefreshVariables(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, r := range cfg.Resolved() {
		seen[r.Name] = true
	}
	for _, name := range []string{
		"EKOKOD_CONSUMPTION_REFRESH_ENABLED",
		"EKOKOD_CONSUMPTION_REFRESH_LOCK_TTL",
	} {
		require.True(t, seen[name], "%s must be listed by config:check", name)
	}
}

// TestConfigCheckListsIngestVariables guards `ekokod config:check`'s
// coverage: every EKOKOD_INGEST_* knob must appear in Resolved() so an
// operator can see it (and its masked/unmasked value) without reading
// source, exactly like every other documented variable.
func TestConfigCheckListsIngestVariables(t *testing.T) {
	cfg, err := config.Load(lookupFrom(valid()))
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, r := range cfg.Resolved() {
		seen[r.Name] = true
	}
	for _, name := range []string{
		"EKOKOD_INGEST_SANITY_MULTIPLE",
		"EKOKOD_INGEST_FUTURE_TOLERANCE",
		"EKOKOD_INGEST_INITIAL_LOOKBACK",
	} {
		require.True(t, seen[name], "%s must be listed by config:check", name)
	}
}
