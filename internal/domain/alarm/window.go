package alarm

import (
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Window is the half-open lookback [From, To) a group is measured over. It
// mirrors store.TimeRange's shape without importing it: this package imports
// nothing from the project but internal/domain/model.
type Window struct{ From, To time.Time }

// NewWindow builds the lookback ending at now. Days are calendar days in the
// instant's own location, so a 3-day window spanning a DST change is still
// three days rather than 71 or 73 hours.
//
// An unrecognised unit is read as hours: period_unit is a SQL enum with two
// values, and answering an impossible third with an empty window would
// silently evaluate nothing at all.
func NewWindow(now time.Time, value int32, u model.PeriodUnit) Window {
	if u == model.PeriodUnitDays {
		return Window{From: now.AddDate(0, 0, -int(value)), To: now}
	}
	return Window{From: now.Add(-time.Duration(value) * time.Hour), To: now}
}
