package legacy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/gridbox"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
)

// MultiplierDecision is R409 for one analyzer: what the stored values are and
// whether that is known. MapWith is applied while mapping; Applied is what the
// resulting readings carry as multiplier_applied.
type MultiplierDecision struct {
	Provider   string
	Stored     string
	MapWith    decimal.Decimal
	Applied    decimal.Decimal
	Determined bool
	Reason     string
}

var one = decimal.NewFromInt(1)

// DecideMultiplier is R409/Q-J5. confirmed is an operator's answer for an
// analyzer whose values could be either raw or multiplied: "raw" or "multiplied".
func DecideMultiplier(provider, stored, confirmed string) MultiplierDecision {
	d := MultiplierDecision{Provider: provider, Stored: stored, MapWith: one, Applied: one}
	m := one
	if v, err := ParseNumber(stored); err == nil && v != nil && v.IsPositive() {
		m = *v
	}
	switch provider {
	case "ARIL":
		// Legacy multiplied ARIL values before storing them (arilLoadProfileToEnergyValue).
		d.Applied, d.Determined, d.Reason = m, true, "aril_stored_multiplied"
	case "GRIDBOX":
		// The provider's *WithMultiplier register, or raw × m (06 §3).
		d.MapWith, d.Applied, d.Determined, d.Reason = m, m, true, "gridbox_registers"
	case "OSOS":
		switch {
		case m.Equal(one):
			d.Determined, d.Reason = true, "multiplier_one"
		case confirmed == "raw":
			d.MapWith, d.Applied, d.Determined, d.Reason = m, m, true, "confirmed_raw"
		case confirmed == "multiplied":
			d.Applied, d.Determined, d.Reason = m, true, "confirmed_multiplied"
		default:
			d.Reason = "osos_multiplier_unknown"
		}
	default:
		d.Reason = "provider_unknown"
	}
	return d
}

// RowReject is one refused reading.
type RowReject struct {
	Kind   model.ReadingKind
	Index  int
	Reason string
}

// IndexDrop is a negative delta between consecutive readings of one kind (R408).
type IndexDrop struct {
	Kind     model.ReadingKind
	From, To time.Time
	Delta    decimal.Decimal
}

// Readings is one analyzer's transformed series.
type Readings struct {
	Readings   []model.MeterReading
	Rejects    []RowReject
	Drops      []IndexDrop
	Duplicates int
}

var arrays = []struct {
	key  string
	kind model.ReadingKind
}{
	{"loadProfile", model.ReadingKindLoadProfile},
	{"daily", model.ReadingKindDaily},
	{"billing", model.ReadingKindBilling},
	{"reset", model.ReadingKindReset},
}

// earliest is the first plausible legacy reading (R408).
var earliest = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

// AnalyzerReadings is R408: every energyValues array → readings, with the
// ingestion mappers doing the register work.
func AnalyzerReadings(doc bson.M, provider string, dec MultiplierDecision, now time.Time) Readings {
	var out Readings
	legacyID := hexID(doc["_id"])
	analyzerID := ID("analyzers", legacyID)
	values := asDoc(doc["energyValues"])
	for _, a := range arrays {
		rows := asArray(values[a.key])
		byTs := map[time.Time]model.MeterReading{}
		for i, row := range rows {
			raw, err := rowJSON(row)
			if err != nil {
				out.Rejects = append(out.Rejects, RowReject{a.kind, i, "unparseable:row"})
				continue
			}
			r, field, ok := mapRow(raw, provider, analyzerID, a.kind, dec)
			if !ok {
				out.Rejects = append(out.Rejects, RowReject{a.kind, i, "unparseable:" + field})
				continue
			}
			if r.Ts.After(now) || r.Ts.Before(earliest) {
				out.Rejects = append(out.Rejects, RowReject{a.kind, i, "out_of_range"})
				continue
			}
			if _, dup := byTs[r.Ts]; dup {
				out.Duplicates++
			}
			byTs[r.Ts] = r // last wins
		}
		series := make([]model.MeterReading, 0, len(byTs))
		for _, r := range byTs {
			series = append(series, r)
		}
		sort.Slice(series, func(i, j int) bool { return series[i].Ts.Before(series[j].Ts) })
		for i := 1; i < len(series); i++ {
			prev, cur := series[i-1].ActiveImport, series[i].ActiveImport
			if prev != nil && cur != nil && cur.LessThan(*prev) {
				out.Drops = append(out.Drops, IndexDrop{Kind: a.kind, From: series[i-1].Ts, To: series[i].Ts, Delta: cur.Sub(*prev)})
			}
		}
		out.Readings = append(out.Readings, series...)
	}
	return out
}

func mapRow(raw json.RawMessage, provider string, analyzerID uuid.UUID, kind model.ReadingKind, dec MultiplierDecision) (model.MeterReading, string, bool) {
	if provider == "GRIDBOX" {
		r, field, err := gridbox.MapStoredRow(raw, analyzerID, kind, dec.MapWith)
		return r, field, err == nil
	}
	r, field, ok := osos.MapStoredRow(raw, analyzerID, kind, dec.MapWith)
	if !ok {
		return r, field, false
	}
	r.MultiplierApplied = dec.Applied
	if provider == "ARIL" {
		r.SourceProvider = model.IntegrationProviderARIL
		// ARIL has no time-of-use split or demand; legacy wrote "0" (06 §4).
		for _, p := range []**decimal.Decimal{&r.T1Import, &r.T2Import, &r.T3Import, &r.T1Export, &r.T2Export, &r.T3Export, &r.MaxDemandKw} {
			if *p != nil && (*p).IsZero() {
				*p = nil
			}
		}
	}
	return r, "", true
}

func asArray(v any) []any {
	switch a := v.(type) {
	case bson.A:
		return a
	case []any:
		return a
	}
	return nil
}

// rowJSON renders one embedded document as relaxed JSON for the provider mappers.
func rowJSON(v any) (json.RawMessage, error) {
	switch v.(type) {
	case bson.D, bson.M, map[string]any:
	default:
		return nil, fmt.Errorf("not a document: %T", v)
	}
	return bson.MarshalExtJSON(v, false, false)
}

func hexID(v any) string {
	switch id := v.(type) {
	case bson.ObjectID:
		return id.Hex()
	case string:
		return strings.TrimSpace(id)
	}
	return fmt.Sprint(v)
}
