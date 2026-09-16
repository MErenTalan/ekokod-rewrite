package tariff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Draft is a tariff definition before persistence.
type Draft struct {
	Tariff       model.Tariff
	VatRate      *decimal.Decimal
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}

// Field error codes.
const (
	CodeRequired    = "required"
	CodeMustBeEmpty = "must_be_empty"
	CodeInvalid     = "invalid"
	CodeNegative    = "negative"
	CodeMustBeTRY   = "must_be_try"
	CodeDuplicate   = "duplicate"
	CodeOutOfRange  = "out_of_range"
)

// ValidationError maps every failing field to a machine code.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + e.Fields[k]
	}
	return "tariff invalid: " + strings.Join(parts, ", ")
}

// Validate applies R124, R125, R127, R128 and I-11/C-3/M-5, collecting every
// field error, and returns the tariff with VatRate set and unset price sources
// normalised to the column default kbk.
func Validate(d Draft) (model.Tariff, error) {
	t := d.Tariff
	f := map[string]string{}
	set := func(field, code string) {
		if _, ok := f[field]; !ok {
			f[field] = code
		}
	}
	switch {
	case d.VatRate == nil:
		set("vat_rate", CodeRequired)
	case d.VatRate.IsNegative():
		set("vat_rate", CodeNegative)
	default:
		t.VatRate = *d.VatRate
	}

	for _, src := range []*model.PriceSource{&t.PowerPriceSource, &t.ReactivePriceSource, &t.DistributionPriceSource} {
		if *src == "" {
			*src = model.PriceSourceKbk
		}
	}
	for field, ok := range map[string]bool{
		"currency": t.Currency.Valid(), "energy_type": t.EnergyType.Valid(), "voltage_level": t.VoltageLevel.Valid(),
		"user_group": t.UserGroup.Valid(), "price_type": t.PriceType.Valid(), "term": t.Term.Valid(),
		"supply_company": t.SupplyCompany.Valid(), "generation_usage": t.GenerationUsage.Valid(),
		"power_price_source": t.PowerPriceSource.Valid(), "reactive_price_source": t.ReactivePriceSource.Valid(),
		"distribution_price_source": t.DistributionPriceSource.Valid(),
	} {
		if !ok {
			set(field, CodeInvalid)
		}
	}

	ptf := t.UsePtfYekdem
	switch t.PriceType {
	case model.PriceTypeSingleTime:
		if !ptf && t.SingleTimePrice == nil {
			set("single_time_price", CodeRequired)
		}
	case model.PriceTypeMultiTime:
		if ptf { // R124
			requireAll(set, map[string]*decimal.Decimal{"kbk_t1": t.KbkT1, "kbk_t2": t.KbkT2, "kbk_t3": t.KbkT3})
		} else {
			requireAll(set, map[string]*decimal.Decimal{"t1_price": t.T1Price, "t2_price": t.T2Price, "t3_price": t.T3Price})
		}
	}
	if ptf {
		if t.KbkEnergy == nil {
			set("kbk_energy", CodeRequired)
		}
		if t.Currency != model.CurrencyTRY { // R127
			set("currency", CodeMustBeTRY)
		}
		if t.DistributionPriceSource == model.PriceSourceKbk && t.KbkDistributionCostTlPerKwh == nil { // I-11b
			set("kbk_distribution_cost_tl_per_kwh", CodeRequired)
		}
		if t.ReactivePriceSource == model.PriceSourceKbk && t.KbkReactivePower == nil {
			set("kbk_reactive_power", CodeRequired)
		}
	}
	if t.GenerationUsage == model.GenerationUsageSubtractFromTotal && t.GenerationPricePerKwh == nil {
		set("generation_price_per_kwh", CodeRequired)
	}

	switch t.Term { // R125
	case model.TariffTermBinomial:
		if t.ContractedPowerKw == nil {
			set("contracted_power_kw", CodeRequired)
		}
		if ptf && t.PowerPriceSource == model.PriceSourceKbk {
			if t.KbkPowerPrice == nil { // I-11a
				set("kbk_power_price", CodeRequired)
			}
		} else if t.PowerUnitPrice == nil {
			set("power_unit_price", CodeRequired)
		}
	case model.TariffTermMonomial:
		if t.ContractedPowerKw != nil {
			set("contracted_power_kw", CodeMustBeEmpty)
		}
		if t.PowerUnitPrice != nil {
			set("power_unit_price", CodeMustBeEmpty)
		}
	}
	if t.OveruseThresholdKwhPerDay != nil && t.OverusePrice == nil { // C-3
		set("overuse_price", CodeRequired)
	}

	for i, tax := range d.Taxes {
		if strings.TrimSpace(tax.Name) == "" {
			set(fmt.Sprintf("taxes[%d].name", i), CodeRequired)
		}
		if tax.Rate.IsNegative() {
			set(fmt.Sprintf("taxes[%d].rate", i), CodeNegative)
		}
	}
	for i, c := range d.ExtraCharges {
		if strings.TrimSpace(c.Name) == "" {
			set(fmt.Sprintf("extra_charges[%d].name", i), CodeRequired)
		}
		if !c.Basis.Valid() {
			set(fmt.Sprintf("extra_charges[%d].basis", i), CodeInvalid)
		}
		if c.Basis == model.ExtraChargeBasisPerContractedKw && t.ContractedPowerKw == nil { // M-5
			set(fmt.Sprintf("extra_charges[%d].basis", i), CodeRequired)
		}
	}
	seen := map[[2]int16]bool{}
	for i, y := range d.ManualYekdem {
		if y.Month < 1 || y.Month > 12 {
			set(fmt.Sprintf("manual_yekdem[%d].month", i), CodeOutOfRange)
		}
		key := [2]int16{y.Year, y.Month}
		if seen[key] {
			set(fmt.Sprintf("manual_yekdem[%d]", i), CodeDuplicate)
		}
		seen[key] = true
	}

	if len(f) > 0 {
		return model.Tariff{}, &ValidationError{Fields: f}
	}
	return t, nil
}

func requireAll(set func(field, code string), fields map[string]*decimal.Decimal) {
	for name, v := range fields {
		if v == nil {
			set(name, CodeRequired)
		}
	}
}
