package consumption

import (
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// toEnergyReading converts a stored meter reading into the pure domain's
// Reading shape. Every energy.Register round-trips through the
// obvious field mapping. MultiplierApplied is deliberately never read here:
// 02-domain-rules.md §2.2 applies the multiplier exactly once, at ingestion,
// and reapplying it in this layer would double it (Global Constraints).
func toEnergyReading(r model.MeterReading) energy.Reading {
	return energy.Reading{
		TS:   r.Ts,
		Kind: toEnergyKind(r.Kind),
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:             r.ActiveImport,
			energy.ReactiveInductiveImport:  r.ReactiveInductiveImport,
			energy.ReactiveCapacitiveImport: r.ReactiveCapacitiveImport,
			energy.T1Import:                 r.T1Import,
			energy.T2Import:                 r.T2Import,
			energy.T3Import:                 r.T3Import,
			energy.ActiveExport:             r.ActiveExport,
			energy.ReactiveInductiveExport:  r.ReactiveInductiveExport,
			energy.ReactiveCapacitiveExport: r.ReactiveCapacitiveExport,
			energy.T1Export:                 r.T1Export,
			energy.T2Export:                 r.T2Export,
			energy.T3Export:                 r.T3Export,
		},
		MaxDemandKw: r.MaxDemandKw,
	}
}

// toEnergyKind maps model.ReadingKind onto energy.Kind (04 §4.1's five
// values mirror 02 §2.3's four plus current_index — see R64). An unknown
// value passes through as-is rather than silently mapping to a wrong kind:
// model.ReadingKind.Valid() is the gate that keeps an invalid value from
// ever reaching here in practice.
func toEnergyKind(k model.ReadingKind) energy.Kind {
	switch k {
	case model.ReadingKindLoadProfile:
		return energy.KindLoadProfile
	case model.ReadingKindDaily:
		return energy.KindDaily
	case model.ReadingKindBilling:
		return energy.KindBilling
	case model.ReadingKindReset:
		return energy.KindReset
	case model.ReadingKindCurrentIndex:
		return energy.KindCurrentIndex
	default:
		return energy.Kind(k)
	}
}

// toModelKind is toEnergyKind's inverse.
func toModelKind(k energy.Kind) model.ReadingKind {
	switch k {
	case energy.KindLoadProfile:
		return model.ReadingKindLoadProfile
	case energy.KindDaily:
		return model.ReadingKindDaily
	case energy.KindBilling:
		return model.ReadingKindBilling
	case energy.KindReset:
		return model.ReadingKindReset
	case energy.KindCurrentIndex:
		return model.ReadingKindCurrentIndex
	default:
		return model.ReadingKind(k)
	}
}
