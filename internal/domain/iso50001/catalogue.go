// Package iso50001 is the TS EN ISO 50001:2018 workbench's pure rules
// (01 §7.17): the clause catalogue, progress and the Gantt statuses.
package iso50001

// Main is a main clause (5–9).
type Main struct {
	ID    string
	Title map[string]string
	Subs  []Sub
}

// Sub is a sub-clause with its explanatory text and optional template (R330).
type Sub struct {
	ID          string
	Template    string
	Title       map[string]string
	Description map[string]string
}

// MainIDs are the clauses the calendar and the Gantt chart plan.
var MainIDs = []string{"5", "6", "7", "8", "9"}

// Mains is the catalogue in clause order.
func Mains() []Main { return catalogue }

// SubIDs lists every sub-clause in order.
func SubIDs() []string {
	var out []string
	for _, m := range catalogue {
		for _, s := range m.Subs {
			out = append(out, s.ID)
		}
	}
	return out
}

// SubByID resolves a sub-clause.
func SubByID(id string) (Sub, bool) {
	for _, m := range catalogue {
		for _, s := range m.Subs {
			if s.ID == id {
				return s, true
			}
		}
	}
	return Sub{}, false
}

// IsMain reports whether id is a main clause.
func IsMain(id string) bool {
	for _, m := range MainIDs {
		if m == id {
			return true
		}
	}
	return false
}
