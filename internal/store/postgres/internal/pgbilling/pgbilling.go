// Package pgbilling converts billing_parameters rows for both the scoped read
// repository and the admin upsert, so the jsonb and enum-array encoding lives
// in one place.
package pgbilling

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgnum"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// band is one reactive_bands element; decimals are JSON strings (05 §1).
type band struct {
	MinKw      string  `json:"min_kw"`
	MaxKw      *string `json:"max_kw"`
	Inductive  string  `json:"inductive"`
	Capacitive string  `json:"capacitive"`
}

// Row is the column set every billing_parameters query returns.
type Row = sqlcgen.BillingParametersEffectiveRow

// Decode maps a row onto the model, parsing decimals from strings, never float64.
func Decode(r Row) (model.BillingParameters, error) {
	var bands []band
	if err := json.Unmarshal(r.ReactiveBands, &bands); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.reactive_bands: %w", err)
	}
	var groups map[string]string
	if err := json.Unmarshal(r.TieringGroups, &groups); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.tiering_groups: %w", err)
	}
	out := model.BillingParameters{
		EffectiveFrom:            r.EffectiveFrom.Time,
		ReactivePenaltyBasis:     model.ReactivePenaltyBasis(r.ReactivePenaltyBasis),
		ReactiveExemptTerms:      convert[model.TariffTerm](r.ReactiveExemptTerms),
		ReactiveExemptUserGroups: convert[model.DistributionUserGroup](r.ReactiveExemptUserGroups),
		ReactiveBands:            make([]model.ReactiveBand, 0, len(bands)),
		TieringGroups:            make(map[model.DistributionUserGroup]decimal.Decimal, len(groups)),
		TieringMode:              model.TieringMode(r.TieringMode),
		TieringVoltageLevels:     convert[model.VoltageLevel](r.TieringVoltageLevels),
		TieringSupplyCompanies:   convert[model.SupplyCompany](r.TieringSupplyCompanies),
		MoneyRoundingMode:        model.MoneyRoundingMode(r.MoneyRoundingMode),
		CreatedAt:                r.CreatedAt.Time,
	}
	var err error
	if out.ReactiveExemptBelowKw, err = pgnum.NumericToDecimalPtr(r.ReactiveExemptBelowKw); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.reactive_exempt_below_kw: %w", err)
	}
	if out.ReactiveGenerationExemptKwh, err = pgnum.NumericToDecimal(r.ReactiveGenerationExemptKwh); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.reactive_generation_exempt_kwh: %w", err)
	}
	if out.PTFMissingHourTolerance, err = pgnum.NumericToDecimal(r.PtfMissingHourTolerance); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.ptf_missing_hour_tolerance: %w", err)
	}
	if out.DemandOverrunMultiplier, err = pgnum.NumericToDecimal(r.DemandOverrunMultiplier); err != nil {
		return model.BillingParameters{}, fmt.Errorf("billing_parameters.demand_overrun_multiplier: %w", err)
	}
	for i, b := range bands {
		mb := model.ReactiveBand{}
		for _, f := range []struct {
			src string
			dst *decimal.Decimal
		}{{b.MinKw, &mb.MinKw}, {b.Inductive, &mb.Inductive}, {b.Capacitive, &mb.Capacitive}} {
			if *f.dst, err = decimal.NewFromString(f.src); err != nil {
				return model.BillingParameters{}, fmt.Errorf("billing_parameters.reactive_bands[%d]: %w", i, err)
			}
		}
		if b.MaxKw != nil {
			d, err := decimal.NewFromString(*b.MaxKw)
			if err != nil {
				return model.BillingParameters{}, fmt.Errorf("billing_parameters.reactive_bands[%d].max_kw: %w", i, err)
			}
			mb.MaxKw = &d
		}
		out.ReactiveBands = append(out.ReactiveBands, mb)
	}
	for g, v := range groups {
		d, err := decimal.NewFromString(v)
		if err != nil {
			return model.BillingParameters{}, fmt.Errorf("billing_parameters.tiering_groups[%s]: %w", g, err)
		}
		out.TieringGroups[model.DistributionUserGroup(g)] = d
	}
	return out, nil
}

// Encode maps p onto the admin upsert's parameters.
func Encode(p model.BillingParameters, effectiveFrom pgtype.Date) (sqlcgen.AdminUpsertBillingParametersParams, error) {
	bands := make([]band, len(p.ReactiveBands))
	for i, b := range p.ReactiveBands {
		bands[i] = band{MinKw: b.MinKw.String(), Inductive: b.Inductive.String(), Capacitive: b.Capacitive.String()}
		if b.MaxKw != nil {
			s := b.MaxKw.String()
			bands[i].MaxKw = &s
		}
	}
	bandsJSON, err := json.Marshal(bands)
	if err != nil {
		return sqlcgen.AdminUpsertBillingParametersParams{}, err
	}
	groups := make(map[string]string, len(p.TieringGroups))
	for g, v := range p.TieringGroups {
		groups[string(g)] = v.String()
	}
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return sqlcgen.AdminUpsertBillingParametersParams{}, err
	}
	return sqlcgen.AdminUpsertBillingParametersParams{
		EffectiveFrom:               effectiveFrom,
		ReactivePenaltyBasis:        sqlcgen.ReactivePenaltyBasis(p.ReactivePenaltyBasis),
		ReactiveExemptBelowKw:       pgnum.DecimalPtrToNumeric(p.ReactiveExemptBelowKw),
		ReactiveExemptTerms:         strs(p.ReactiveExemptTerms),
		ReactiveExemptUserGroups:    strs(p.ReactiveExemptUserGroups),
		ReactiveGenerationExemptKwh: pgnum.DecimalToNumeric(p.ReactiveGenerationExemptKwh),
		ReactiveBands:               bandsJSON,
		TieringGroups:               groupsJSON,
		TieringMode:                 sqlcgen.TieringMode(p.TieringMode),
		TieringVoltageLevels:        strs(p.TieringVoltageLevels),
		TieringSupplyCompanies:      strs(p.TieringSupplyCompanies),
		PtfMissingHourTolerance:     pgnum.DecimalToNumeric(p.PTFMissingHourTolerance),
		DemandOverrunMultiplier:     pgnum.DecimalToNumeric(p.DemandOverrunMultiplier),
		MoneyRoundingMode:           sqlcgen.MoneyRoundingMode(p.MoneyRoundingMode),
	}, nil
}

// IstanbulDate is on's Europe/Istanbul calendar date as a SQL date (I-14).
func IstanbulDate(on time.Time) pgtype.Date {
	y, m, d := on.In(istanbul).Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

func convert[T ~string](in []string) []T {
	out := make([]T, len(in))
	for i, s := range in {
		out[i] = T(s)
	}
	return out
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}
