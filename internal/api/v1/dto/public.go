package dto

// 05 §17, F12a R351–R356.

// PublicBillRequest is POST /public/bill-calculator.
type PublicBillRequest struct {
	UserGroup        string   `json:"user_group" validate:"required,oneof=residential commercial industrial agricultural lighting" required:"true" enum:"residential,commercial,industrial,agricultural,lighting"`
	VoltageLevel     string   `json:"voltage_level" validate:"required,oneof=lv mv" required:"true" enum:"lv,mv"`
	Term             string   `json:"term" validate:"required,oneof=monomial binomial" required:"true" enum:"monomial,binomial"`
	MultiTime        bool     `json:"multi_time"`
	Start            *Date    `json:"start" validate:"required" required:"true"`
	End              *Date    `json:"end" validate:"required" required:"true"`
	TotalConsumption *Decimal `json:"total_consumption,omitempty"`
	T1               *Decimal `json:"t1,omitempty"`
	T2               *Decimal `json:"t2,omitempty"`
	T3               *Decimal `json:"t3,omitempty"`
	Demand           *Decimal `json:"demand,omitempty"`
	ContractPower    *Decimal `json:"contract_power,omitempty"`
}

// PublicBillTariff names the schedule row that priced the estimate.
type PublicBillTariff struct {
	EffectiveFrom Date    `json:"effective_from" required:"true"`
	GroupUsed     string  `json:"group_used" required:"true"`
	Source        *string `json:"source,omitempty"`
}

// PublicBill is R352's breakdown.
type PublicBill struct {
	Energy       Decimal          `json:"energy" required:"true"`
	Distribution Decimal          `json:"distribution" required:"true"`
	Power        Decimal          `json:"power" required:"true"`
	Overuse      Decimal          `json:"overuse" required:"true"`
	VatBase      Decimal          `json:"vat_base" required:"true"`
	Vat          Decimal          `json:"vat" required:"true"`
	Total        Decimal          `json:"total" required:"true"`
	VatRate      Decimal          `json:"vat_rate" required:"true"`
	Days         int              `json:"days" required:"true"`
	Basis        string           `json:"basis" required:"true" enum:"total,bands"`
	Tariff       PublicBillTariff `json:"tariff" required:"true"`
}
