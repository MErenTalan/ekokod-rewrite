package icmal

import (
	"strings"
	"unicode"
)

// field names one icmal column the parser understands.
type field string

const (
	fPeriod             field = "period"
	fEtso               field = "etso"
	fSubscriberGroup    field = "subscriber_group"
	fTotalKwh           field = "total_kwh"
	fT0Kwh              field = "t0_kwh"
	fT1Kwh              field = "t1_kwh"
	fT2Kwh              field = "t2_kwh"
	fT3Kwh              field = "t3_kwh"
	fSupplementaryKwh   field = "supplementary_kwh"
	fEnergyCharge       field = "energy_charge"
	fDistributionCharge field = "distribution_charge"
	fReactiveCharge     field = "reactive_charge"
	fPowerCharge        field = "power_charge"
	fOveruseCharge      field = "overuse_charge"
	fInductiveKvarh     field = "inductive_kvarh"
	fCapacitiveKvarh    field = "capacitive_kvarh"
	fDemand             field = "demand_kw"
	fVatBase            field = "vat_base"
	fVat                field = "vat"
	fBtv                field = "btv"
	fEnergyFund         field = "energy_fund"
	fTrt                field = "trt"
	fPriceDifference    field = "price_difference"
	fCorrection         field = "correction_amount"
	fCancelled          field = "cancelled"
	fTerm               field = "term"
	fVoltage            field = "voltage_level"
	fTimeType           field = "time_type"
)

// aliases lists header aliases per field in priority order (02 §8.1, LBR C.1, C-1).
var aliases = []struct {
	field   field
	numeric bool
	names   []string
}{
	{fPeriod, false, []string{"Muhasebe Dönemi", "Dönem"}},
	{fEtso, false, []string{"ETSO Kodu", "ETSO"}},
	{fSubscriberGroup, false, []string{"Fatura Abone Grubu", "Abone Grubu"}},
	{fTotalKwh, true, []string{"Toplam Kwh"}},
	{fT0Kwh, true, []string{"T0 Kwh"}},
	{fT1Kwh, true, []string{"T1 Kwh"}},
	{fT2Kwh, true, []string{"T2 Kwh"}},
	{fT3Kwh, true, []string{"T3 Kwh"}},
	{fSupplementaryKwh, true, []string{"Ek Tüketim T0 Kwh"}},
	{fEnergyCharge, true, []string{"Enerji Bedeli Toplam", "Enerji Bedeli"}},
	{fDistributionCharge, true, []string{"Toplam Dağıtım Bedeli", "Dağıtım Bedeli"}},
	{fReactiveCharge, true, []string{"Reaktif Bedel", "Reaktif Bedeli"}},
	{fPowerCharge, true, []string{"Güç Bedeli"}},
	{fOveruseCharge, true, []string{"Güç Aşım Bedeli"}},
	{fInductiveKvarh, true, []string{"Reaktif Induktif Kwh", "Induktif Kwh"}},
	{fCapacitiveKvarh, true, []string{"Reaktif Kapasitif Kwh", "Kapasitif Kwh"}},
	{fDemand, true, []string{"Demant", "Talep", "Demand"}},
	{fVatBase, true, []string{"KDV Matrahı", "Matrah"}},
	{fVat, true, []string{"KDV"}},
	{fBtv, true, []string{"Btv Bedeli", "Btv", "Belediye Tüketim Vergisi"}},
	{fEnergyFund, true, []string{"Enerji Fonu"}},
	{fTrt, true, []string{"Trt Bedeli"}},
	{fPriceDifference, true, []string{"Fiyat Farkı"}},
	{fCorrection, true, []string{"Düzeltme Tutarı"}},
	{fCancelled, false, []string{"Fatura İptal Mi?", "İptal"}},
	{fTerm, false, []string{"Terim", "Tarife Terimi"}},
	{fVoltage, false, []string{"AG/OG", "Gerilim"}},
	{fTimeType, false, []string{"TekZaman/Üç Zaman", "Zaman Tipi"}},
}

var turkishFold = strings.NewReplacer("ı", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c", "i̇", "i")

// NormaliseHeader lower-cases with Turkish rules, folds Turkish letters to
// ASCII and drops everything but letters and digits.
func NormaliseHeader(s string) string {
	folded := turkishFold.Replace(strings.ToLowerSpecial(unicode.TurkishCase, s))
	var b strings.Builder
	for _, r := range folded {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// columnIndex maps each field to its column: the first alias that matches
// any header wins, and among duplicate headers the first column wins.
func columnIndex(header []string) map[field]int {
	normalised := make([]string, len(header))
	for i, h := range header {
		normalised[i] = NormaliseHeader(h)
	}
	out := map[field]int{}
	for _, a := range aliases {
	names:
		for _, name := range a.names {
			want := NormaliseHeader(name)
			for i, h := range normalised {
				if h == want {
					out[a.field] = i
					break names
				}
			}
		}
	}
	return out
}

// HeaderScore counts the cells of row that name a known column, so a caller
// can find the header row of a sheet with preamble rows.
func HeaderScore(row []string) int {
	known := map[string]bool{}
	for _, a := range aliases {
		for _, name := range a.names {
			known[NormaliseHeader(name)] = true
		}
	}
	n := 0
	for _, cell := range row {
		if known[NormaliseHeader(cell)] {
			n++
		}
	}
	return n
}
