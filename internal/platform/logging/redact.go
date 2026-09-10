package logging

import (
	"context"
	"log/slog"
	"strings"
)

// Redacted replaces the value of any attribute whose key looks like a secret.
const Redacted = "[REDACTED]"

// secretKeyParts are matched case-insensitively as substrings of an attribute key.
var secretKeyParts = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey",
	"authorization", "credential", "pepper", "private_key", "encryption_key",
	"signing_key", "session", "cookie", "tgt", "ticket",
}

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range secretKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// redactHandler wraps a handler, redacting secret-looking attributes and
// appending the context fields carried by WithField.
type redactHandler struct{ inner slog.Handler }

func (h redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactHandler{inner: h.inner.WithAttrs(redactAttrs(attrs))}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{inner: h.inner.WithGroup(name)}
}

func (h redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	out.AddAttrs(Fields(ctx)...)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, redactAttr(a))
	}
	return out
}

func redactAttr(a slog.Attr) slog.Attr {
	// Resolve slog.LogValuer values before inspecting them, so a secret
	// hidden behind a LogValue() method (common on integration client
	// config/credential structs) is redacted rather than deferred to the
	// inner handler, which would resolve it after redaction has already run.
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redactAttrs(a.Value.Group())...)}
	}
	if isSecretKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	return a
}
