package energy

import "time"

// ActivityWindow is §3.6's fixed 7 days. It is deliberately independent of
// two other, differently-scoped notions of "active" (R66): it is NOT
// analyzers.is_active, which records operator enablement of an analyzer,
// and it is NOT the configurable data-communication alarm threshold F7
// introduces. All three answer a different question; this constant answers
// only "did this metering point report recently".
const ActivityWindow = 7 * 24 * time.Hour

// Status is a metering point's §3.6 activity status.
type Status string

// The two activity statuses (02 §3.6).
const (
	StatusActive  Status = "active"
	StatusPassive Status = "passive"
)

// ActivityStatus reports §3.6's status: active iff lastReadingAt is non-nil
// and now.Sub(*lastReadingAt) <= ActivityWindow. The boundary is chosen
// inclusive — a last reading exactly ActivityWindow ago is still active,
// matching "within the last 7 days" read as a closed interval. A nil last
// reading (no reading has ever arrived) is passive.
func ActivityStatus(lastReadingAt *time.Time, now time.Time) Status {
	if lastReadingAt == nil {
		return StatusPassive
	}
	if now.Sub(*lastReadingAt) <= ActivityWindow {
		return StatusActive
	}
	return StatusPassive
}
