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

var base = time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)

// reading builds a load-profile row with the three registers the reactive
// groups difference.
func reading(h int, active, inductive, capacitive string) model.MeterReading {
	a := decimal.RequireFromString(active)
	i := decimal.RequireFromString(inductive)
	c := decimal.RequireFromString(capacitive)
	return model.MeterReading{Ts: base.Add(time.Duration(h) * time.Hour), Kind: model.ReadingKindLoadProfile,
		ActiveImport: &a, ReactiveInductiveImport: &i, ReactiveCapacitiveImport: &c}
}

func reactiveRule() model.Alarm {
	return model.Alarm{Type: model.AlarmTypeReactiveLimit, InductiveRatioThreshold: dec("20"),
		InductivePeriodValue: i32(24), InductivePeriodUnit: unit(model.PeriodUnitHours)}
}

func TestReactiveFiresWhenInductiveRatioExceeds(t *testing.T) {
	t.Parallel()
	// 100 kWh active, 25 kVarh inductive → 25% > 20%.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.True(t, v.Fired())
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldInductive, v.Breaches[0].Field)
	require.Equal(t, "25", v.Breaches[0].Measured.String())
	require.Equal(t, "20", v.Breaches[0].Threshold.String())
}

func TestReactiveStaysSilentAtOrBelowThreshold(t *testing.T) {
	t.Parallel()
	// Exactly 20% is not "> 20": legacy used a strict comparison.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "120", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestReactiveNonPositiveActiveDiffYieldsZeroRatio(t *testing.T) {
	t.Parallel()
	// R215, legacy: a flat or falling active register gives ratio 0, not a
	// division by zero and not a breach.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1000", "500", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Empty(t, v.NoVerdict)
}

func TestReactiveFewerThanTwoReadingsIsNoVerdict(t *testing.T) {
	t.Parallel()
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInductive}, v.NoVerdict)
}

func TestReactiveCapacitiveUsesItsOwnRegister(t *testing.T) {
	t.Parallel()
	// A capacitive rule must not fire on the inductive register.
	a := model.Alarm{Type: model.AlarmTypeReactiveLimit, CapacitiveRatioThreshold: dec("10"),
		CapacitivePeriodValue: i32(24), CapacitivePeriodUnit: unit(model.PeriodUnitHours)}
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Capacitive: []model.MeterReading{reading(0, "1000", "100", "5"), reading(24, "1100", "900", "20")}}
	v := alarm.EvaluateReactive(a, in)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldCapacitive, v.Breaches[0].Field)
	require.Equal(t, "15", v.Breaches[0].Measured.String())
}

func TestReactiveActiveConsumptionMaxAndMin(t *testing.T) {
	t.Parallel()
	a := model.Alarm{Type: model.AlarmTypeReactiveLimit,
		ActiveConsumptionMax: dec("50"), ActiveConsumptionMaxPeriodValue: i32(24), ActiveConsumptionMaxPeriodUnit: unit(model.PeriodUnitHours),
		ActiveConsumptionMin: dec("10"), ActiveConsumptionMinPeriodValue: i32(24), ActiveConsumptionMinPeriodUnit: unit(model.PeriodUnitHours)}

	over := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		ActiveMax: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1100", "0", "0")},
		ActiveMin: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1100", "0", "0")}}
	v := alarm.EvaluateReactive(a, over)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldActiveMax, v.Breaches[0].Field)
	require.Equal(t, "100", v.Breaches[0].Measured.String())

	under := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		ActiveMax: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1005", "0", "0")},
		ActiveMin: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "1005", "0", "0")}}
	v = alarm.EvaluateReactive(a, under)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldActiveMin, v.Breaches[0].Field)
}

func TestReactiveMissingRegisterIsNoVerdictNotZero(t *testing.T) {
	t.Parallel()
	// A nil register is unknown, not zero: a nil inductive reading must never
	// be read as "0 kVarh, therefore compliant".
	first, last := reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")
	last.ReactiveInductiveImport = nil
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(), Inductive: []model.MeterReading{first, last}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.False(t, v.Fired())
	require.Equal(t, []string{alarm.FieldInductive}, v.NoVerdict)
}

func TestReactiveCarriesTheWindowItMeasured(t *testing.T) {
	t.Parallel()
	w := alarm.NewWindow(base.Add(24*time.Hour), 24, model.PeriodUnitHours)
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "125", "0")},
		Windows:   map[string]alarm.Window{alarm.FieldInductive: w}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.Equal(t, w, v.Breaches[0].Window)
}

func TestReactiveIgnoresAGroupWithNoThreshold(t *testing.T) {
	t.Parallel()
	// Readings supplied for a group the rule does not configure are not a
	// breach and not a no-verdict: the group simply is not evaluated.
	in := alarm.ReactiveInput{AnalyzerID: uuid.New(),
		Inductive: []model.MeterReading{reading(0, "1000", "100", "0"), reading(24, "1100", "900", "0")},
		ActiveMax: []model.MeterReading{reading(0, "1000", "0", "0"), reading(24, "9999", "0", "0")}}
	v := alarm.EvaluateReactive(reactiveRule(), in)
	require.Len(t, v.Breaches, 1)
	require.Equal(t, alarm.FieldInductive, v.Breaches[0].Field)
}

func TestNewWindowUnits(t *testing.T) {
	t.Parallel()
	require.Equal(t, base.Add(-24*time.Hour), alarm.NewWindow(base, 24, model.PeriodUnitHours).From)
	require.Equal(t, base.AddDate(0, 0, -3), alarm.NewWindow(base, 3, model.PeriodUnitDays).From)
	require.Equal(t, base, alarm.NewWindow(base, 3, model.PeriodUnitDays).To)
	// An unknown unit is read as hours rather than producing an empty window,
	// which would silently evaluate nothing.
	require.Equal(t, base.Add(-5*time.Hour), alarm.NewWindow(base, 5, model.PeriodUnit("weeks")).From)
}
