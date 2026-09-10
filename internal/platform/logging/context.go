package logging

import (
	"context"
	"log/slog"
)

// Field names propagated through context into every log record.
const (
	FieldRequestID = "request_id"
	FieldPrincipal = "principal"
	FieldCompany   = "company_id"
	FieldJob       = "job_id"
)

type fieldsKey struct{}

// WithField returns a context carrying an additional log field.
func WithField(ctx context.Context, key, value string) context.Context {
	existing := Fields(ctx)
	next := make([]slog.Attr, len(existing), len(existing)+1)
	copy(next, existing)
	next = append(next, slog.String(key, value))
	return context.WithValue(ctx, fieldsKey{}, next)
}

// Fields returns the log fields carried by ctx.
func Fields(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	fields, _ := ctx.Value(fieldsKey{}).([]slog.Attr)
	return fields
}
