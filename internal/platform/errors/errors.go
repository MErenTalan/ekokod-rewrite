// Package errors defines the single application error type: a machine code, an
// HTTP status, a localisable message key and a wrapped cause. Handlers map it
// to a response; jobs map it to an operational message. Nothing is swallowed.
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// Error is the application error type.
type Error struct {
	Code       string
	HTTPStatus int
	MessageKey string
	Params     map[string]any
	Cause      error
}

// New creates an Error with no cause.
func New(code string, status int, messageKey string) *Error {
	return &Error{Code: code, HTTPStatus: status, MessageKey: messageKey}
}

// Wrap creates an Error carrying cause.
func Wrap(cause error, code string, status int, messageKey string) *Error {
	return &Error{Code: code, HTTPStatus: status, MessageKey: messageKey, Cause: cause}
}

// WithParams attaches parameters for message interpolation.
func (e *Error) WithParams(params map[string]any) *Error {
	clone := *e
	clone.Params = params
	return &clone
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// Is matches another *Error by code, so sentinels work with errors.Is.
func (e *Error) Is(target error) bool {
	var t *Error
	if !stderrors.As(target, &t) {
		return false
	}
	return t.Code == e.Code
}

// Sentinels. Use them as prototypes: New(NotFound.Code, NotFound.HTTPStatus, key).
var (
	NotFound     = New("not_found", http.StatusNotFound, "errors.generic.notFound")
	Unauthorized = New("unauthorized", http.StatusUnauthorized, "errors.generic.unauthorized")
	Forbidden    = New("forbidden", http.StatusForbidden, "errors.generic.forbidden")
	Validation   = New("validation_failed", http.StatusUnprocessableEntity, "errors.generic.validation")
	Conflict     = New("conflict", http.StatusConflict, "errors.generic.conflict")
	Unavailable  = New("unavailable", http.StatusServiceUnavailable, "errors.generic.unavailable")
	Internal     = New("internal", http.StatusInternalServerError, "errors.generic.internal")
)

func as(err error) *Error {
	var e *Error
	if stderrors.As(err, &e) {
		return e
	}
	return Internal
}

// CodeOf returns the machine code, defaulting to "internal".
func CodeOf(err error) string { return as(err).Code }

// StatusOf returns the HTTP status, defaulting to 500.
func StatusOf(err error) int { return as(err).HTTPStatus }

// MessageKeyOf returns the i18n key, defaulting to the generic internal key.
func MessageKeyOf(err error) string { return as(err).MessageKey }
