package epias

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// mcpRow is one successfully parsed PTF hour.
type mcpRow struct {
	Ts    time.Time
	Price decimal.Decimal
}

// unitCostRow is one successfully parsed YEKDEM month.
type unitCostRow struct {
	Year, Month int16
	Value       decimal.Decimal
}

// newStrictDecoder builds a *json.Decoder over body with UseNumber (so
// every JSON number survives as json.Number, never float64) and
// DisallowUnknownFields off (a provider's response may legitimately carry
// fields this client does not model; only the fields it declares are ever
// read).
//
// Every .Decode call below takes its target's address directly at the
// call site (never through a shared `func(body []byte, v any) error`
// helper): TestIntegrationTreesDoNotParseFloats flags ANY
// (*json.Decoder).Decode call whose static argument type is `any` — an
// interface, which is exactly the route a JSON number could reach an
// untyped float64 through with no struct field ever spelling "float" —
// regardless of what a given call site actually passes, so the concrete
// struct pointer must be visible at each Decode call itself.
func newStrictDecoder(body []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec
}

// parseMCPResponse decodes one day-ahead MCP HTTP response body into rows,
// with per-row warnings for a row that fails to parse (adapter-patterns
// #7) and ErrMalformedPayload for a response whose top-level shape is
// neither known shape (adapter-patterns #6), or that contains two rows for
// the same Date (06 §7 / task-12 brief: "Duplicate `date` within one
// response → ErrMalformedPayload" — a malformed import must not silently
// pick a winner).
func parseMCPResponse(body []byte) ([]mcpRow, []integration.Warning, error) {
	var env mcpEnvelope
	if err := newStrictDecoder(body).Decode(&env); err != nil {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "mcp"}
	}
	raw, ok := env.rows()
	if !ok {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "mcp"}
	}

	rows := make([]mcpRow, 0, len(raw))
	var warnings []integration.Warning
	seen := make(map[time.Time]bool, len(raw))

	for i, r := range raw {
		var item mcpItem
		if err := newStrictDecoder(r).Decode(&item); err != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("row %d: %s", i, "could not decode")})
			continue
		}
		ts, err := normalize.ISO8601(item.Date)
		if err != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("date at row %d", i)})
			continue
		}
		price, err := normalize.JSONNumber(item.Price)
		if err != nil {
			warnings = append(warnings, integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("price at row %d", i)})
			continue
		}
		ts = ts.UTC()
		if seen[ts] {
			return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "mcp"}
		}
		seen[ts] = true
		rows = append(rows, mcpRow{Ts: ts, Price: price})
	}
	return rows, warnings, nil
}

// unitCostLayouts are the "period"/"date" field formats tried, in order,
// to resolve a YEKDEM row's (year, month) — unverified against a live
// response (see wire.go's warning); tried widest-to-narrowest so a
// full-instant date value still resolves correctly.
var unitCostLayouts = []string{"2006-01", "2006-01-02"}

// resolveYearMonth parses either a "period" (e.g. "2026-03") or a "date"
// (e.g. "2026-03-01" or a full ISO 8601 instant) into an Istanbul-local
// (year, month). Exactly one of period/date must be non-nil and parse; any
// other shape reports ok=false.
func resolveYearMonth(period, date *string) (year, month int16, ok bool) {
	var raw string
	switch {
	case period != nil:
		raw = *period
	case date != nil:
		raw = *date
	default:
		return 0, 0, false
	}
	if raw == "" {
		return 0, 0, false
	}

	for _, layout := range unitCostLayouts {
		if t, err := time.ParseInLocation(layout, raw, normalize.Istanbul); err == nil {
			y, m, _ := t.Date()
			return int16(y), int16(m), true
		}
	}
	if t, err := normalize.ISO8601(raw); err == nil {
		y, m, _ := t.In(normalize.Istanbul).Date()
		return int16(y), int16(m), true
	}
	return 0, 0, false
}

// parseUnitCostResponse decodes one YEKDEM unit-cost HTTP response body
// into rows, plus the count of rows dropped because they failed to parse.
//
// M2 (task-12-fix1-findings.md, fix round 1): unlike parseMCPResponse, a
// bad individual row here cannot be reported through a
// []integration.Warning the way an MCP row's can — YekdemUnitCost's own
// signature (task-12 brief: "func (c *Client) YekdemUnitCost(ctx
// context.Context, from, to time.Time) ([]model.YekdemMonthly, error)") is
// pinned by the brief with no Warnings slot, and that exact signature is
// also what marketdata.PriceSource's interface requires — changing it
// would break that pinned contract. So the drop count returned here does
// NOT leave parseUnitCostResponse as a Warning; instead Client.YekdemUnitCost
// accumulates it across chunks and exposes it through the OPTIONAL
// YekdemDropped() capability (see client.go), which marketdata.Syncer
// type-asserts for and folds into the sync run's `detail` JSON
// (syncDetail.YekdemDropped in sync.go) — so a systematically malformed
// YEKDEM feed is still visible to an operator reading job_runs, even
// though it cannot ride a Warning. A malformed top-level shape, or two
// rows for the same (year, month) within one response, is still a hard
// ErrMalformedPayload, exactly as for MCP — those are not "dropped rows",
// they invalidate the whole response.
func parseUnitCostResponse(body []byte) ([]unitCostRow, int, error) {
	var env unitCostEnvelope
	if err := newStrictDecoder(body).Decode(&env); err != nil {
		return nil, 0, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "unit-cost"}
	}
	raw, ok := env.rows()
	if !ok {
		return nil, 0, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "unit-cost"}
	}

	rows := make([]unitCostRow, 0, len(raw))
	type key struct{ y, m int16 }
	seen := make(map[key]bool, len(raw))
	dropped := 0

	for _, r := range raw {
		var item unitCostItem
		if err := newStrictDecoder(r).Decode(&item); err != nil {
			dropped++
			continue
		}
		year, month, ok := resolveYearMonth(item.Period, item.Date)
		if !ok {
			dropped++
			continue
		}
		value, err := normalize.JSONNumber(item.UnitCost)
		if err != nil {
			dropped++
			continue
		}
		k := key{year, month}
		if seen[k] {
			return nil, 0, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderEPIAS, Op: "unit-cost"}
		}
		seen[k] = true
		rows = append(rows, unitCostRow{Year: year, Month: month, Value: value})
	}
	return rows, dropped, nil
}
