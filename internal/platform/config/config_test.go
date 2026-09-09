package config_test

import (
	"strings"
	"testing"
	"time"

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
