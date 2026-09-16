package billing

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrInvalidRequest is a malformed request, rejected before any I/O.
var ErrInvalidRequest = errors.New("billing: invalid request")

// R113 codes: failures that persist no bill.
const (
	CodeTariffNotFound           = "tariff_not_found"
	CodeNoConsumptionData        = "no_consumption_data"
	CodeUnresolvedAnomaly        = "unresolved_anomaly"
	CodePeriodNotClosed          = "period_not_closed"
	CodeBillingParametersMissing = "billing_parameters_missing"
	CodeBillingParametersInvalid = "billing_parameters_invalid" // M-6
)

// ComputeError is a data condition that keeps a bill from being computed.
type ComputeError struct {
	Code   string
	Detail map[string]string
}

func (e *ComputeError) Error() string {
	keys := make([]string, 0, len(e.Detail))
	for k := range e.Detail {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%s", k, e.Detail[k])
	}
	return fmt.Sprintf("billing: %s (%s)", e.Code, strings.Join(parts, ", "))
}

func computeErr(code string, kv ...string) *ComputeError {
	e := &ComputeError{Code: code, Detail: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		e.Detail[kv[i]] = kv[i+1]
	}
	return e
}
