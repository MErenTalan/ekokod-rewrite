package reportview

type labels struct {
	monthlyTitle, yearlyTitle, company, buildings, period                           string
	noData, notSelected, coverage, buildingsWord, plantsWord                        string
	info, summary, consumptionChart, billChart                                      string
	infoRows                                                                        [15]string
	summaryRows                                                                     [4]string
	currentYear, previousYear                                                       string
	consumptionTable, month, consumption, rooftop, bill, reactive, total            string
	dailyConsumption, dailyRooftop                                                  string
	solar, solarTotal, target, achievement, dailyProduction                         string
	yearlySummary                                                                   [4]string
	historyChart, yearlyConsumption, yearlyProduction, yearlyBillChart, targetChart string
	targetSeries, actualSeries                                                      string
	comparison, compConsumption, compProduction, solarShare, gridShare              string
	carbon, carbonConsumption, carbonReduction, carbonNet, carbonFactor, carbonNote string
	carbonUnavailable, sourceYear, partial, omitted, noChartData                    string
	kwhMonth, kwhYear, kwhDay, moneyMonth, moneyYear, tonYear, perKwh               string
	months                                                                          [12]string
}

var catalogue = map[string]labels{
	"tr": {
		monthlyTitle: "Aylık Elektrik Tüketim ve Üretim Raporu", yearlyTitle: "Yıllık Kaynak Tüketim Raporu",
		company: "Şirket", buildings: "Bina(lar)", period: "Rapor dönemi",
		noData: "veri yok", notSelected: "seçim dışı", coverage: "%d / %d %s", buildingsWord: "bina", plantsWord: "santral",
		info: "Bilgi tablosu", summary: "Özet", consumptionChart: "Aylık elektrik tüketimi", billChart: "Aylık elektrik faturası",
		infoRows: [15]string{"Rapor dönemi", "Bina(lar)", "Elektrik tarifesi", "Elektrik alış fiyatı", "Ortalama alış fiyatı",
			"Çatı GES satış fiyatı", "Arazi GES satış fiyatı", "Aylık toplam tüketim", "Günlük ortalama tüketim",
			"Çatı GES aylık üretimi", "Arazi GES aylık üretimi", "Toplam üretim", "Günlük ortalama üretim",
			"Elektrik faturası", "Reaktif ceza"},
		summaryRows: [4]string{"Toplam tüketim", "Toplam üretim", "Toplam fatura", "Reaktif ceza"},
		currentYear: "Bu yıl", previousYear: "Geçen yıl",
		consumptionTable: "Elektrik tüketimi ve çatı GES üretimi", month: "Ay", consumption: "Tüketim (kWh)",
		rooftop: "Çatı GES üretimi (kWh)", bill: "Elektrik faturası", reactive: "Reaktif ceza", total: "Yıllık toplam",
		dailyConsumption: "Günlük ortalama tüketim (kWh/gün)", dailyRooftop: "Çatı GES günlük ortalama üretim (kWh/gün)",
		solar: "Güneş enerjisi üretim raporu", solarTotal: "Toplam yıllık üretim", target: "Hedeflenen üretim",
		achievement: "Hedef gerçekleşme oranı", dailyProduction: "Günlük ortalama üretim",
		yearlySummary: [4]string{"Yıllık tüketim", "Yıllık üretim", "Yıllık fatura", "Hedef gerçekleşme"},
		historyChart:  "Yıllık elektrik tüketimi ve üretimi", yearlyConsumption: "Yıllık tüketim", yearlyProduction: "Yıllık üretim",
		yearlyBillChart: "Yıllık elektrik faturası", targetChart: "Hedeflenen ve gerçekleşen üretim (santral bazında)",
		targetSeries: "Hedef", actualSeries: "Gerçekleşen",
		comparison: "Nihai karşılaştırma", compConsumption: "Yıllık tüketim", compProduction: "Yıllık üretim",
		solarShare: "GES ile karşılanan", gridShare: "Şebekeden alınan",
		carbon: "Karbon emisyonu", carbonConsumption: "Elektrik tüketiminden kaynaklanan emisyon",
		carbonReduction: "Elektrik üretiminden kaynaklanan emisyon azaltımı", carbonNet: "Nihai emisyon",
		carbonFactor: "Kullanılan emisyon faktörü", carbonNote: "Türkiye şebeke ortalaması",
		carbonUnavailable: "Şebeke emisyon faktörü tanımlı değil; karbon bölümü hesaplanamadı.",
		sourceYear:        "kaynak yılı", partial: "Bu rapor henüz kapanmamış ya da tazelenmemiş bir dönem içeriyor; rakamlar kesinleşmedi.",
		omitted: "Grafikte gösterilmeyen para birimleri", noChartData: "Bu grafik için veri yok.",
		kwhMonth: "kWh/ay", kwhYear: "kWh/yıl", kwhDay: "kWh/gün", moneyMonth: "%s/ay", moneyYear: "%s/yıl", tonYear: "ton/yıl", perKwh: "TL/kWh",
		months: [12]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"},
	},
	"en": {
		monthlyTitle: "Monthly Electricity Consumption and Production Report", yearlyTitle: "Yearly Resource Consumption Report",
		company: "Company", buildings: "Building(s)", period: "Report period",
		noData: "no data", notSelected: "not selected", coverage: "%d of %d %s", buildingsWord: "buildings", plantsWord: "plants",
		info: "Information", summary: "Summary", consumptionChart: "Monthly electricity consumption", billChart: "Monthly electricity bill",
		infoRows: [15]string{"Report period", "Building(s)", "Electricity tariff", "Purchase price", "Average purchase price",
			"Rooftop PV feed-in price", "Utility-scale PV feed-in price", "Monthly total consumption", "Daily average consumption",
			"Rooftop PV monthly production", "Utility-scale PV monthly production", "Total production", "Daily average production",
			"Electricity bill", "Reactive penalty"},
		summaryRows: [4]string{"Total consumption", "Total production", "Total bill", "Reactive penalty"},
		currentYear: "This year", previousYear: "Last year",
		consumptionTable: "Electricity consumption and rooftop PV production", month: "Month", consumption: "Consumption (kWh)",
		rooftop: "Rooftop PV production (kWh)", bill: "Electricity bill", reactive: "Reactive penalty", total: "Yearly total",
		dailyConsumption: "Average daily consumption (kWh/day)", dailyRooftop: "Average daily rooftop PV production (kWh/day)",
		solar: "Solar production report", solarTotal: "Total yearly production", target: "Target production",
		achievement: "Target achievement rate", dailyProduction: "Average daily production",
		yearlySummary: [4]string{"Yearly consumption", "Yearly production", "Yearly bill", "Target achievement"},
		historyChart:  "Yearly electricity consumption and production", yearlyConsumption: "Yearly consumption", yearlyProduction: "Yearly production",
		yearlyBillChart: "Yearly electricity bill", targetChart: "Target vs. actual production (per plant)",
		targetSeries: "Target", actualSeries: "Actual",
		comparison: "Final comparison", compConsumption: "Yearly consumption", compProduction: "Yearly production",
		solarShare: "Met by solar", gridShare: "Taken from the grid",
		carbon: "Carbon emissions", carbonConsumption: "Emissions from electricity consumption",
		carbonReduction: "Emission reduction from electricity generation", carbonNet: "Net emissions",
		carbonFactor: "Emission factor used", carbonNote: "Turkey grid average",
		carbonUnavailable: "No grid emission factor is configured; the carbon section cannot be computed.",
		sourceYear:        "source year", partial: "This report includes a period that has not closed or been refreshed yet; its figures are provisional.",
		omitted: "Currencies not shown in the chart", noChartData: "There is no data for this chart.",
		kwhMonth: "kWh/month", kwhYear: "kWh/year", kwhDay: "kWh/day", moneyMonth: "%s/month", moneyYear: "%s/year", tonYear: "t/year", perKwh: "TL/kWh",
		months: [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	},
}
