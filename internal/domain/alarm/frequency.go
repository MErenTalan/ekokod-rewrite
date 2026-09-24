package alarm

import (
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Suppressed reports whether a notification is suppressed because one was
// already delivered inside the frequency window (R218).
//
// The frequency limits the NOTIFICATION, never the event: an alarm that
// breaches always records a row, so the log stays honest about what happened
// even when nobody was told again.
//
// The window is half-open, so a notification exactly one period old has aged
// out. A rule that configures no frequency — or, defensively, half of one —
// never suppresses: legacy stored the value and never read it, and a
// unit-less value must not start silently swallowing notifications now.
func Suppressed(a model.Alarm, lastNotifiedAt *time.Time, now time.Time) bool {
	if lastNotifiedAt == nil || a.NotificationFrequencyValue == nil || a.NotificationFrequencyUnit == nil {
		return false
	}
	w := NewWindow(now, *a.NotificationFrequencyValue, *a.NotificationFrequencyUnit)
	return lastNotifiedAt.After(w.From)
}
