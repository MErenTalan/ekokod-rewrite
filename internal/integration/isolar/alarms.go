package isolar

import (
	"context"
	"encoding/json"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// opGetFaultAlarmInfo is integration_definitions.json's isolar row key
// (R40) for fault alarms.
const opGetFaultAlarmInfo = "get_fault_alarm_info"

// faultsPageSize matches Devices' page/size convention (06 §6 gives no
// explicit number for this call).
const faultsPageSize = 100

// faultsProcessStatus asks for BOTH open and closed faults ("999" —
// documented by bcem-energy's alarms/route.ts:64 comment: "processStatus:
// string (8: Unclosed, 9: Closed, 999: Both)"). Every isolarClient.ts call
// site defaults to "8" (Unclosed) for its own alarm-check purpose, but this
// package's Faults fetches a [from, to) HISTORY window and filters
// client-side (see below) — it would silently miss faults that closed
// within the window if it asked for only the unclosed ones, so it asks for
// both. Documented, not a legacy contradiction: no call site's choice of
// "8" is itself part of the wire SHAPE R41 governs, only a caller-side
// filter default.
const faultsProcessStatus = "999"

// Fault is one fault alarm, as getFaultAlarmInfo reports it. Forwarding
// (translation, recipient delivery, isolar_forwarded_alarms dedup) is F9's
// job ("isolar.sync_plant is F9's job", BLOCKER item 4) — this Client only
// fetches.
type Fault struct {
	Ref        string
	PSID       string
	PSKey      *string
	Code       string
	Message    string
	OccurredAt time.Time
}

// Faults fetches every fault alarm across the whole credential (the call
// carries no ps_id: getFaultAlarmInfo reports across every plant the
// credential can see, each row naming its own ps_id/ps_key — R41,
// isolarClient.ts:652-671's own ps_id: options.psId || "" default) over
// [from, to), following rowCount across pages of faultsPageSize (R44 page
// budget).
//
// R41 provenance for what is (and is NOT) sent: isolarTypes.ts's
// GetFaultAlarmInfoReq interface (:262-273) declares optional
// startTime/endTime request fields, but isolarClient.ts's actual
// getFaultAlarmInfo() function (:652-671) — the wire authority's concrete
// implementation, not just its type declaration — never wires them into
// the request body; it only ever sends appkey/ps_id/process_status/page/
// size. This package follows the concrete implementation (the thing that
// actually shaped real traffic) rather than the unused interface fields,
// and instead filters the account-wide, paginated result to [from, to) by
// create_time client-side, the same half-open-window convention
// mapPoint/series.go already applies. Flagged for F14 verification
// alongside every other unconfirmed iSolar wire detail (R21/R35/this
// task's wire.go note).
func (c *Client) Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]Fault, error) {
	var out []Fault
	received := 0
	for page := 1; ; page++ {
		if page > maxPageBudget {
			return nil, c.pageBudgetExceeded(opGetFaultAlarmInfo)
		}
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetFaultAlarmInfo,
			bearer: true,
			body: map[string]any{
				"ps_id":          "",
				"process_status": faultsProcessStatus,
				"page":           page,
				"size":           faultsPageSize,
			},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedFaults
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opGetFaultAlarmInfo)
		}
		pageList := pd.PageListCamel
		if len(pageList) == 0 {
			pageList = pd.PageListSnake
		}
		received += len(pageList)
		for _, wf := range pageList {
			f, mapErr := mapFault(wf, from, to)
			if mapErr != nil {
				// adapter-patterns.md item 7: skip one bad row (unparseable
				// or outside [from, to)), not the whole page.
				continue
			}
			out = append(out, f)
		}
		rowCount, rcErr := faultsRowCount(pd)
		if rcErr != nil || received >= int(rowCount) || len(pageList) == 0 {
			break
		}
	}
	return out, nil
}

// faultsRowCount prefers rowCount (camelCase — the shape confirmed
// elsewhere in this API family), falling back to row_count when rowCount's
// json.Number is empty (the key was absent) — see wire.go's wirePagedFaults
// doc for why this call hedges both spellings.
func faultsRowCount(pd wirePagedFaults) (int64, error) {
	if pd.RowCountCamel.String() != "" {
		return pd.RowCountCamel.Int64()
	}
	return pd.RowCountSnake.Int64()
}

func mapFault(w wireFault, from, to time.Time) (Fault, error) {
	if w.FaultCode == "" || w.PsID == "" {
		return Fault{}, malformedErr(opGetFaultAlarmInfo)
	}
	occurredAt, err := normalize.LocalLayout(isolarTimestampLayout, w.CreateTime)
	if err != nil {
		return Fault{}, err
	}
	if occurredAt.Before(from) || !occurredAt.Before(to) {
		return Fault{}, errOutsideWindow
	}
	return Fault{
		Ref:        w.FaultCode,
		PSID:       w.PsID,
		PSKey:      w.PsKey,
		Code:       w.FaultCode,
		Message:    w.FaultName,
		OccurredAt: occurredAt,
	}, nil
}
