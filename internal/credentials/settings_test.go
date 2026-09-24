package credentials

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSettingsRejectSecretsAndUnknownKeys proves settings.go's three rules:
// use_billing_indexes (bool) is the only allowed key; any other key is
// ErrInvalidSettings; a key that LOOKS like a secret (matches
// (?i)pass|secret|token|key) gets the dedicated "secrets are not settings"
// message rather than the generic "unknown setting" one.
func TestSettingsRejectSecretsAndUnknownKeys(t *testing.T) {
	t.Run("allows use_billing_indexes bool", func(t *testing.T) {
		raw, err := validateSettings(json.RawMessage(`{"use_billing_indexes": true}`))
		require.NoError(t, err)
		require.JSONEq(t, `{"use_billing_indexes": true}`, string(raw))
	})

	t.Run("empty settings default to {}", func(t *testing.T) {
		raw, err := validateSettings(nil)
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(raw))
	})

	t.Run("wrong type for use_billing_indexes", func(t *testing.T) {
		_, err := validateSettings(json.RawMessage(`{"use_billing_indexes": "yes"}`))
		require.ErrorIs(t, err, ErrInvalidSettings)
	})

	t.Run("unknown key rejected", func(t *testing.T) {
		_, err := validateSettings(json.RawMessage(`{"some_other_flag": true}`))
		require.ErrorIs(t, err, ErrInvalidSettings)
		require.NotContains(t, err.Error(), "secrets are not settings")
	})

	for _, key := range []string{"password", "api_secret", "auth_token", "app_key", "PASSWORD", "Secret_Value"} {
		t.Run("secretish key "+key, func(t *testing.T) {
			raw := json.RawMessage(`{"` + key + `": "hunter2"}`)
			_, err := validateSettings(raw)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrInvalidSettings))
			require.Contains(t, err.Error(), "secrets are not settings")
			// The rejected value must never appear in the error text.
			require.NotContains(t, err.Error(), "hunter2")
		})
	}

	t.Run("malformed json", func(t *testing.T) {
		_, err := validateSettings(json.RawMessage(`{not json`))
		require.ErrorIs(t, err, ErrInvalidSettings)
	})

	t.Run("trailing data after object", func(t *testing.T) {
		_, err := validateSettings(json.RawMessage(`{}{}`))
		require.ErrorIs(t, err, ErrInvalidSettings)
	})
}
