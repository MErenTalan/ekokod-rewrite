package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/stretchr/testify/require"
)

// TestCredentialsNeverFormatSecrets pins 03 §7 "never logged" for the one
// type that carries plaintext. Every rendering path a developer might use by
// accident is exercised; a new path that leaks must be added here.
func TestCredentialsNeverFormatSecrets(t *testing.T) {
	const pw = "FIXTURE-PLAINTEXT-7f3a"
	c := integration.Credentials{
		Username: "fixture-user",
		Secret:   integration.NewSecret([]byte(pw)),
		Extra:    map[string]integration.Secret{"access_token": integration.NewSecret([]byte(pw + "-tok"))},
	}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil)) // bare handler on purpose: Secret must not rely on redaction by key
	logger.Info("creds", slog.Any("c", c), slog.Any("s", c.Secret))
	js, err := json.Marshal(c)
	require.NoError(t, err)
	for name, rendered := range map[string]string{
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c),
		"%s": fmt.Sprintf("%s", c.Secret), "%q": fmt.Sprintf("%q", c.Secret), "%x": fmt.Sprintf("%x", c.Secret),
		"json": string(js), "slog": logBuf.String(),
		"error": fmt.Errorf("wrap: %v", c).Error(),
		// GoString() called directly: since Secret also implements
		// fmt.Formatter, fmt itself never reaches GoStringer (Formatter is
		// checked first, for every verb, including %#v — see the fmt package
		// doc's ordering), so this is the only path that actually exercises
		// GoString's own text. It is a real path a developer could still take
		// by calling GoString() directly instead of going through fmt.
		"GoString()": c.Secret.GoString(),
	} {
		require.NotContains(t, rendered, pw, "secret leaked through %s", name)
	}
	require.Equal(t, pw, c.Secret.Reveal())
}
