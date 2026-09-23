package billing

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// The invoice dashboard of 01 §7.10 is derived, never stored. Its only input
// is the period's bills (R233), so the figure on the screen and the figure on
// the invoice are the same number — and every total here is arithmetic over
// the rows the screen shows, which is what 09 §F8 asks to be tested.

// divergenceTolerance is the kurus below which a building invoice and the sum
// of its analyzer invoices are the same number for a reader (R234).
var divergenceTolerance = decimal.RequireFromString("0.01")

// DashboardRow is one invoice as the dashboard lists it. Names are resolved by
// the caller: a bill whose building or analyzer was deleted after it was
// issued still has a row, with whatever name survives.
type DashboardRow struct {
	BillID     uuid.UUID
	BuildingID *uuid.UUID
	AnalyzerID *uuid.UUID

	BuildingName       string
	AnalyzerName       string
	InstallationNumber string
	EtsoCode           string
	PeriodKey          string

	Consumption decimal.Decimal
	Production  decimal.Decimal
	// ConsumptionPrice and ProductionPrice are the unit prices the bill
	// actually used; either may be absent (a PTF bill has no single energy
	// price to name, a tariff without generation has no production price).
	ConsumptionPrice *decimal.Decimal
	ProductionPrice  *decimal.Decimal

	Invoice    decimal.Decimal
	Currency   model.CurrencyCode
	Superseded bool
}

// DashboardBuilding is one building's section: its analyzer rows and their
// sums. BuildingBill is the building-scope invoice when one exists for the
// period; 02 §6.11 prices it over the aggregate, so it may differ from the sum
// of the rows, and the difference is shown rather than reconciled (R234).
type DashboardBuilding struct {
	BuildingID   uuid.UUID
	BuildingName string
	Rows         []DashboardRow

	TotalConsumption decimal.Decimal
	TotalProduction  decimal.Decimal
	TotalInvoice     decimal.Decimal
	Currency         model.CurrencyCode

	BuildingBill     *DashboardRow
	DivergesFromRows bool
}

// DashboardNetting is the §7.10 summary, one per currency present (R253):
// R127 leaves this system without an exchange rate, so nothing is ever summed
// across currencies.
type DashboardNetting struct {
	Currency         model.CurrencyCode
	TotalConsumption decimal.Decimal
	TotalProduction  decimal.Decimal
	Net              decimal.Decimal
	// NetStatus is "net_consumption" when Net is not negative, else
	// "net_production".
	NetStatus    string
	TotalInvoice decimal.Decimal
	// EfficiencyPct is production over consumption as a percentage, absent
	// when there was no consumption to compare against.
	EfficiencyPct *decimal.Decimal
	PeriodKey     string
	CompanyBillID *uuid.UUID
}

// Net status values.
const (
	NetStatusConsumption = "net_consumption"
	NetStatusProduction  = "net_production"
)

// DashboardResult is one month.
type DashboardResult struct {
	Buildings []DashboardBuilding
	Netting   []DashboardNetting
}

// BuildDashboard groups the period's analyzer rows by building, sums them, and
// nets them per currency. buildingBills and companyBills are the same period's
// building- and company-scope invoices, shown beside the rows rather than
// mixed into them.
func BuildDashboard(rows, buildingBills, companyBills []DashboardRow) DashboardResult {
	out := DashboardResult{Buildings: groupByBuilding(rows, buildingBills)}
	out.Netting = netPerCurrency(rows, companyBills)
	return out
}

func groupByBuilding(rows, buildingBills []DashboardRow) []DashboardBuilding {
	var order []uuid.UUID
	byID := map[uuid.UUID]*DashboardBuilding{}
	for _, r := range rows {
		key := uuid.Nil
		if r.BuildingID != nil {
			key = *r.BuildingID
		}
		b := byID[key]
		if b == nil {
			b = &DashboardBuilding{BuildingID: key, BuildingName: r.BuildingName, Currency: r.Currency,
				TotalConsumption: decimal.Zero, TotalProduction: decimal.Zero, TotalInvoice: decimal.Zero}
			byID[key], order = b, append(order, key)
		}
		b.Rows = append(b.Rows, r)
		b.TotalConsumption = b.TotalConsumption.Add(r.Consumption)
		b.TotalProduction = b.TotalProduction.Add(r.Production)
		b.TotalInvoice = b.TotalInvoice.Add(r.Invoice)
	}
	for _, bill := range buildingBills {
		if bill.BuildingID == nil {
			continue
		}
		b := byID[*bill.BuildingID]
		if b == nil {
			continue
		}
		copied := bill
		b.BuildingBill = &copied
		b.DivergesFromRows = bill.Invoice.Sub(b.TotalInvoice).Abs().GreaterThan(divergenceTolerance)
	}
	out := make([]DashboardBuilding, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func netPerCurrency(rows, companyBills []DashboardRow) []DashboardNetting {
	var order []model.CurrencyCode
	byCurrency := map[model.CurrencyCode]*DashboardNetting{}
	for _, r := range rows {
		n := byCurrency[r.Currency]
		if n == nil {
			n = &DashboardNetting{Currency: r.Currency, PeriodKey: r.PeriodKey, TotalConsumption: decimal.Zero,
				TotalProduction: decimal.Zero, TotalInvoice: decimal.Zero}
			byCurrency[r.Currency], order = n, append(order, r.Currency)
		}
		n.TotalConsumption = n.TotalConsumption.Add(r.Consumption)
		n.TotalProduction = n.TotalProduction.Add(r.Production)
		n.TotalInvoice = n.TotalInvoice.Add(r.Invoice)
	}
	for _, bill := range companyBills {
		if n := byCurrency[bill.Currency]; n != nil && n.CompanyBillID == nil {
			id := bill.BillID
			n.CompanyBillID = &id
		}
	}
	out := make([]DashboardNetting, 0, len(order))
	for _, c := range order {
		n := byCurrency[c]
		n.Net = n.TotalConsumption.Sub(n.TotalProduction)
		n.NetStatus = NetStatusConsumption
		if n.Net.IsNegative() {
			n.NetStatus = NetStatusProduction
		}
		if n.TotalConsumption.IsPositive() {
			pct := n.TotalProduction.Div(n.TotalConsumption).Mul(hundred).Round(2)
			n.EfficiencyPct = &pct
		}
		out = append(out, *n)
	}
	return out
}
