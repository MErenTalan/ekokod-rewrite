package icmal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Table is a sheet read by the caller: a header row and data rows.
type Table struct {
	Header []string
	Rows   [][]string
}

// Warning is a row-level (or file-level, Row 0) finding.
type Warning struct {
	Row  int // 1-based data row, 0 = file level
	Code string
	Text string
}

// Warning codes.
const (
	WarnInvalidPeriod         = "invalid_period"
	WarnInvalidNumber         = "invalid_number"
	WarnCancelledExcluded     = "cancelled_row_excluded"
	WarnSupplementaryExcluded = "supplementary_correction_excluded"
	WarnZeroKwhExcluded       = "zero_kwh_row_excluded"
	WarnMissingEtso           = "etso_missing"
	WarnBasePriceMissing      = "base_price_missing"
	WarnReactiveAmbiguous     = "reactive_register_ambiguous"
	WarnReactiveQuantity      = "reactive_quantity_missing"
	WarnPowerManualEntry      = "power_requires_manual_entry"
	WarnPowerUnstable         = "power_price_unstable"
)

// Parsed is a parsed icmal.
type Parsed struct {
	Rows     []model.IcmalRow
	Format   NumberFormat
	Warnings []Warning
}

// ErrMissingColumn is returned when a required column is absent.
var ErrMissingColumn = errors.New("icmal: required column missing")

// Parse maps the table onto icmal rows (02 §8.1, R129, R133). Raw keeps every
// original cell keyed by its first header occurrence.
func Parse(t Table) (Parsed, error) {
	cols := columnIndex(t.Header)
	for _, required := range []field{fPeriod, fTotalKwh} {
		if _, ok := cols[required]; !ok {
			return Parsed{}, fmt.Errorf("%w: %s", ErrMissingColumn, required)
		}
	}
	format, err := DetectNumberFormat(t)
	if err != nil {
		return Parsed{}, err
	}
	out := Parsed{Format: format}
	for n, row := range t.Rows {
		rowNo := n + 1
		cell := func(f field) (string, bool) {
			i, ok := cols[f]
			if !ok || i >= len(row) {
				return "", false
			}
			return strings.TrimSpace(row[i]), true
		}
		period, _ := cell(fPeriod)
		period = keepDigits(period)
		if len(period) != 6 {
			out.Warnings = append(out.Warnings, Warning{Row: rowNo, Code: WarnInvalidPeriod, Text: "accounting period is not YYYYMM"})
			continue
		}
		r := model.IcmalRow{Period: period}
		if v, ok := cell(fEtso); ok && v != "" {
			r.EtsoCode = &v
		}
		bad := false
		for _, target := range []struct {
			f   field
			dst **decimal.Decimal
		}{
			{fTotalKwh, &r.TotalKwh}, {fT0Kwh, &r.T0Kwh}, {fT1Kwh, &r.T1Kwh}, {fT2Kwh, &r.T2Kwh}, {fT3Kwh, &r.T3Kwh},
			{fEnergyCharge, &r.EnergyCharge}, {fDistributionCharge, &r.DistributionCharge}, {fReactiveCharge, &r.ReactiveCharge},
			{fPowerCharge, &r.PowerCharge}, {fOveruseCharge, &r.OveruseCharge}, {fInductiveKvarh, &r.InductiveKvarh},
			{fCapacitiveKvarh, &r.CapacitiveKvarh}, {fDemand, &r.DemandKw}, {fVatBase, &r.VatBase}, {fVat, &r.Vat},
			{fBtv, &r.Btv}, {fEnergyFund, &r.EnergyFund}, {fTrt, &r.Trt}, {fPriceDifference, &r.PriceDifference},
			{fCorrection, &r.CorrectionAmount},
		} {
			v, ok := cell(target.f)
			if !ok {
				continue
			}
			d, err := ParseNumber(v, format)
			if err != nil {
				out.Warnings = append(out.Warnings, Warning{Row: rowNo, Code: WarnInvalidNumber, Text: fmt.Sprintf("%s: %q", target.f, v)})
				bad = true
				continue
			}
			*target.dst = d
		}
		if bad {
			continue
		}
		if v, ok := cell(fCancelled); ok {
			switch strings.ToLowerSpecial(unicode.TurkishCase, v) {
			case "evet", "yes", "true":
				r.IsCancelled = true
			}
		}
		if v, ok := cell(fTerm); ok && v != "" {
			r.Term = &v
		}
		if v, ok := cell(fVoltage); ok && v != "" {
			r.VoltageLevel = &v
		}
		if v, ok := cell(fTimeType); ok { // R133: never inferred from T1–T3
			switch folded := NormaliseHeader(v); {
			case strings.HasPrefix(folded, "tek"):
				r.IsMultiTime = new(bool)
			case strings.HasPrefix(folded, "uc"):
				multi := true
				r.IsMultiTime = &multi
			}
		}
		raw := map[string]string{}
		for i, h := range t.Header {
			if _, dup := raw[h]; dup || i >= len(row) {
				continue
			}
			raw[h] = row[i]
		}
		r.Raw, _ = json.Marshal(raw) // a map of strings cannot fail to marshal
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}

func keepDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
