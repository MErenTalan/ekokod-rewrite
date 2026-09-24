package carbonview

type labels struct {
	ghgTitle, isoTitle, company, building, address, period, sub, total, noRecords string
	totalLine, pending, gridTitle, item, value, consumption, generation, factor   string
	source, consumptionT, reductionT, netT, noMeter, generated                    string
	groups, subs                                                                  map[string]string
}

//nolint:misspell // Turkish copy ("Adres", "Personel"), not English typos
var catalogue = map[string]labels{
	"tr": {
		ghgTitle: "GHG Protokolü Karbon Ayak İzi Raporu", isoTitle: "ISO 14064 Karbon Ayak İzi Raporu",
		company: "Şirket", building: "Bina", address: "Adres", period: "Dönem", sub: "Alt kategori", total: "Toplam",
		noRecords: "Bu grupta kayıt yok.", totalLine: "Toplam: %s kg CO₂e", pending: "%d kayıt onay bekliyor; rapora dahildir.",
		gridTitle: "Şebeke elektriği özeti", item: "Kalem", value: "Değer", consumption: "Şebekeden tüketim",
		generation: "Üretim (sayaç ihracatı)", factor: "Şebeke emisyon faktörü", source: "Faktör kaynağı",
		consumptionT: "Tüketim kaynaklı emisyon", reductionT: "Üretim kaynaklı azaltım", netT: "Net emisyon",
		noMeter: "Sayaç verisinden otomatik kayıt yok; şebeke özeti hesaplanamadı.", generated: "Oluşturulma",
		groups: map[string]string{
			"scope_1": "Kapsam 1 — Doğrudan emisyonlar", "scope_2": "Kapsam 2 — Enerji kaynaklı dolaylı emisyonlar",
			"scope_3":    "Kapsam 3 — Diğer dolaylı emisyonlar",
			"category_1": "Kategori 1 — Doğrudan emisyonlar ve uzaklaştırmalar", "category_2": "Kategori 2 — İthal edilen enerjiden dolaylı emisyonlar",
			"category_3": "Kategori 3 — Taşımacılıktan dolaylı emisyonlar", "category_4": "Kategori 4 — Kullanılan ürünlerden dolaylı emisyonlar",
			"category_5": "Kategori 5 — Kuruluşun ürünlerinin kullanımından dolaylı emisyonlar", "category_6": "Kategori 6 — Diğer kaynaklardan dolaylı emisyonlar",
		},
		subs: map[string]string{
			"sub_space_heating": "Ortam Isıtması", "sub_process_combustion": "Proses Amaçlı Yanma", "sub_other_combustion": "Diğer Yanma",
			"sub_passenger_transport": "Personel Taşımacılığı", "sub_business_travel": "İş Seyahati",
			"sub_employee_commuting": "Çalışanların İşe Geliş Gidişleri", "sub_inbound_freight": "Gelen Mal Taşıma",
			"sub_outbound_freight": "Giden Mal Taşıma", "sub_process_emissions": "Proses Emisyonları",
			"sub_fugitive_emissions": "Kaçak Gaz Emisyonları", "sub_grid_electricity": "Şebekeden Elektrik Tüketimi",
			"sub_elec_generation": "Elektrik Üretimi", "sub_purchased_hc": "Satın Alınan Isıtma / Soğutma",
			"sub_purchased_steam": "Satın Alınan Buhar", "sub_purchased_goods": "Satın Alınan Ürün ve Hizmetler",
			"sub_eol_sold_products": "Satılan Ürünlerin Yaşam Sonu", "sub_capital_goods": "Sermaye Malları",
			"sub_waste_disposal": "Atık Bertarafı",
		},
	},
	"en": {
		ghgTitle: "GHG Protocol Carbon Footprint Report", isoTitle: "ISO 14064 Carbon Footprint Report",
		company: "Company", building: "Building", address: "Address", period: "Period", sub: "Sub-category", total: "Total",
		noRecords: "No records in this group.", totalLine: "Total: %s kg CO₂e", pending: "%d records await approval; they are included.",
		gridTitle: "Grid electricity summary", item: "Item", value: "Value", consumption: "Grid consumption",
		generation: "Generation (meter export)", factor: "Grid emission factor", source: "Factor source",
		consumptionT: "Consumption emission", reductionT: "Production reduction", netT: "Net emission",
		noMeter: "No automated meter records; the grid summary is unavailable.", generated: "Generated",
		groups: map[string]string{
			"scope_1": "Scope 1 — Direct emissions", "scope_2": "Scope 2 — Energy indirect emissions",
			"scope_3":    "Scope 3 — Other indirect emissions",
			"category_1": "Category 1 — Direct emissions and removals", "category_2": "Category 2 — Indirect emissions from imported energy",
			"category_3": "Category 3 — Indirect emissions from transportation", "category_4": "Category 4 — Indirect emissions from products used",
			"category_5": "Category 5 — Indirect emissions from the use of products", "category_6": "Category 6 — Indirect emissions from other sources",
		},
		subs: map[string]string{
			"sub_space_heating": "Space heating", "sub_process_combustion": "Process combustion", "sub_other_combustion": "Other combustion",
			"sub_passenger_transport": "Passenger transport", "sub_business_travel": "Business travel",
			"sub_employee_commuting": "Employee commuting", "sub_inbound_freight": "Inbound freight",
			"sub_outbound_freight": "Outbound freight", "sub_process_emissions": "Process emissions",
			"sub_fugitive_emissions": "Fugitive emissions", "sub_grid_electricity": "Grid electricity",
			"sub_elec_generation": "Electricity generation", "sub_purchased_hc": "Purchased heating / cooling",
			"sub_purchased_steam": "Purchased steam", "sub_purchased_goods": "Purchased goods and services",
			"sub_eol_sold_products": "End-of-life of sold products", "sub_capital_goods": "Capital goods",
			"sub_waste_disposal": "Waste disposal",
		},
	},
}
