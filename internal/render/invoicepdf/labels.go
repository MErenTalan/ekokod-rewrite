package invoicepdf

import (
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// labels maps locale → key → text; line codes are keys too. Enum values use
// a prefix (ug: user group, vl: voltage, term:, pt: price type, sc: supply).
var labels = map[string]map[string]string{
	"tr": {
		model.BillLineEnergy: "Enerji bedeli", model.BillLineEnergyLowTier: "Enerji bedeli (düşük kademe)",
		model.BillLineEnergyHighTier: "Enerji bedeli (yüksek kademe)", model.BillLineEnergyT1: "Enerji bedeli T1 (gündüz)",
		model.BillLineEnergyT2: "Enerji bedeli T2 (puant)", model.BillLineEnergyT3: "Enerji bedeli T3 (gece)",
		model.BillLineDistribution: "Dağıtım bedeli", model.BillLineGreenEnergy: "Yeşil enerji bedeli", model.BillLinePower: "Güç bedeli",
		model.BillLineDemandOverrun: "Güç aşım bedeli", model.BillLineReactiveInductive: "Reaktif bedel (endüktif)",
		model.BillLineReactiveCapacitive: "Reaktif bedel (kapasitif)", model.BillLineReactiveWaiver: "Reaktif bedel tenzili (yılın ilk ihlali)",
		model.BillLineVat: "KDV", model.BillLineGenerationCredit: "Üretim mahsubu", model.BillLineGenerationSale: "Üretim satış bedeli (bilgi)",
		model.BillLineExtraPrefix: "Ek bedel", model.BillLineTaxPrefix: "Vergi/fon",

		"title": "Elektrik Faturası", "amount_due": "Ödenecek tutar", "vat_included": "KDV dahil", "days": "gün",
		"preview": "ÖNİZLEME — kesilmedi", "missing_days": "Eksik veri — bu fatura dönemin tamamını kapsamıyor. Verisi olmayan günler: ",
		"subscriber": "Abone", "company": "Şirket", "building": "Bina", "scope": "Kapsam", "meter": "Sayaç", "meters": "Sayaç sayısı",
		"installed_power": "Kurulu güç", "scope_analyzer": "Sayaç faturası", "scope_building": "Bina faturası", "scope_company": "Şirket faturası",
		"tariff": "Tarife", "tariff_name": "Tarife adı", "user_group": "Abone grubu", "voltage_term": "Gerilim / terim",
		"price_type": "Zaman dilimi", "supply": "Tedarik", "contracted_power": "Sözleşme gücü", "tariff_date": "Yürürlük",
		"per_building": "Her bina kendi tarifesiyle", "ptf_yekdem": "PTF + YEKDEM",
		"ug:residential": "Mesken", "ug:residential_plus": "Mesken (artı)", "ug:commercial": "Ticarethane",
		"ug:commercial_plus": "Ticarethane (artı)", "ug:industrial": "Sanayi", "ug:agricultural": "Tarımsal sulama",
		"ug:lighting": "Aydınlatma", "ug:martyrs_families": "Şehit aileleri", "ug:public_lighting": "Genel aydınlatma",
		"vl:lv": "AG", "vl:mv": "OG", "term:monomial": "Tek terimli", "term:binomial": "Çift terimli",
		"pt:single_time": "Tek zamanlı", "pt:multi_time": "Üç zamanlı", "sc:incumbent": "Görevli tedarik", "sc:private": "Serbest tedarik",

		"net": "Net tüketim", "import": "Çekiş", "export": "Veriş", "max_demand": "Maksimum demant", "contract": "Sözleşme",
		"no_demand": "Demant verisi yok", "inductive_ratio": "Endüktif oran", "capacitive_ratio": "Kapasitif oran",
		"limit": "Sınır", "no_limit": "Sınır uygulanmadı",

		"lines": "Fatura kalemleri", "item": "Kalem", "quantity": "Miktar", "unit_price": "Birim fiyat", "amount": "Tutar",
		"vat_base": "KDV matrahı", "unit:period": "dönem",
		"sale_note": "mahsup sonrası şebekeye verilen fazla üretim; ödenecek tutara dahil değildir.",
		"footnote":  "Tutarlar yuvarlanmamış miktar ve birim fiyattan hesaplanır; kayıtlı 4/6 haneli değerlerden yeniden hesaplamada kuruş farkı olabilir.",

		"reactive": "Reaktif", "inductive": "Endüktif", "capacitive": "Kapasitif", "reactive_price": "Reaktif birim fiyat",
		"st:charged": "Ceza uygulandı", "st:waived": "İlk ihlal — tenzil edildi", "st:within": "Sınır içinde", "st:exempt": "Muaf",
		"st:no_power": "Kurulu güç tanımsız", "st:no_data": "Reaktif veri eksik", "st:company_charged": "Binalarda ceza var",
		"st:company_none": "Ceza yok",

		"hours": "Saat (eşleşen / beklenen)", "hours_missing": "Eksik saat", "ptf_average": "Ortalama PTF", "yekdem": "YEKDEM",
		"energy_card": "Enerji", "effective_price": "Ortalama birim fiyat", "t1": "T1 gündüz", "t2": "T2 puant", "t3": "T3 gece",
		"low_tier": "Düşük kademe", "high_tier": "Yüksek kademe",

		"readings": "Endeksler", "register": "Endeks", "start": "İlk", "end": "Son", "diff": "Fark",
		"members": "Sayaçlar", "installation": "Tesisat no", "name": "Ad",
		"footer_note": "ekokod ölçüm verisinden hesaplanmıştır; resmi fatura yerine geçmez.", "computed": "Hesaplama",
		"bill_no": "Fatura no", "page": "Sayfa",

		"flag:ptf_data_missing": "PTF verisi eksik", "flag:reactive_data_missing": "Reaktif veri eksik",
		"flag:installed_power_missing": "Kurulu güç tanımsız", "flag:generation_data_missing": "Üretim verisi eksik",
		"flag:tiering_price_missing": "Kademe fiyatı eksik",

		"m01": "Ocak", "m02": "Şubat", "m03": "Mart", "m04": "Nisan", "m05": "Mayıs", "m06": "Haziran",
		"m07": "Temmuz", "m08": "Ağustos", "m09": "Eylül", "m10": "Ekim", "m11": "Kasım", "m12": "Aralık",

		"reg:active_import": "Aktif çekiş (kWh)", "reg:t1_import": "T1 gündüz (kWh)", "reg:t2_import": "T2 puant (kWh)", "reg:t3_import": "T3 gece (kWh)",
		"reg:reactive_inductive_import": "Endüktif reaktif (kVArh)", "reg:reactive_capacitive_import": "Kapasitif reaktif (kVArh)",
		"reg:active_export": "Aktif veriş (kWh)", "reg:t1_export": "Veriş T1 (kWh)", "reg:t2_export": "Veriş T2 (kWh)", "reg:t3_export": "Veriş T3 (kWh)",
		"reg:reactive_inductive_export": "Veriş endüktif (kVArh)", "reg:reactive_capacitive_export": "Veriş kapasitif (kVArh)",
	},
	"en": {
		model.BillLineEnergy: "Energy", model.BillLineEnergyLowTier: "Energy (low tier)", model.BillLineEnergyHighTier: "Energy (high tier)",
		model.BillLineEnergyT1: "Energy T1 (day)", model.BillLineEnergyT2: "Energy T2 (peak)", model.BillLineEnergyT3: "Energy T3 (night)",
		model.BillLineDistribution: "Distribution", model.BillLineGreenEnergy: "Green energy", model.BillLinePower: "Power",
		model.BillLineDemandOverrun: "Demand overrun", model.BillLineReactiveInductive: "Reactive (inductive)",
		model.BillLineReactiveCapacitive: "Reactive (capacitive)", model.BillLineReactiveWaiver: "Reactive waived (first violation of the year)",
		model.BillLineVat: "VAT", model.BillLineGenerationCredit: "Generation credit", model.BillLineGenerationSale: "Generation sale (info)",
		model.BillLineExtraPrefix: "Extra charge", model.BillLineTaxPrefix: "Tax/fund",

		"title": "Electricity Invoice", "amount_due": "Amount due", "vat_included": "VAT included", "days": "days",
		"preview": "PREVIEW — not issued", "missing_days": "Incomplete data — this bill does not cover the whole period. Days without data: ",
		"subscriber": "Subscriber", "company": "Company", "building": "Building", "scope": "Scope", "meter": "Meter", "meters": "Meters",
		"installed_power": "Installed power", "scope_analyzer": "Meter bill", "scope_building": "Building bill", "scope_company": "Company bill",
		"tariff": "Tariff", "tariff_name": "Name", "user_group": "User group", "voltage_term": "Voltage / term",
		"price_type": "Time of use", "supply": "Supply", "contracted_power": "Contracted power", "tariff_date": "Effective",
		"per_building": "Each building on its own tariff", "ptf_yekdem": "PTF + YEKDEM",
		"ug:residential": "Residential", "ug:residential_plus": "Residential (plus)", "ug:commercial": "Commercial",
		"ug:commercial_plus": "Commercial (plus)", "ug:industrial": "Industrial", "ug:agricultural": "Agricultural irrigation",
		"ug:lighting": "Lighting", "ug:martyrs_families": "Martyrs' families", "ug:public_lighting": "Public lighting",
		"vl:lv": "LV", "vl:mv": "MV", "term:monomial": "Monomial", "term:binomial": "Binomial",
		"pt:single_time": "Single rate", "pt:multi_time": "Three-rate", "sc:incumbent": "Incumbent supplier", "sc:private": "Private supplier",

		"net": "Net consumption", "import": "Import", "export": "Export", "max_demand": "Max demand", "contract": "Contract",
		"no_demand": "No demand data", "inductive_ratio": "Inductive ratio", "capacitive_ratio": "Capacitive ratio",
		"limit": "Limit", "no_limit": "No limit applied",

		"lines": "Invoice lines", "item": "Item", "quantity": "Quantity", "unit_price": "Unit price", "amount": "Amount",
		"vat_base": "VAT base", "unit:period": "period",
		"sale_note": "excess generation fed to the grid after netting; not part of the amount due.",
		"footnote":  "Amounts are computed from unrounded quantities and prices; re-deriving them from the stored 4/6-decimal values can differ by a kuruş.",

		"reactive": "Reactive", "inductive": "Inductive", "capacitive": "Capacitive", "reactive_price": "Reactive unit price",
		"st:charged": "Penalty charged", "st:waived": "First violation — waived", "st:within": "Within limits", "st:exempt": "Exempt",
		"st:no_power": "Installed power unknown", "st:no_data": "Reactive data missing", "st:company_charged": "Charged in buildings",
		"st:company_none": "No penalty",

		"hours": "Matched / expected hours", "hours_missing": "Hours missing", "ptf_average": "Average PTF", "yekdem": "YEKDEM",
		"energy_card": "Energy", "effective_price": "Average unit price", "t1": "T1 day", "t2": "T2 peak", "t3": "T3 night",
		"low_tier": "Low tier", "high_tier": "High tier",

		"readings": "Readings", "register": "Register", "start": "Start", "end": "End", "diff": "Difference",
		"members": "Meters", "installation": "Installation no", "name": "Name",
		"footer_note": "Computed by ekokod from metered data; not an official invoice.", "computed": "Computed",
		"bill_no": "Bill no", "page": "Page",

		"flag:ptf_data_missing": "PTF data missing", "flag:reactive_data_missing": "Reactive data missing",
		"flag:installed_power_missing": "Installed power missing", "flag:generation_data_missing": "Generation data missing",
		"flag:tiering_price_missing": "Tier price missing",

		"m01": "January", "m02": "February", "m03": "March", "m04": "April", "m05": "May", "m06": "June",
		"m07": "July", "m08": "August", "m09": "September", "m10": "October", "m11": "November", "m12": "December",

		"reg:active_import": "Active import (kWh)", "reg:t1_import": "T1 day (kWh)", "reg:t2_import": "T2 peak (kWh)", "reg:t3_import": "T3 night (kWh)",
		"reg:reactive_inductive_import": "Reactive inductive (kVArh)", "reg:reactive_capacitive_import": "Reactive capacitive (kVArh)",
		"reg:active_export": "Active export (kWh)", "reg:t1_export": "Export T1 (kWh)", "reg:t2_export": "Export T2 (kWh)", "reg:t3_export": "Export T3 (kWh)",
		"reg:reactive_inductive_export": "Export inductive (kVArh)", "reg:reactive_capacitive_export": "Export capacitive (kVArh)",
	},
}

// label falls back to the part after a key's prefix ("reg:custom" → "custom").
func label(locale, key string) string {
	if v, ok := labels[locale][key]; ok {
		return v
	}
	if _, rest, found := strings.Cut(key, ":"); found {
		return rest
	}
	return key
}

// normaliseLocale falls back to the product's own locale.
func normaliseLocale(locale string) string {
	if _, ok := labels[locale]; !ok {
		return "tr"
	}
	return locale
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

// monthLabel renders "2026-01" as "Ocak 2026"; an unparsable key prints as is.
func monthLabel(locale, key string) string {
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return key
	}
	return label(locale, "m"+t.Format("01")) + " " + t.Format("2006")
}

// flagText localises a bill's comma-separated flag reason.
func flagText(locale, reason string) string {
	parts := strings.Split(reason, ",")
	for i, p := range parts {
		parts[i] = label(locale, "flag:"+strings.TrimSpace(p))
	}
	return strings.Join(parts, ", ")
}
