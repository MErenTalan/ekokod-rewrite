package alarm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func commsRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeDataCommunication, CommunicationThresholdHours: i32(6)}
}

func TestCommsFiresWhenLastReadingIsOlderThanThreshold(t *testing.T) {
	t.Parallel()
	last := base.Add(-7 * time.Hour)
	v := alarm.EvaluateComms(commsRule(), uuid.New(), &last, base)
	require.True(t, v.Fired())
	require.Equal(t, alarm.FieldCommsHours, v.Breaches[0].Field)
	require.Equal(t, "7", v.Breaches[0].Measured.String())
	require.Equal(t, "6", v.Breaches[0].Threshold.String())
	require.Equal(t, last.Format(time.RFC3339), v.Breaches[0].Extra["last_reading_at"])
}

func TestCommsStaysSilentInsideThreshold(t *testing.T) {
	t.Parallel()
	last := base.Add(-5 * time.Hour)
	v := alarm.EvaluateComms(commsRule(), uuid.New(), &last, base)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestCommsExactlyAtThresholdStaysSilent(t *testing.T) {
	t.Parallel()
	// "older than this window" is strict, as legacy's diffHours > threshold was.
	last := base.Add(-6 * time.Hour)
	require.False(t, alarm.EvaluateComms(commsRule(), uuid.New(), &last, base).Fired())
}

func TestCommsNeverReportedIsNoVerdict(t *testing.T) {
	t.Parallel()
	// R216: a meter added five minutes ago has not lost communication.
	v := alarm.EvaluateComms(commsRule(), uuid.New(), nil, base)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldCommsHours}, v.NoVerdict)
}

func powerRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeCurrentVoltagePower, PowerMax: dec("100"), PowerMin: dec("5")}
}

func withDemand(kw string) *model.MeterReading {
	d := decimal.RequireFromString(kw)
	return &model.MeterReading{Ts: base, Kind: model.ReadingKindLoadProfile, MaxDemandKw: &d}
}

func TestPowerFiresOverMaxAndUnderMin(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	over := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("120.5"), w)
	require.Len(t, over.Breaches, 1)
	require.Equal(t, alarm.FieldPowerMax, over.Breaches[0].Field)
	require.Equal(t, "120.5", over.Breaches[0].Measured.String())
	require.Equal(t, w, over.Breaches[0].Window)

	under := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("2"), w)
	require.Len(t, under.Breaches, 1)
	require.Equal(t, alarm.FieldPowerMin, under.Breaches[0].Field)
}

func TestPowerInsideBoundsStaysSilent(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	v := alarm.EvaluatePower(powerRule(), uuid.New(), withDemand("50"), w)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestPowerNoReadingOrNoDemandIsNoVerdict(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base, 48, model.PeriodUnitHours)
	// R214: nothing reported in the 48-hour lookback.
	require.Equal(t, []string{alarm.FieldPowerMax, alarm.FieldPowerMin},
		alarm.EvaluatePower(powerRule(), uuid.New(), nil, w).NoVerdict)
	// A reading that carries no max demand is not 0 kW — which would otherwise
	// fire the minimum bound on every meter that does not report demand.
	require.Equal(t, []string{alarm.FieldPowerMax, alarm.FieldPowerMin},
		alarm.EvaluatePower(powerRule(), uuid.New(),
			&model.MeterReading{Ts: base, Kind: model.ReadingKindLoadProfile}, w).NoVerdict)
}

func TestPowerNeverReadsVoltage(t *testing.T) {
	t.Parallel()
	// R212: even a stored voltage threshold produces no breach — the rule was
	// validated away, and the evaluator has no voltage input to compare.
	a := powerRule()
	a.VoltageMax, a.VoltageMin = dec("1"), dec("100000")
	v := alarm.EvaluatePower(a, uuid.New(), withDemand("50"), alarm.NewWindow(base, 48, model.PeriodUnitHours))
	require.False(t, v.Fired())
	for _, b := range v.Breaches {
		require.NotContains(t, []string{alarm.FieldVoltageMax, alarm.FieldVoltageMin}, b.Field)
	}
}

func bill(period, total string) *model.Bill {
	return &model.Bill{ID: uuid.New(), Scope: model.BillScopeAnalyzer, PeriodKey: period,
		TotalCost: decimal.RequireFromString(total)}
}

func invoiceRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeInvoiceIncrease, InvoiceThresholdPct: dec("20")}
}

func TestInvoiceFiresAboveThreshold(t *testing.T) {
	t.Parallel()
	latest := bill("2026-08", "1300")
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), latest, bill("2026-07", "1000"))
	require.True(t, v.Fired())
	require.Equal(t, alarm.FieldInvoicePct, v.Breaches[0].Field)
	require.Equal(t, "30", v.Breaches[0].Measured.String())
	require.Equal(t, "2026-08", v.Breaches[0].Extra["period_key"])
	require.Equal(t, latest.ID.String(), v.Breaches[0].Extra["bill_id"])
}

func TestInvoiceStaysSilentAtOrBelowThreshold(t *testing.T) {
	t.Parallel()
	require.False(t, alarm.EvaluateInvoice(invoiceRule(), uuid.New(),
		bill("2026-08", "1200"), bill("2026-07", "1000")).Fired())
}

func TestInvoiceFallingBillNeverFires(t *testing.T) {
	t.Parallel()
	// A cheaper invoice is a negative percentage, never a breach.
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "500"), bill("2026-07", "1000"))
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestInvoiceZeroPreviousIsNoVerdict(t *testing.T) {
	t.Parallel()
	// No percentage exists against zero, and treating it as an infinite
	// increase would fire on every first invoice after an idle period.
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "1300"), bill("2026-07", "0"))
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInvoicePct}, v.NoVerdict)
}

func TestInvoiceFewerThanTwoBillsIsNoVerdict(t *testing.T) {
	t.Parallel()
	v := alarm.EvaluateInvoice(invoiceRule(), uuid.New(), bill("2026-08", "1300"), nil)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInvoicePct}, v.NoVerdict)
}
