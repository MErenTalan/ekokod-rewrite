package alarm_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func dec(s string) *decimal.Decimal             { d := decimal.RequireFromString(s); return &d }
func i32(v int32) *int32                        { return &v }
func unit(u model.PeriodUnit) *model.PeriodUnit { return &u }

func fields(t *testing.T, err error) map[string][]string {
	t.Helper()
	var ve *alarm.ValidationError
	require.ErrorAs(t, err, &ve)
	return ve.Fields
}

func TestValidateRequiresAnAnalyzer(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6)}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 0))[alarm.FieldAnalyzers])
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateReactiveNeedsOneCompleteGroup(t *testing.T) {
	t.Parallel()
	// R230: a threshold without its period is not a group.
	bare := model.Alarm{Type: model.AlarmTypeReactiveLimit, InductiveRatioThreshold: dec("20")}
	require.Equal(t, []string{alarm.CodeAtLeastOne},
		fields(t, alarm.Validate(bare, 1))[alarm.FieldSettings])

	full := bare
	full.InductivePeriodValue, full.InductivePeriodUnit = i32(24), unit(model.PeriodUnitHours)
	require.NoError(t, alarm.Validate(full, 1))
}

func TestValidateRejectsVoltage(t *testing.T) {
	t.Parallel()
	// R212: no provider reports voltage; legacy read a field that never existed.
	a := model.Alarm{Type: model.AlarmTypeCurrentVoltagePower, PowerMax: dec("100"), VoltageMax: dec("400")}
	require.Equal(t, []string{alarm.CodeUnsupported},
		fields(t, alarm.Validate(a, 1))[alarm.FieldVoltageMax])

	a.VoltageMax = nil
	require.NoError(t, alarm.Validate(a, 1))

	a.VoltageMin = dec("180")
	require.Equal(t, []string{alarm.CodeUnsupported},
		fields(t, alarm.Validate(a, 1))[alarm.FieldVoltageMin])
}

func TestValidatePowerNeedsOneBound(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeCurrentVoltagePower}
	require.Equal(t, []string{alarm.CodeAtLeastOne},
		fields(t, alarm.Validate(a, 1))[alarm.FieldSettings])
}

func TestValidateCommsHoursRequiredAndBounded(t *testing.T) {
	t.Parallel()
	// R216: legacy's silent default of 6 hours is gone.
	a := model.Alarm{Type: model.AlarmTypeDataCommunication}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldCommsHours])

	a.CommunicationThresholdHours = i32(8761)
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldCommsHours])

	a.CommunicationThresholdHours = i32(0)
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldCommsHours])
}

func TestValidateInvoicePctPositive(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeInvoiceIncrease}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldInvoicePct])

	a.InvoiceThresholdPct = dec("0")
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldInvoicePct])

	a.InvoiceThresholdPct = dec("20")
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateIgnoresOtherTypesSettings(t *testing.T) {
	t.Parallel()
	// model.Alarm's own warning: Type is the ONLY thing that says which fields
	// are meaningful, so a stray field from another type is not an error —
	// the service clears it. This pins that Validate does not reject it.
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6),
		InvoiceThresholdPct: dec("20")}
	require.NoError(t, alarm.Validate(a, 1))
}

func TestValidateFrequencyNeedsBothOrNeither(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6),
		NotificationFrequencyValue: i32(6)}
	require.Equal(t, []string{alarm.CodeRequired},
		fields(t, alarm.Validate(a, 1))[alarm.FieldFrequencyValue])

	a.NotificationFrequencyUnit = unit(model.PeriodUnitHours)
	require.NoError(t, alarm.Validate(a, 1))

	a.NotificationFrequencyValue = i32(8761)
	require.Equal(t, []string{alarm.CodeRange},
		fields(t, alarm.Validate(a, 1))[alarm.FieldFrequencyValue])
}

func TestValidateUnknownTypeIsRejected(t *testing.T) {
	t.Parallel()
	// A type the switch does not know must not validate vacuously: an unknown
	// alarm_type would otherwise be storable with no settings at all.
	require.Equal(t, []string{alarm.CodeUnsupported},
		fields(t, alarm.Validate(model.Alarm{Type: model.AlarmType("whatever")}, 1))[alarm.FieldType])
}
