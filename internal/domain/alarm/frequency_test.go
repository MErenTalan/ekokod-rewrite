package alarm_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func freqRule(value int32, u model.PeriodUnit) model.Alarm {
	return model.Alarm{NotificationFrequencyValue: i32(value), NotificationFrequencyUnit: unit(u)}
}

func TestSuppressedInsideWindowAllowedAfterIt(t *testing.T) {
	t.Parallel()
	a := freqRule(6, model.PeriodUnitHours)
	inside := base.Add(-5 * time.Hour)
	require.True(t, alarm.Suppressed(a, &inside, base))
	outside := base.Add(-7 * time.Hour)
	require.False(t, alarm.Suppressed(a, &outside, base))
}

func TestSuppressedExactlyAtWindowEdgeAllows(t *testing.T) {
	t.Parallel()
	// The window is half-open: a notification exactly one period old has aged out.
	edge := base.Add(-6 * time.Hour)
	require.False(t, alarm.Suppressed(freqRule(6, model.PeriodUnitHours), &edge, base))
}

func TestNoFrequencyNeverSuppresses(t *testing.T) {
	t.Parallel()
	// Legacy stored the frequency and never read it; a rule that configures
	// none keeps that behaviour and notifies every firing.
	recent := base.Add(-1 * time.Minute)
	require.False(t, alarm.Suppressed(model.Alarm{}, &recent, base))
}

func TestHalfAFrequencyNeverSuppresses(t *testing.T) {
	t.Parallel()
	// Validate refuses this shape, but a row written before it existed must not
	// silently suppress on a unit-less value.
	a := model.Alarm{NotificationFrequencyValue: i32(6)}
	recent := base.Add(-1 * time.Minute)
	require.False(t, alarm.Suppressed(a, &recent, base))
}

func TestNoPreviousNotificationNeverSuppresses(t *testing.T) {
	t.Parallel()
	require.False(t, alarm.Suppressed(freqRule(30, model.PeriodUnitDays), nil, base))
}

func TestSuppressedCountsDaysAsCalendarDays(t *testing.T) {
	t.Parallel()
	a := freqRule(2, model.PeriodUnitDays)
	inside := base.AddDate(0, 0, -1)
	require.True(t, alarm.Suppressed(a, &inside, base))
	outside := base.AddDate(0, 0, -3)
	require.False(t, alarm.Suppressed(a, &outside, base))
}
