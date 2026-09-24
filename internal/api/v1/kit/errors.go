// Package kit is the /api/v1 handler toolkit: binding, the error envelope,
// localised messages, pagination and JSON responses.
package kit

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Request errors the binder and middleware raise (R153, R154).
var (
	ErrUnknownParameter  = perr.New("unknown_parameter", http.StatusBadRequest, "errors.api.unknownParameter")
	ErrInvalidParameters = perr.New("invalid_parameters", http.StatusBadRequest, "errors.api.invalidParameters")
	ErrInvalidBody       = perr.New("invalid_body", http.StatusBadRequest, "errors.api.invalidBody")
	ErrBodyTooLarge      = perr.New("body_too_large", http.StatusRequestEntityTooLarge, "errors.api.bodyTooLarge")
	ErrUnsupportedMedia  = perr.New("unsupported_media_type", http.StatusUnsupportedMediaType, "errors.api.unsupportedMediaType")
	ErrInvalidLimit      = perr.New("invalid_limit", http.StatusBadRequest, "errors.api.invalidLimit")
	ErrInvalidCursor     = perr.New("invalid_cursor", http.StatusBadRequest, "errors.api.invalidCursor")
	ErrRateLimited       = perr.New("rate_limited", http.StatusTooManyRequests, "errors.generic.rateLimited")
	ErrMethodNotAllowed  = perr.New("method_not_allowed", http.StatusMethodNotAllowed, "errors.api.methodNotAllowed")
	ErrIdempotencyReuse  = perr.New("idempotency_key_mismatch", http.StatusUnprocessableEntity, "errors.api.idempotencyMismatch")
	ErrIdempotencyBusy   = perr.New("idempotency_in_progress", http.StatusConflict, "errors.api.idempotencyInProgress")
)

// Logger is where unexpected errors are reported; set once by the router.
var Logger = slog.New(slog.DiscardHandler)

// WriteError renders err as the envelope. Store sentinels map to 404/409; any
// error that is not a *perr.Error becomes a 500 whose cause is logged, never sent.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *perr.Error
	switch {
	case errors.As(err, &e):
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrInvalidScope):
		e = perr.NotFound
	case errors.Is(err, store.ErrConflict):
		e = perr.Conflict
	default:
		Logger.ErrorContext(r.Context(), "api: unexpected error", append([]any{slog.String("error", err.Error()),
			slog.String("path", r.URL.Path)}, attrs(r)...)...)
		e = perr.Internal
	}
	if e.HTTPStatus >= 500 && e.Cause != nil {
		Logger.ErrorContext(r.Context(), "api: server error", slog.String("code", e.Code), slog.String("error", e.Cause.Error()))
	}
	body := dto.Error{Error: dto.ErrorBody{
		Code: e.Code, Message: Message(e.Code, Locale(r)), Details: e.Params,
		RequestID: w.Header().Get(middleware.HeaderRequestID),
	}}
	WriteJSON(w, e.HTTPStatus, body)
}

func attrs(r *http.Request) []any {
	out := []any{}
	for _, a := range logging.Fields(r.Context()) {
		out = append(out, a)
	}
	return out
}

// WriteJSON writes v with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }
