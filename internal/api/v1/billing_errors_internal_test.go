package v1

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	domaintariff "github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func TestComputeErrorMapping(t *testing.T) {
	cases := map[string]int{
		"tariff_not_found": 422, "no_consumption_data": 422, "unresolved_anomaly": 409, "period_not_closed": 409,
		"ptf_data_missing": 422, "billing_parameters_missing": 500, "billing_parameters_invalid": 500,
	}
	for code, status := range cases {
		err := billingErr(fmt.Errorf("wrapped: %w", &billingsvc.ComputeError{Code: code, Detail: map[string]string{"period": "2026-08"}}))
		var e *perr.Error
		require.ErrorAs(t, err, &e, code)
		require.Equal(t, code, e.Code)
		require.Equal(t, status, e.HTTPStatus, code)
		require.Equal(t, map[string]any{"period": "2026-08"}, e.Params)
	}

	var e *perr.Error
	require.ErrorAs(t, billingErr(&domaintariff.ValidationError{Fields: map[string]string{"taxes[0].rate": "negative"}}), &e)
	require.Equal(t, "validation_failed", e.Code)
	require.Equal(t, map[string]any{"taxes[0].rate": []string{"negative"}}, e.Params)

	for _, err := range []error{billingsvc.ErrInvalidRequest, tariffsvc.ErrInvalidRequest} {
		require.ErrorAs(t, billingErr(err), &e)
		require.Equal(t, 400, e.HTTPStatus)
	}
	require.True(t, errors.Is(billingErr(store.ErrNotFound), store.ErrNotFound), "404 stays the store sentinel")
}
