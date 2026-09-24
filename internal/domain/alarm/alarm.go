// Package alarm evaluates alarm rules (01 §7.12). It is pure: no repository,
// no clock, no I/O. Callers load readings, bills and timestamps and pass them
// in, which is what makes every acceptance criterion in 09 §F7 a table test.
//
// model.Alarm keeps one wide row with a nullable threshold group per type, so
// Type is the ONLY thing that says which fields are meaningful. Everything
// here switches on Type first, and Validate is what stops a field belonging to
// another type from being stored as if it meant something.
package alarm

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Field names used in validation codes and in an event's detail; they match
// the DTO's own JSON names so a 422 names the field the client sent.
const (
	FieldType           = "type"
	FieldAnalyzers      = "analyzer_ids"
	FieldSettings       = "settings"
	FieldFrequencyValue = "notification_frequency_value"

	FieldInductive  = "inductive_ratio_threshold"
	FieldCapacitive = "capacitive_ratio_threshold"
	FieldActiveMax  = "active_consumption_max"
	FieldActiveMin  = "active_consumption_min"

	FieldCommsHours = "communication_threshold_hours"

	FieldPowerMax = "power_max"
	FieldPowerMin = "power_min"
	// FieldVoltageMax and FieldVoltageMin exist only to be REJECTED (R212).
	FieldVoltageMax = "voltage_max"
	FieldVoltageMin = "voltage_min"

	FieldInvoicePct = "invoice_threshold_pct"
)

// Validation codes.
const (
	CodeRequired = "required"
	// CodeUnsupported is R212: voltage has no data source in any provider
	// payload, so a voltage threshold is refused rather than silently ignored.
	CodeUnsupported = "unsupported"
	CodeAtLeastOne  = "at_least_one"
	CodeRange       = "range"
)

// maxPeriodHours bounds every period and frequency: a year in hours. A window
// longer than that would read further back than any tenant's data goes.
const maxPeriodHours int32 = 8760

// ValidationError names the offending fields and their codes; the service
// turns it into perr.Validation.WithParams.
type ValidationError struct{ Fields map[string][]string }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("alarm: invalid settings: %v", e.Fields)
}

// Validate checks a rule's settings against its type (R212, R216, R230). It
// never looks at channels or recipients — the service owns those (R231).
func Validate(a model.Alarm, analyzerCount int) error {
	f := map[string][]string{}
	if analyzerCount < 1 {
		f[FieldAnalyzers] = []string{CodeRequired} // R230
	}
	validateFrequency(a, f)
	switch a.Type {
	case model.AlarmTypeReactiveLimit:
		validateReactive(a, f)
	case model.AlarmTypeDataCommunication:
		validateComms(a, f)
	case model.AlarmTypeCurrentVoltagePower:
		validatePower(a, f)
	case model.AlarmTypeInvoiceIncrease:
		validateInvoice(a, f)
	default:
		// Not vacuous: an unknown alarm_type would otherwise be storable with
		// no settings at all, and then evaluate to nothing forever.
		f[FieldType] = []string{CodeUnsupported}
	}
	if len(f) == 0 {
		return nil
	}
	return &ValidationError{Fields: f}
}

// validateFrequency: a frequency is optional, but half of one is a bug rather
// than a default — legacy stored the value and never read it.
func validateFrequency(a model.Alarm, f map[string][]string) {
	switch v, u := a.NotificationFrequencyValue, a.NotificationFrequencyUnit; {
	case (v == nil) != (u == nil):
		f[FieldFrequencyValue] = []string{CodeRequired}
	case v != nil && (*v < 1 || *v > maxPeriodHours):
		f[FieldFrequencyValue] = []string{CodeRange}
	case u != nil && !u.Valid():
		f[FieldFrequencyValue] = []string{CodeRange}
	}
}

// group is one reactive threshold with the period it is measured over.
type group struct {
	threshold *decimal.Decimal
	value     *int32
	unit      *model.PeriodUnit
}

// complete reports whether the group can be evaluated at all: a threshold
// without its period measures nothing (R230).
func (g group) complete() bool {
	return g.threshold != nil && g.value != nil && g.unit != nil &&
		*g.value >= 1 && *g.value <= maxPeriodHours && g.unit.Valid()
}

// groups returns the four reactive groups, so the validator and the evaluator
// can never disagree about what a group is.
func groups(a model.Alarm) map[string]group {
	return map[string]group{
		FieldInductive:  {a.InductiveRatioThreshold, a.InductivePeriodValue, a.InductivePeriodUnit},
		FieldCapacitive: {a.CapacitiveRatioThreshold, a.CapacitivePeriodValue, a.CapacitivePeriodUnit},
		FieldActiveMax:  {a.ActiveConsumptionMax, a.ActiveConsumptionMaxPeriodValue, a.ActiveConsumptionMaxPeriodUnit},
		FieldActiveMin:  {a.ActiveConsumptionMin, a.ActiveConsumptionMinPeriodValue, a.ActiveConsumptionMinPeriodUnit},
	}
}

func validateReactive(a model.Alarm, f map[string][]string) {
	for _, g := range groups(a) {
		if g.complete() {
			return
		}
	}
	f[FieldSettings] = []string{CodeAtLeastOne}
}

func validateComms(a model.Alarm, f map[string][]string) {
	switch h := a.CommunicationThresholdHours; {
	case h == nil:
		f[FieldCommsHours] = []string{CodeRequired}
	case *h < 1 || *h > maxPeriodHours:
		f[FieldCommsHours] = []string{CodeRange}
	}
}

func validatePower(a model.Alarm, f map[string][]string) {
	// R212: the columns stay (F1 transcribed 04 §7) but nothing may set them.
	if a.VoltageMax != nil {
		f[FieldVoltageMax] = []string{CodeUnsupported}
	}
	if a.VoltageMin != nil {
		f[FieldVoltageMin] = []string{CodeUnsupported}
	}
	if a.PowerMax == nil && a.PowerMin == nil {
		f[FieldSettings] = []string{CodeAtLeastOne}
	}
}

func validateInvoice(a model.Alarm, f map[string][]string) {
	switch p := a.InvoiceThresholdPct; {
	case p == nil:
		f[FieldInvoicePct] = []string{CodeRequired}
	case !p.IsPositive():
		f[FieldInvoicePct] = []string{CodeRange}
	}
}
