package alarms

import "github.com/MErenTalan/ekokod-rewrite/internal/domain/model"

// SettingsFor exposes settingsFor to the package's external test.
//
// It is tested directly because no route through Create can reach it with a
// voltage threshold set on a power rule: alarm.Validate rejects that first.
// settingsFor is the SECOND line of defence for R212 — the one that holds if
// validation is ever loosened — and a second line nothing exercises is not a
// defence at all.
func SettingsFor(in Input) model.Alarm { return settingsFor(in) }
