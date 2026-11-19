package cli

import (
	"io"
	"log/slog"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
)

// newCommandLogger builds the structured logger every long-running command
// (api, worker, scheduler) uses. It must always go through logging.New —
// never construct a bare JSON- or text-format slog handler directly — because
// logging.New wraps the handler in a redacting one
// (internal/platform/logging/redact.go) that is the sole thing keeping
// secret-looking attribute values out of the logs. Before this function
// existed, reverting any one call site
// to a bare handler compiled and passed the whole suite silently. Centralising the construction here means there is
// exactly one place to get this right for this package, but
// TestNewCommandLoggerRedactsSecretLookingAttributes only pins this
// function's own behaviour — it does not stop a call site from being
// reverted to a bare handler. TestOnlyLoggingPackageConstructsBareSlogHandlers
// (internal/arch/arch_test.go) is what pins it for every caller, repo-wide,
// by failing the build if any non-test file outside
// internal/platform/logging/logging.go constructs one of those handlers
// directly.
func newCommandLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	return logging.New(cfg.LogLevel, string(cfg.LogFormat), w)
}
