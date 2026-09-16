package energy_test

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// t0 is an arbitrary, fixed instant every test in this package anchors its
// windows and readings to.
var t0 = time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

// dec parses a decimal literal into a *decimal.Decimal, failing the test
// binary at init-adjacent call time (via decimal.RequireFromString) if the
// literal is malformed — every literal in this package's tests is a fixed
// constant, so a parse failure here is a typo in the test, not a runtime
// condition to handle gracefully.
func dec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// readingAt builds a load-profile reading at ts with active_import set to
// activeImport and every other register absent.
func readingAt(ts time.Time, activeImport string) *energy.Reading {
	return &energy.Reading{
		TS:   ts,
		Kind: energy.KindLoadProfile,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport: dec(activeImport),
		},
	}
}
