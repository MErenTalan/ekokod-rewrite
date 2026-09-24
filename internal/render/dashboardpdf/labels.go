package dashboardpdf

type labels struct {
	title, period, subtotal, buildingBill, divergence string
	netting, consumption, production, net, invoice    string
	netConsumption, netProduction                     string
	efficiency, unavailable, plants, noData           string
	totalProduction, totalSale, sale, feedIn          string
	columns                                           []string
}

var catalogue = map[string]labels{
	"tr": {
		title: "Fatura Panosu", period: "Dönem", subtotal: "Ara toplam", buildingBill: "Bina faturası",
		divergence: "Bina faturası, analizör faturalarının toplamından farklı: bina tarifesi toplam tüketime bir kez uygulanır (02 §6.11).",
		netting:    "Netleşme", consumption: "tüketim", production: "üretim", net: "net", invoice: "fatura",
		netConsumption: "net tüketim", netProduction: "net üretim",
		efficiency: "Verimlilik", unavailable: "hesaplanamadı", plants: "GES santralleri",
		noData: "veri yok", totalProduction: "Toplam üretim", totalSale: "Toplam satış", sale: "satış", feedIn: "üretim fiyatı",
		columns: []string{"Dönem", "Bina", "Sayaç", "Tesisat no", "ETSO", "Tüketim (kWh)", "Üretim (kWh)", "Birim fiyat", "Fatura"},
	},
	"en": {
		title: "Invoice Dashboard", period: "Period", subtotal: "Subtotal", buildingBill: "Building invoice",
		divergence: "The building invoice differs from the sum of the analyzer invoices: the building tariff is applied once to the aggregate (02 §6.11).",
		netting:    "Netting", consumption: "consumption", production: "production", net: "net", invoice: "invoice",
		netConsumption: "net consumption", netProduction: "net production",
		efficiency: "Efficiency", unavailable: "not available", plants: "Solar plants (GES)",
		noData: "no data", totalProduction: "Total production", totalSale: "Total sale", sale: "sale", feedIn: "production price",
		columns: []string{"Period", "Building", "Meter", "Installation no", "ETSO", "Consumption (kWh)", "Production (kWh)", "Unit price", "Invoice"},
	},
}
