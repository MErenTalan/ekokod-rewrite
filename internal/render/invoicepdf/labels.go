package invoicepdf

import (
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// labels maps locale → key → text; line codes are keys too.
var labels = map[string]map[string]string{
	"tr": {
		model.BillLineEnergy: "Enerji bedeli", model.BillLineEnergyLowTier: "Enerji bedeli (düşük kademe)",
		model.BillLineEnergyHighTier: "Enerji bedeli (yüksek kademe)", model.BillLineEnergyT1: "Enerji bedeli T1 (gündüz)",
		model.BillLineEnergyT2: "Enerji bedeli T2 (puant)", model.BillLineEnergyT3: "Enerji bedeli T3 (gece)",
		model.BillLineDistribution: "Dağıtım bedeli", model.BillLineGreenEnergy: "Yeşil enerji bedeli", model.BillLinePower: "Güç bedeli",
		model.BillLineDemandOverrun: "Güç aşım bedeli", model.BillLineReactiveInductive: "Reaktif bedel (endüktif)",
		model.BillLineReactiveCapacitive: "Reaktif bedel (kapasitif)", model.BillLineVat: "KDV", model.BillLineGenerationCredit: "Üretim mahsubu",
		model.BillLineExtraPrefix: "Ek bedel", model.BillLineTaxPrefix: "Vergi/fon",
		"title": "Elektrik Faturası", "preview": "ÖNİZLEME — kesilmedi", "company": "Şirket", "building": "Bina", "scope": "Kapsam",
		"period": "Dönem", "dates": "Tarih aralığı", "days": "Gün", "tariff_date": "Tarife yürürlük", "currency": "Para birimi",
		"readings": "Endeksler", "register": "Sayaç", "start": "İlk", "end": "Son", "quantities": "Tüketim",
		"lines": "Fatura kalemleri", "item": "Kalem", "quantity": "Miktar", "unit_price": "Birim fiyat", "amount": "Tutar",
		"vat_base": "KDV matrahı", "total": "Ödenecek tutar", "reactive": "Reaktif", "ratio": "Oran", "limit": "Sınır",
		"applied": "Uygulandı", "yes": "Evet", "no": "Hayır", "ptf": "PTF + YEKDEM", "hours_expected": "Beklenen saat",
		"hours_matched": "Eşleşen saat", "hours_missing": "Eksik saat", "ptf_average": "Ortalama PTF", "yekdem": "YEKDEM",
		"members": "Sayaçlar", "flags": "İşaretler", "net": "Net tüketim", "active_import": "Aktif çekiş", "active_export": "Aktif veriş",
		"inductive": "Endüktif", "capacitive": "Kapasitif", "max_demand": "Maksimum demant", "footnote": "Tutarlar yuvarlanmamış miktar ve birim fiyattan hesaplanır; kayıtlı 4/6 haneli değerlerden yeniden hesaplamada kuruş farkı olabilir.",
		"analyzer": "Sayaç", "company_scope": "Şirket", "building_scope": "Bina",
	},
	"en": {
		model.BillLineEnergy: "Energy", model.BillLineEnergyLowTier: "Energy (low tier)", model.BillLineEnergyHighTier: "Energy (high tier)",
		model.BillLineEnergyT1: "Energy T1 (day)", model.BillLineEnergyT2: "Energy T2 (peak)", model.BillLineEnergyT3: "Energy T3 (night)",
		model.BillLineDistribution: "Distribution", model.BillLineGreenEnergy: "Green energy", model.BillLinePower: "Power",
		model.BillLineDemandOverrun: "Demand overrun", model.BillLineReactiveInductive: "Reactive (inductive)",
		model.BillLineReactiveCapacitive: "Reactive (capacitive)", model.BillLineVat: "VAT", model.BillLineGenerationCredit: "Generation credit",
		model.BillLineExtraPrefix: "Extra charge", model.BillLineTaxPrefix: "Tax/fund",
		"title": "Electricity Invoice", "preview": "PREVIEW — not issued", "company": "Company", "building": "Building", "scope": "Scope",
		"period": "Period", "dates": "Dates", "days": "Days", "tariff_date": "Tariff effective", "currency": "Currency",
		"readings": "Readings", "register": "Register", "start": "Start", "end": "End", "quantities": "Quantities",
		"lines": "Invoice lines", "item": "Item", "quantity": "Quantity", "unit_price": "Unit price", "amount": "Amount",
		"vat_base": "VAT base", "total": "Amount due", "reactive": "Reactive", "ratio": "Ratio", "limit": "Limit",
		"applied": "Applied", "yes": "Yes", "no": "No", "ptf": "PTF + YEKDEM", "hours_expected": "Hours expected",
		"hours_matched": "Hours matched", "hours_missing": "Hours missing", "ptf_average": "Average PTF", "yekdem": "YEKDEM",
		"members": "Meters", "flags": "Flags", "net": "Net consumption", "active_import": "Active import", "active_export": "Active export",
		"inductive": "Inductive", "capacitive": "Capacitive", "max_demand": "Max demand", "footnote": "Amounts are computed from unrounded quantities and prices; re-deriving them from the stored 4/6-decimal values can differ by a kuruş.",
		"analyzer": "Meter", "company_scope": "Company", "building_scope": "Building",
	},
}

func label(locale, key string) string {
	if v, ok := labels[locale][key]; ok {
		return v
	}
	return key
}

// lineLabel localises a line code; extra:/tax: codes keep their own name.
func lineLabel(locale, code string) string {
	for _, prefix := range []string{model.BillLineExtraPrefix, model.BillLineTaxPrefix} {
		if name, ok := strings.CutPrefix(code, prefix); ok {
			return label(locale, prefix) + ": " + name
		}
	}
	return label(locale, code)
}
