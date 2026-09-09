package cli

import (
	"io"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
)

// newCommandLogger builds the structured logger every long-running command
// (api, worker, scheduler) uses. It must always go through logging.New —
// never a bare slog.NewJSONHandler/slog.NewTextHandler — because logging.New
// wraps the handler in a redacting one (internal/platform/logging/redact.go)
// that is the sole thing keeping secret-looking attribute values out of the
// logs. This is carried-forward requirement 1 from task 9's brief: it exists
// only because an earlier reviewer found the redacting handler was the sole
// thing keeping secrets out of the logs, and before this function existed,
// reverting any one call site to a bare handler compiled and passed the
// whole suite silently (task 9 review, Minor-4). Centralising the
// construction here means there is exactly one place to get this right, and
// TestNewCommandLoggerRedactsSecretLookingAttributes pins it for every
// caller at once.
func newCommandLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	return logging.New(cfg.LogLevel, string(cfg.LogFormat), w)
}
