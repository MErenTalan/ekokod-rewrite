// Package comparison computes the sectoral comparison of 02 §10.4 (R162):
// averages over peers with a value and competition ranks, ascending.
package comparison

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReasonSectorTooSmall hides a sector whose average could reveal one other tenant.
const ReasonSectorTooSmall = "sector_too_small"

// MinPeers is the smallest sector (with a monthly figure) that is compared.
const MinPeers = 3

const divPrecision = 6

// Figures are one building's inputs.
type Figures struct {
	BuildingID     uuid.UUID
	Daily, Monthly *decimal.Decimal
	Personnel      *int32
	AreaM2         *decimal.Decimal
}

// Metric is the building's value, the sector average, its rank (0 when it has
// no value) and how many peers were ranked.
type Metric struct {
	Value, Average *decimal.Decimal
	Rank, Ranked   int
}

// Result is the comparison for one building.
type Result struct {
	Available                               bool
	Reason                                  string
	Peers                                   int
	Daily, Monthly, CO2, PerCapita, PerArea Metric
}

type valueOf func(Figures) *decimal.Decimal

// Compare compares self against peers (which include self). gridFactor nil
// leaves CO2 unknown.
func Compare(self uuid.UUID, peers []Figures, gridFactor *decimal.Decimal) Result {
	res := Result{Peers: len(peers)}
	withMonthly := 0
	for _, p := range peers {
		if p.Monthly != nil {
			withMonthly++
		}
	}
	if withMonthly < MinPeers {
		res.Reason = ReasonSectorTooSmall
		return res
	}
	res.Available = true
	res.Daily = metric(self, peers, func(f Figures) *decimal.Decimal { return f.Daily })
	res.Monthly = metric(self, peers, func(f Figures) *decimal.Decimal { return f.Monthly })
	if gridFactor != nil {
		res.CO2 = metric(self, peers, func(f Figures) *decimal.Decimal {
			if f.Monthly == nil {
				return nil
			}
			v := f.Monthly.Mul(*gridFactor)
			return &v
		})
	}
	res.PerCapita = metric(self, peers, func(f Figures) *decimal.Decimal {
		if f.Monthly == nil || f.Personnel == nil || *f.Personnel <= 0 {
			return nil
		}
		v := f.Monthly.DivRound(decimal.NewFromInt32(*f.Personnel), divPrecision)
		return &v
	})
	res.PerArea = metric(self, peers, func(f Figures) *decimal.Decimal {
		if f.Monthly == nil || f.AreaM2 == nil || !f.AreaM2.IsPositive() {
			return nil
		}
		v := f.Monthly.DivRound(*f.AreaM2, divPrecision)
		return &v
	})
	return res
}

func metric(self uuid.UUID, peers []Figures, value valueOf) Metric {
	var m Metric
	sum := decimal.Zero
	var values []decimal.Decimal
	for _, p := range peers {
		v := value(p)
		if v == nil {
			continue
		}
		values = append(values, *v)
		sum = sum.Add(*v)
		if p.BuildingID == self {
			own := *v
			m.Value = &own
		}
	}
	m.Ranked = len(values)
	if m.Ranked > 0 {
		avg := sum.DivRound(decimal.NewFromInt(int64(m.Ranked)), divPrecision)
		m.Average = &avg
	}
	if m.Value != nil {
		m.Rank = 1
		for _, v := range values {
			if v.LessThan(*m.Value) {
				m.Rank++
			}
		}
	}
	return m
}
