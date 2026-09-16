package consumption

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// Summary totals a series (05 §5 /consumption/summary). Every field is nil
// when no row contributed one; a total is never a zero standing in for "no
// data".
type Summary struct {
	Totals      map[energy.Register]*decimal.Decimal
	Averages    map[energy.Register]*decimal.Decimal
	Peak        *Row // the row with the highest active-import consumption
	Valley      *Row // the lowest non-nil one
	MaxDemandKw *decimal.Decimal
	Rows        int
	Suspect     int // rows carrying at least one suspect register
}

// Summarise totals rows the caller already fetched from EITHER path (Task 7's
// Analytics.Consumption or Billing.Consumption). A register reported only by
// suspect rows has a nil total (never a zero standing in for "no data"):
// a row "carries" a register for Totals/Averages only when its Values entry
// is non-nil AND the register is not present in that row's Suspect map.
// Averages divides by the number of rows that contributed a value for that
// register — never by len(rows) — using DivRound(…, energy.DivisionScale).
//
// Peak and Valley consider active_import only, ignoring any row where it is
// nil or suspect; on a tie the earliest window wins. Both fields point at
// COPIES of the winning row — not at an element of rows — so a caller
// mutating *s.Peak or *s.Valley can never corrupt the input slice.
//
// MaxDemandKw is the maximum over every row's non-nil MaxDemandKw field
// (MaxDemandKw is not a register and is never marked suspect).
func Summarise(rows []Row) Summary {
	registers := energy.AllRegisters()

	sums := make(map[energy.Register]decimal.Decimal, len(registers))
	counts := make(map[energy.Register]int, len(registers))
	carried := make(map[energy.Register]bool, len(registers))

	var maxDemand *decimal.Decimal
	var peak, valley *Row
	suspectRows := 0

	for i := range rows {
		row := &rows[i]

		if len(row.Suspect) > 0 {
			suspectRows++
		}

		for _, reg := range registers {
			v := row.Values[reg]
			if v == nil {
				continue
			}
			if _, suspect := row.Suspect[reg]; suspect {
				continue
			}
			if carried[reg] {
				sums[reg] = sums[reg].Add(*v)
			} else {
				sums[reg] = *v
				carried[reg] = true
			}
			counts[reg]++
		}

		if row.MaxDemandKw != nil && (maxDemand == nil || row.MaxDemandKw.GreaterThan(*maxDemand)) {
			v := *row.MaxDemandKw
			maxDemand = &v
		}

		active := row.Values[energy.ActiveImport]
		if active == nil {
			continue
		}
		if _, suspect := row.Suspect[energy.ActiveImport]; suspect {
			continue
		}
		if isNewExtreme(peak, active, row, true) {
			c := cloneRow(row)
			peak = &c
		}
		if isNewExtreme(valley, active, row, false) {
			c := cloneRow(row)
			valley = &c
		}
	}

	totals := make(map[energy.Register]*decimal.Decimal, len(registers))
	averages := make(map[energy.Register]*decimal.Decimal, len(registers))
	for _, reg := range registers {
		if !carried[reg] {
			totals[reg] = nil
			averages[reg] = nil
			continue
		}
		sum := sums[reg]
		totals[reg] = &sum
		avg := sum.DivRound(decimal.NewFromInt(int64(counts[reg])), energy.DivisionScale)
		averages[reg] = &avg
	}

	return Summary{
		Totals:      totals,
		Averages:    averages,
		Peak:        peak,
		Valley:      valley,
		MaxDemandKw: maxDemand,
		Rows:        len(rows),
		Suspect:     suspectRows,
	}
}

// isNewExtreme reports whether candidate (with active-import value v) should
// replace current as the running peak (wantHigher true) or valley
// (wantHigher false): strictly beyond current's value, or tied on value with
// an earlier Window.From (the earliest window wins a tie).
func isNewExtreme(current *Row, v *decimal.Decimal, candidate *Row, wantHigher bool) bool {
	if current == nil {
		return true
	}
	cur := current.Values[energy.ActiveImport]
	switch {
	case wantHigher && v.GreaterThan(*cur):
		return true
	case !wantHigher && v.LessThan(*cur):
		return true
	case v.Equal(*cur):
		return candidate.Window.From.Before(current.Window.From)
	default:
		return false
	}
}

// cloneRow deep-copies row's maps so a pointer into the result (Summary.Peak,
// Summary.Valley) can never alias — and so mutating it can never corrupt —
// the caller's own rows slice.
func cloneRow(row *Row) Row {
	c := *row
	c.Values = cloneRegisterMap(row.Values)
	c.Indexes = cloneRegisterMap(row.Indexes)
	c.Suspect = cloneSuspectMap(row.Suspect)
	c.Resolution = cloneResolutionMap(row.Resolution)
	return c
}

func cloneRegisterMap(m map[energy.Register]*decimal.Decimal) map[energy.Register]*decimal.Decimal {
	if m == nil {
		return nil
	}
	out := make(map[energy.Register]*decimal.Decimal, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneSuspectMap(m map[energy.Register]energy.Suspicion) map[energy.Register]energy.Suspicion {
	if m == nil {
		return nil
	}
	out := make(map[energy.Register]energy.Suspicion, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneResolutionMap(m map[energy.Register]string) map[energy.Register]string {
	if m == nil {
		return nil
	}
	out := make(map[energy.Register]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
