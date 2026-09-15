package isolar

import (
	"context"
	"encoding/json"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// opGetFaultAlarmInfo is 06 §6's operation name for fault alarms.
const opGetFaultAlarmInfo = "getFaultAlarmInfo"

// faultsPageSize matches Devices' page/size convention (06 §6 gives no
// explicit number for this call).
const faultsPageSize = 100

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
// credential can see, each row naming its own ps_id/ps_key) over
// [from, to), following rowCount across pages of faultsPageSize.
func (c *Client) Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]Fault, error) {
	var out []Fault
	for page := 1; page <= maxPages; page++ {
		raw, err := c.call(ctx, creds, callOptions{
			op:     opGetFaultAlarmInfo,
			bearer: true,
			body: map[string]any{
				"start_time": formatIsolarTime(from),
				"end_time":   formatIsolarTime(to),
				"curPage":    page,
				"size":       faultsPageSize,
			},
		})
		if err != nil {
			return nil, err
		}
		var pd wirePagedFaults
		if err := json.Unmarshal(raw, &pd); err != nil {
			return nil, malformedErr(opGetFaultAlarmInfo)
		}
		for _, wf := range pd.PageList {
			f, mapErr := mapFault(wf)
			if mapErr != nil {
				// adapter-patterns.md item 7: skip one bad row, not the
				// whole page.
				continue
			}
			out = append(out, f)
		}
		rowCount, rcErr := pd.RowCount.Int64()
		if rcErr != nil || len(out) >= int(rowCount) || len(pd.PageList) == 0 {
			break
		}
	}
	return out, nil
}

func mapFault(w wireFault) (Fault, error) {
	if w.AlarmID == "" || w.PsID == "" {
		return Fault{}, malformedErr(opGetFaultAlarmInfo)
	}
	occurredAt, err := normalize.LocalLayout(isolarTimeLayout, w.StartTime)
	if err != nil {
		return Fault{}, err
	}
	return Fault{
		Ref:        w.AlarmID,
		PSID:       w.PsID,
		PSKey:      w.PsKey,
		Code:       w.WarningCode,
		Message:    w.WarningName,
		OccurredAt: occurredAt,
	}, nil
}
