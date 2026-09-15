package credentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
)

// settingsUseBillingIndexes is the one key settings.go allows.
const settingsUseBillingIndexes = "use_billing_indexes"

// settingsSecretishKey matches a settings key that looks like it is trying
// to smuggle secret material through integration_credentials.settings — the
// one JSON column List/View returns verbatim (unlike secret_enc/extra_enc,
// it is never sealed). Any key matching this, known or not, gets a
// dedicated "secrets are not settings" message instead of the generic
// "unknown setting" one.
var settingsSecretishKey = regexp.MustCompile(`(?i)pass|secret|token|key`)

// validateSettings enforces the one allowed key (use_billing_indexes, bool)
// and rejects everything else. A nil/empty raw is treated as "{}" —
// Configure/Update both accept "no settings supplied".
func validateSettings(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`), nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	var m map[string]json.RawMessage
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSettings, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after JSON value", ErrInvalidSettings)
	}

	for k, v := range m {
		if k != settingsUseBillingIndexes {
			if settingsSecretishKey.MatchString(k) {
				return nil, fmt.Errorf("%w: secrets are not settings (key %q)", ErrInvalidSettings, k)
			}
			return nil, fmt.Errorf("%w: unknown setting %q", ErrInvalidSettings, k)
		}
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, fmt.Errorf("%w: %q must be a boolean", ErrInvalidSettings, k)
		}
	}
	return raw, nil
}
