package dto

import (
	"time"

	"github.com/google/uuid"
)

// Bill is a stored invoice.
type Bill struct {
	ID                     uuid.UUID  `json:"id" required:"true"`
	Scope                  string     `json:"scope" required:"true" enum:"analyzer,building,company"`
	BuildingID             *uuid.UUID `json:"building_id"`
	AnalyzerID             *uuid.UUID `json:"analyzer_id"`
	PeriodKey              string     `json:"period_key" required:"true"`
	PeriodStart            time.Time  `json:"period_start" required:"true"`
	PeriodEnd              time.Time  `json:"period_end" required:"true"`
	DaysInPeriod           int32      `json:"days_in_period" required:"true"`
	TariffID               *uuid.UUID `json:"tariff_id"`
	ActiveImport           Decimal    `json:"active_import" required:"true"`
	NetConsumption         Decimal    `json:"net_consumption" required:"true"`
	InductiveKvarh         Decimal    `json:"inductive_kvarh" required:"true"`
	CapacitiveKvarh        Decimal    `json:"capacitive_kvarh" required:"true"`
	MaxDemandKw            *Decimal   `json:"max_demand_kw"`
	EnergyCost             Decimal    `json:"energy_cost" required:"true"`
	DistributionCost       Decimal    `json:"distribution_cost" required:"true"`
	PowerCost              Decimal    `json:"power_cost" required:"true"`
	ReactivePenalty        Decimal    `json:"reactive_penalty" required:"true"`
	VatCost                Decimal    `json:"vat_cost" required:"true"`
	TotalCost              Decimal    `json:"total_cost" required:"true"`
	Currency               string     `json:"currency" required:"true"`
	InductiveRatio         *Decimal   `json:"inductive_ratio"`
	CapacitiveRatio        *Decimal   `json:"capacitive_ratio"`
	ReactivePenaltyApplied bool       `json:"reactive_penalty_applied" required:"true"`
	PtfYekdemUsed          bool       `json:"ptf_yekdem_used" required:"true"`
	Status                 string     `json:"status" required:"true" enum:"draft,issued,flagged,superseded"`
	FlagReason             *string    `json:"flag_reason"`
	HasPDF                 bool       `json:"has_pdf" required:"true"`
	ComputedAt             time.Time  `json:"computed_at" required:"true"`
}

// BillLine is one invoice line.
type BillLine struct {
	Code      string   `json:"code" required:"true"`
	Label     string   `json:"label" required:"true"`
	Quantity  *Decimal `json:"quantity"`
	Unit      *string  `json:"unit"`
	UnitPrice *Decimal `json:"unit_price"`
	RatePct   *Decimal `json:"rate_pct"`
	Amount    Decimal  `json:"amount" required:"true"`
}

// BillDetail is GET /bills/{id}.
type BillDetail struct {
	Bill
	Lines     []BillLine  `json:"lines" required:"true"`
	MemberIDs []uuid.UUID `json:"member_analyzer_ids" required:"true"`
}

// BillListRequest is GET /bills.
type BillListRequest struct {
	Scope      string     `query:"scope" json:"-" validate:"omitempty,oneof=analyzer building company" enum:"analyzer,building,company"`
	BuildingID *uuid.UUID `query:"building_id" json:"-"`
	AnalyzerID *uuid.UUID `query:"analyzer_id" json:"-"`
	Period     string     `query:"period" json:"-" validate:"omitempty,datetime=2006-01"`
	Status     []string   `query:"status" json:"-"`
	PageRequest
}

// BillHourlyRequest is GET /bills/{id}/hourly-detail.
type BillHourlyRequest struct {
	ID     uuid.UUID `path:"id" json:"-"`
	Format string    `query:"format" json:"-" validate:"omitempty,oneof=json xlsx" enum:"json,xlsx"`
}

// BillHour is one priced hour.
type BillHour struct {
	Ts          time.Time `json:"ts" required:"true"`
	Consumption Decimal   `json:"consumption" required:"true"`
	PTF         Decimal   `json:"ptf" required:"true"`
	Yekdem      Decimal   `json:"yekdem" required:"true"`
	Kbk         Decimal   `json:"kbk" required:"true"`
	UnitPrice   Decimal   `json:"unit_price" required:"true"`
	Cost        Decimal   `json:"cost" required:"true"`
}

// BillHours is the JSON hourly detail.
type BillHours struct {
	Items []BillHour `json:"items" required:"true"`
}

// BillComputeRequest is POST /bills/compute.
type BillComputeRequest struct {
	Scope       string      `json:"scope" validate:"required,oneof=analyzer building company" required:"true" enum:"analyzer,building,company"`
	BuildingIDs []uuid.UUID `json:"building_ids,omitempty" validate:"max=500"`
	AnalyzerIDs []uuid.UUID `json:"analyzer_ids,omitempty" validate:"max=500"`
	Period      string      `json:"period" validate:"required,datetime=2006-01" required:"true" pattern:"^[0-9]{4}-[0-9]{2}$"`
	Force       bool        `json:"force"`
}

// BillComputeAccepted lists the enqueued jobs.
type BillComputeAccepted struct {
	JobIDs []string `json:"job_ids" required:"true"`
}

// BillLatestRequest is GET /bills/latest.
type BillLatestRequest struct {
	Scope     string    `query:"scope" json:"-" validate:"required,oneof=analyzer building company" required:"true" enum:"analyzer,building,company"`
	SubjectID uuid.UUID `query:"subject_id" json:"-" validate:"required" required:"true"`
}
