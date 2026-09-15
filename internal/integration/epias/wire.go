package epias

import "encoding/json"

// This file holds EVERY EPİAŞ-shaped struct this client decodes, and
// nothing else — no pure logic lives here (mapping.go's counterpart in the
// meter-adapter template). It exists as its own file so the one place a
// wrong or unverified field name could hide is a single, small, reviewable
// surface.
//
// WIRE-SHAPE WARNING: these field names are transcribed from
// docs/rewrite/06-integrations.md §7 and the task-12 brief, not captured
// from a live EPİAŞ response — this codebase never calls the real
// endpoint. F14 (the isolar/EPİAŞ hardening pass) is the task that
// verifies them against real transparency-platform responses and is the
// only task expected to correct anything below.

// mcpEnvelope is the day-ahead MCP (PTF) response's top level. Exactly one
// of Items or Body.DayAheadMCPList carries the rows; both are pointers so
// "absent"/"null" (both nil: malformed) is distinguishable from "present
// but empty" (an empty, non-nil slice: a legitimate zero-row result) per
// adapter-patterns #6.
type mcpEnvelope struct {
	Items *[]json.RawMessage `json:"items"`
	Body  *struct {
		DayAheadMCPList *[]json.RawMessage `json:"dayAheadMCPList"`
	} `json:"body"`
}

// rows returns the envelope's row list and whether one of the two known
// shapes was actually present.
func (e mcpEnvelope) rows() ([]json.RawMessage, bool) {
	if e.Items != nil {
		return *e.Items, true
	}
	if e.Body != nil && e.Body.DayAheadMCPList != nil {
		return *e.Body.DayAheadMCPList, true
	}
	return nil, false
}

// mcpItem is one row of either MCP response shape. Hour is present in some
// observed response variants but is redundant with Date (which already
// carries the full hour-resolution instant) — unverified against a live
// response; kept only so an unexpected extra field never fails decoding.
type mcpItem struct {
	Date  string      `json:"date"`
	Hour  *string     `json:"hour"`
	Price json.Number `json:"price"`
}

// unitCostEnvelope is the YEKDEM unit-cost response's top level, same
// two-shapes-of-one-field pattern as mcpEnvelope.
type unitCostEnvelope struct {
	Items *[]json.RawMessage `json:"items"`
	Body  *struct {
		RenewableSMUnitCostList *[]json.RawMessage `json:"renewableSMUnitCostList"`
	} `json:"body"`
}

func (e unitCostEnvelope) rows() ([]json.RawMessage, bool) {
	if e.Items != nil {
		return *e.Items, true
	}
	if e.Body != nil && e.Body.RenewableSMUnitCostList != nil {
		return *e.Body.RenewableSMUnitCostList, true
	}
	return nil, false
}

// unitCostItem is one row of either YEKDEM response shape. Period and Date
// are both pointers because only one is expected per row (the brief names
// both: "period|date") — whichever is present resolves the (year, month).
type unitCostItem struct {
	Period   *string     `json:"period"`
	Date     *string     `json:"date"`
	UnitCost json.Number `json:"unitCost"`
}

// mcpRequestBody is the day-ahead MCP data call's JSON body.
type mcpRequestBody struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// unitCostRequestBody is the YEKDEM unit-cost call's JSON body — same
// shape as mcpRequestBody, kept as its own type so the two endpoints never
// share a struct that a future field addition to one silently leaks into
// the other.
type unitCostRequestBody struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}
