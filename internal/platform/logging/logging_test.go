package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &out))
	return out
}

func TestLoggerRedactsSecretAttributes(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("info", "json", &buf)

	log.Info("integration configured",
		slog.String("password", "s3cret"),
		slog.String("api_key", "abcdef"),
		slog.String("authorization", "Bearer xyz"),
		slog.String("encryption_key", "aGVsbG8="),
		slog.String("provider", "gridbox"))

	line := buf.String()
	require.NotContains(t, line, "s3cret")
	require.NotContains(t, line, "abcdef")
	require.NotContains(t, line, "Bearer xyz")
	require.NotContains(t, line, "aGVsbG8=")
	require.Contains(t, line, "gridbox")
	require.Contains(t, line, "[REDACTED]")
}

func TestLoggerRedactsNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("info", "json", &buf)
	log.Info("credentials", slog.Group("credential", slog.String("secret", "hunter2")))
	require.NotContains(t, buf.String(), "hunter2")
}

func TestLoggerEmitsContextFields(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("debug", "json", &buf)

	ctx := logging.WithField(context.Background(), logging.FieldRequestID, "req-1")
	ctx = logging.WithField(ctx, logging.FieldCompany, "company-9")
	log.InfoContext(ctx, "handled")

	got := decode(t, &buf)
	require.Equal(t, "req-1", got["request_id"])
	require.Equal(t, "company-9", got["company_id"])
}

type creds struct{}

func (creds) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("username", "svc-acct"),
		slog.String("password", "hunter2-logvaluer"),
	)
}

func TestLoggerRedactsSecretsBehindLogValuer(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("info", "json", &buf)

	log.Info("connecting", slog.Any("config", creds{}))

	line := buf.String()
	require.NotContains(t, line, "hunter2-logvaluer")
	require.Contains(t, line, "svc-acct")
	require.Contains(t, line, "[REDACTED]")
}

func TestLevelIsHonoured(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New("warn", "json", &buf)
	log.Info("should not appear")
	require.Empty(t, buf.String())
	log.Warn("should appear")
	require.Contains(t, buf.String(), "should appear")
}
