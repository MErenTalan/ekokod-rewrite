package errors_test

import (
	stderrors "errors"
	"net/http"
	"testing"

	apperrors "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/stretchr/testify/require"
)

func TestErrorCarriesCodeStatusAndMessageKey(t *testing.T) {
	err := apperrors.New("tariff_not_found", http.StatusNotFound, "errors.tariff.notFound").
		WithParams(map[string]any{"buildingId": "b-1"})

	require.Equal(t, "tariff_not_found", apperrors.CodeOf(err))
	require.Equal(t, http.StatusNotFound, apperrors.StatusOf(err))
	require.Equal(t, "errors.tariff.notFound", apperrors.MessageKeyOf(err))
	require.Contains(t, err.Error(), "tariff_not_found")
}

func TestWrapPreservesCause(t *testing.T) {
	cause := stderrors.New("connection refused")
	err := apperrors.Wrap(cause, "db_unavailable", http.StatusServiceUnavailable, "errors.db.unavailable")

	require.ErrorIs(t, err, cause)
	require.Equal(t, "db_unavailable", apperrors.CodeOf(err))
	require.Contains(t, err.Error(), "connection refused")
}

func TestSentinelsMatchByCode(t *testing.T) {
	err := apperrors.New(apperrors.NotFound.Code, http.StatusNotFound, "errors.generic.notFound")
	require.ErrorIs(t, err, apperrors.NotFound)
	require.NotErrorIs(t, err, apperrors.Forbidden)
}

func TestUnknownErrorDefaultsToInternal(t *testing.T) {
	err := stderrors.New("boom")
	require.Equal(t, "internal", apperrors.CodeOf(err))
	require.Equal(t, http.StatusInternalServerError, apperrors.StatusOf(err))
	require.Equal(t, "errors.generic.internal", apperrors.MessageKeyOf(err))
}
