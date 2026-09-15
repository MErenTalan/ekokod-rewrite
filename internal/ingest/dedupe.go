package ingest

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Dedupe implements D1: rows sharing (AnalyzerID, Ts, Kind) with identical
// registers keep one copy; rows sharing the key with DIFFERENT register
// values are all rejected as RejectConflictingDuplicate, so the batch never
// asks BulkInsert to arbitrate (it would refuse with ErrConflict anyway —
// Global Constraints, "batches are all-or-nothing, duplicate keys refused").
//
// kept preserves the first-seen order of each distinct key; a group of
// identical duplicates contributes its FIRST occurrence to kept and no
// Rejection at all (a true duplicate is not an error, just redundant data).
func Dedupe(rows []model.MeterReading) (kept []model.MeterReading, rejected []Rejection) {
	type key struct {
		analyzerID string
		ts         int64
		kind       model.ReadingKind
	}
	rowKey := func(r model.MeterReading) key {
		return key{analyzerID: r.AnalyzerID.String(), ts: r.Ts.UTC().UnixNano(), kind: r.Kind}
	}

	groups := make(map[key][]int, len(rows))
	var order []key
	for i, r := range rows {
		k := rowKey(r)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}

	for _, k := range order {
		idxs := groups[k]
		if len(idxs) == 1 {
			kept = append(kept, rows[idxs[0]])
			continue
		}
		identical := true
		for _, idx := range idxs[1:] {
			if !registersEqual(rows[idxs[0]], rows[idx]) {
				identical = false
				break
			}
		}
		if identical {
			kept = append(kept, rows[idxs[0]])
			continue
		}
		for _, idx := range idxs {
			rejected = append(rejected, Rejection{Ts: rows[idx].Ts, Kind: rows[idx].Kind, Reason: RejectConflictingDuplicate})
		}
	}
	return kept, rejected
}

// registersEqual reports whether a and b carry the same value (nil or
// otherwise) in every one of allRegisters' fourteen fields.
func registersEqual(a, b model.MeterReading) bool {
	for _, reg := range allRegisters {
		if !decimalPtrEqual(reg.get(a), reg.get(b)) {
			return false
		}
	}
	return true
}

func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
