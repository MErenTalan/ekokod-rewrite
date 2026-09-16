package model

// The enum types below mirror the SQL enums declared in the migrations
// one-for-one: same set of values, same spelling. They are named string types
// rather than bare strings so that a building_admin can never be assigned to a
// field expecting a bill status, and so that a value read from the database
// can be compared against a constant instead of a string literal.
//
// Each type carries a Valid method. Postgres already rejects an unknown value
// on write, so Valid is not the primary defence; it is what lets a handler
// reject a bad value from an HTTP request before a round trip, and what makes
// an exhaustive switch's default branch reachable in tests.

// UserRole mirrors the SQL enum user_role (migration 00002).
type UserRole string

// The roles of user_role, in the order the enum declares them. The two
// readonly roles are distinct values rather than a boolean on the role,
// because the schema models them that way.
const (
	UserRoleAdmin                 UserRole = "admin"
	UserRoleCompanyAdmin          UserRole = "company_admin"
	UserRoleCompanyReadonlyAdmin  UserRole = "company_readonly_admin"
	UserRoleBuildingAdmin         UserRole = "building_admin"
	UserRoleBuildingReadonlyAdmin UserRole = "building_readonly_admin"
	UserRoleDemo                  UserRole = "demo"
)

// UserRoles is every value of user_role, in declaration order.
func UserRoles() []UserRole {
	return []UserRole{
		UserRoleAdmin,
		UserRoleCompanyAdmin,
		UserRoleCompanyReadonlyAdmin,
		UserRoleBuildingAdmin,
		UserRoleBuildingReadonlyAdmin,
		UserRoleDemo,
	}
}

// Valid reports whether r is one of the declared roles.
func (r UserRole) Valid() bool { return contains(UserRoles(), r) }

// IntegrationProvider mirrors the SQL enum integration_provider
// (migration 00003).
type IntegrationProvider string

// The providers of integration_provider.
const (
	IntegrationProviderOSOS    IntegrationProvider = "osos"
	IntegrationProviderGridbox IntegrationProvider = "gridbox"
	IntegrationProviderARIL    IntegrationProvider = "aril"
	IntegrationProviderPM5340  IntegrationProvider = "pm5340"
	IntegrationProviderISolar  IntegrationProvider = "isolar"
)

// IntegrationProviders is every value of integration_provider.
func IntegrationProviders() []IntegrationProvider {
	return []IntegrationProvider{
		IntegrationProviderOSOS,
		IntegrationProviderGridbox,
		IntegrationProviderARIL,
		IntegrationProviderPM5340,
		IntegrationProviderISolar,
	}
}

// Valid reports whether p is one of the declared providers.
func (p IntegrationProvider) Valid() bool { return contains(IntegrationProviders(), p) }

// PanelOrientation mirrors the SQL enum panel_orientation (migration 00003).
type PanelOrientation string

// The compass points of panel_orientation.
const (
	PanelOrientationN  PanelOrientation = "n"
	PanelOrientationS  PanelOrientation = "s"
	PanelOrientationE  PanelOrientation = "e"
	PanelOrientationW  PanelOrientation = "w"
	PanelOrientationNE PanelOrientation = "ne"
	PanelOrientationSE PanelOrientation = "se"
	PanelOrientationNW PanelOrientation = "nw"
	PanelOrientationSW PanelOrientation = "sw"
)

// PanelOrientations is every value of panel_orientation.
func PanelOrientations() []PanelOrientation {
	return []PanelOrientation{
		PanelOrientationN, PanelOrientationS, PanelOrientationE, PanelOrientationW,
		PanelOrientationNE, PanelOrientationSE, PanelOrientationNW, PanelOrientationSW,
	}
}

// Valid reports whether o is one of the declared orientations.
func (o PanelOrientation) Valid() bool { return contains(PanelOrientations(), o) }

// ReadingKind mirrors the SQL enum reading_kind (migration 00004). It is part
// of the meter_readings primary key: the same analyzer and timestamp may carry
// one row per kind.
type ReadingKind string

// The kinds of reading_kind.
const (
	// ReadingKindLoadProfile is the interval series (yük profili).
	ReadingKindLoadProfile ReadingKind = "load_profile"
	// ReadingKindDaily is the daily index snapshot.
	ReadingKindDaily ReadingKind = "daily"
	// ReadingKindBilling is the index the distributor bills on.
	ReadingKindBilling ReadingKind = "billing"
	// ReadingKindReset marks a meter reset, after which registers restart.
	ReadingKindReset ReadingKind = "reset"
	// ReadingKindCurrentIndex is the instantaneous register read.
	ReadingKindCurrentIndex ReadingKind = "current_index"
)

// ReadingKinds is every value of reading_kind.
func ReadingKinds() []ReadingKind {
	return []ReadingKind{
		ReadingKindLoadProfile, ReadingKindDaily, ReadingKindBilling,
		ReadingKindReset, ReadingKindCurrentIndex,
	}
}

// Valid reports whether k is one of the declared kinds.
func (k ReadingKind) Valid() bool { return contains(ReadingKinds(), k) }

// PriceType mirrors the SQL enum price_type (migration 00006).
type PriceType string

// The values of price_type: a single unit price, or the T1/T2/T3 triple.
const (
	PriceTypeSingleTime PriceType = "single_time"
	PriceTypeMultiTime  PriceType = "multi_time"
)

// PriceTypes is every value of price_type.
func PriceTypes() []PriceType { return []PriceType{PriceTypeSingleTime, PriceTypeMultiTime} }

// Valid reports whether p is one of the declared price types.
func (p PriceType) Valid() bool { return contains(PriceTypes(), p) }

// TariffTerm mirrors the SQL enum tariff_term (migration 00006). A binomial
// tariff charges for contracted power as well as energy.
type TariffTerm string

// The values of tariff_term.
const (
	TariffTermMonomial TariffTerm = "monomial"
	TariffTermBinomial TariffTerm = "binomial"
)

// TariffTerms is every value of tariff_term.
func TariffTerms() []TariffTerm { return []TariffTerm{TariffTermMonomial, TariffTermBinomial} }

// Valid reports whether t is one of the declared terms.
func (t TariffTerm) Valid() bool { return contains(TariffTerms(), t) }

// EnergyType mirrors the SQL enum energy_type (migration 00006).
type EnergyType string

// The values of energy_type.
const (
	EnergyTypeGrid  EnergyType = "grid_energy"
	EnergyTypeGreen EnergyType = "green_energy"
)

// EnergyTypes is every value of energy_type.
func EnergyTypes() []EnergyType { return []EnergyType{EnergyTypeGrid, EnergyTypeGreen} }

// Valid reports whether e is one of the declared energy types.
func (e EnergyType) Valid() bool { return contains(EnergyTypes(), e) }

// VoltageLevel mirrors the SQL enum voltage_level (migration 00006): alçak
// gerilim (lv) and orta gerilim (mv).
type VoltageLevel string

// The values of voltage_level.
const (
	VoltageLevelLV VoltageLevel = "lv"
	VoltageLevelMV VoltageLevel = "mv"
)

// VoltageLevels is every value of voltage_level.
func VoltageLevels() []VoltageLevel { return []VoltageLevel{VoltageLevelLV, VoltageLevelMV} }

// Valid reports whether v is one of the declared voltage levels.
func (v VoltageLevel) Valid() bool { return contains(VoltageLevels(), v) }

// SupplyCompany mirrors the SQL enum supply_company (migration 00006):
// görevli tedarik şirketi (incumbent) and serbest tedarikçi (private).
type SupplyCompany string

// The values of supply_company.
const (
	SupplyCompanyIncumbent SupplyCompany = "incumbent"
	SupplyCompanyPrivate   SupplyCompany = "private"
)

// SupplyCompanies is every value of supply_company.
func SupplyCompanies() []SupplyCompany {
	return []SupplyCompany{SupplyCompanyIncumbent, SupplyCompanyPrivate}
}

// Valid reports whether s is one of the declared supply companies.
func (s SupplyCompany) Valid() bool { return contains(SupplyCompanies(), s) }

// GenerationUsage mirrors the SQL enum generation_usage (migration 00006): how
// on-site generation is applied to a bill.
type GenerationUsage string

// The values of generation_usage.
const (
	// GenerationUsageNone ignores generation when pricing.
	GenerationUsageNone GenerationUsage = "none"
	// GenerationUsageSubtractFromConsumption nets generation off the
	// consumed energy before pricing it.
	GenerationUsageSubtractFromConsumption GenerationUsage = "subtract_from_consumption"
	// GenerationUsageSubtractFromTotal credits generation against the
	// computed total at its own price, which is why the schema requires
	// generation_price_per_kwh whenever this value is used.
	GenerationUsageSubtractFromTotal GenerationUsage = "subtract_from_total"
)

// GenerationUsages is every value of generation_usage.
func GenerationUsages() []GenerationUsage {
	return []GenerationUsage{
		GenerationUsageNone,
		GenerationUsageSubtractFromConsumption,
		GenerationUsageSubtractFromTotal,
	}
}

// Valid reports whether g is one of the declared generation usages.
func (g GenerationUsage) Valid() bool { return contains(GenerationUsages(), g) }

// CurrencyCode mirrors the SQL enum currency_code (migration 00006).
type CurrencyCode string

// The values of currency_code.
const (
	CurrencyTRY CurrencyCode = "TRY"
	CurrencyUSD CurrencyCode = "USD"
	CurrencyEUR CurrencyCode = "EUR"
)

// CurrencyCodes is every value of currency_code.
func CurrencyCodes() []CurrencyCode { return []CurrencyCode{CurrencyTRY, CurrencyUSD, CurrencyEUR} }

// Valid reports whether c is one of the declared currencies.
func (c CurrencyCode) Valid() bool { return contains(CurrencyCodes(), c) }

// DistributionUserGroup mirrors the SQL enum distribution_user_group
// (migration 00006): the abonelik grubu the published tariff schedule is keyed
// by.
type DistributionUserGroup string

// The values of distribution_user_group.
const (
	UserGroupResidential     DistributionUserGroup = "residential"
	UserGroupResidentialPlus DistributionUserGroup = "residential_plus"
	UserGroupCommercial      DistributionUserGroup = "commercial"
	UserGroupCommercialPlus  DistributionUserGroup = "commercial_plus"
	UserGroupIndustrial      DistributionUserGroup = "industrial"
	UserGroupAgricultural    DistributionUserGroup = "agricultural"
	UserGroupLighting        DistributionUserGroup = "lighting"
	UserGroupMartyrsFamilies DistributionUserGroup = "martyrs_families"
	UserGroupPublicLighting  DistributionUserGroup = "public_lighting"
)

// DistributionUserGroups is every value of distribution_user_group.
func DistributionUserGroups() []DistributionUserGroup {
	return []DistributionUserGroup{
		UserGroupResidential, UserGroupResidentialPlus, UserGroupCommercial,
		UserGroupCommercialPlus, UserGroupIndustrial, UserGroupAgricultural,
		UserGroupLighting, UserGroupMartyrsFamilies, UserGroupPublicLighting,
	}
}

// Valid reports whether g is one of the declared user groups.
func (g DistributionUserGroup) Valid() bool { return contains(DistributionUserGroups(), g) }

// CarbonScope mirrors the SQL enum carbon_scope (migration 00007).
type CarbonScope string

// The values of carbon_scope, as defined by the GHG Protocol.
const (
	CarbonScope1 CarbonScope = "scope_1"
	CarbonScope2 CarbonScope = "scope_2"
	CarbonScope3 CarbonScope = "scope_3"
)

// CarbonScopes is every value of carbon_scope.
func CarbonScopes() []CarbonScope { return []CarbonScope{CarbonScope1, CarbonScope2, CarbonScope3} }

// Valid reports whether s is one of the declared scopes.
func (s CarbonScope) Valid() bool { return contains(CarbonScopes(), s) }

// CarbonStatus mirrors the SQL enum carbon_status (migration 00007).
type CarbonStatus string

// The values of carbon_status.
const (
	CarbonStatusPending  CarbonStatus = "pending"
	CarbonStatusApproved CarbonStatus = "approved"
	CarbonStatusRejected CarbonStatus = "rejected"
)

// CarbonStatuses is every value of carbon_status.
func CarbonStatuses() []CarbonStatus {
	return []CarbonStatus{CarbonStatusPending, CarbonStatusApproved, CarbonStatusRejected}
}

// Valid reports whether s is one of the declared statuses.
func (s CarbonStatus) Valid() bool { return contains(CarbonStatuses(), s) }

// BillScope mirrors the SQL enum bill_scope (migration 00009): the level a
// bill was computed at.
type BillScope string

// The values of bill_scope.
const (
	BillScopeAnalyzer BillScope = "analyzer"
	BillScopeBuilding BillScope = "building"
	BillScopeCompany  BillScope = "company"
)

// BillScopes is every value of bill_scope.
func BillScopes() []BillScope {
	return []BillScope{BillScopeAnalyzer, BillScopeBuilding, BillScopeCompany}
}

// Valid reports whether s is one of the declared bill scopes.
func (s BillScope) Valid() bool { return contains(BillScopes(), s) }

// BillStatus mirrors the SQL enum bill_status (migration 00009).
type BillStatus string

// The values of bill_status. Recomputation supersedes rather than deletes, so
// BillStatusSuperseded is the value the partial unique index excludes:
// migration 00009 permits exactly one non-superseded bill per scope and period.
const (
	BillStatusDraft      BillStatus = "draft"
	BillStatusIssued     BillStatus = "issued"
	BillStatusFlagged    BillStatus = "flagged"
	BillStatusSuperseded BillStatus = "superseded"
)

// BillStatuses is every value of bill_status.
func BillStatuses() []BillStatus {
	return []BillStatus{BillStatusDraft, BillStatusIssued, BillStatusFlagged, BillStatusSuperseded}
}

// Valid reports whether s is one of the declared statuses.
func (s BillStatus) Valid() bool { return contains(BillStatuses(), s) }

// ReportType mirrors the SQL enum report_type (migration 00010).
type ReportType string

// The values of report_type.
const (
	ReportTypeMonthly ReportType = "monthly"
	ReportTypeYearly  ReportType = "yearly"
)

// ReportTypes is every value of report_type.
func ReportTypes() []ReportType { return []ReportType{ReportTypeMonthly, ReportTypeYearly} }

// Valid reports whether r is one of the declared report types.
func (r ReportType) Valid() bool { return contains(ReportTypes(), r) }

// ReportStatus mirrors the SQL enum report_status (migration 00010).
type ReportStatus string

// The values of report_status.
const (
	ReportStatusPending   ReportStatus = "pending"
	ReportStatusCompleted ReportStatus = "completed"
	ReportStatusError     ReportStatus = "error"
)

// ReportStatuses is every value of report_status.
func ReportStatuses() []ReportStatus {
	return []ReportStatus{ReportStatusPending, ReportStatusCompleted, ReportStatusError}
}

// Valid reports whether s is one of the declared statuses.
func (s ReportStatus) Valid() bool { return contains(ReportStatuses(), s) }

// PlantSelection mirrors the SQL enum plant_selection (migration 00010): which
// power plants a report covers.
type PlantSelection string

// The values of plant_selection.
const (
	PlantSelectionAll     PlantSelection = "all"
	PlantSelectionGrid    PlantSelection = "grid"
	PlantSelectionRooftop PlantSelection = "rooftop"
)

// PlantSelections is every value of plant_selection.
func PlantSelections() []PlantSelection {
	return []PlantSelection{PlantSelectionAll, PlantSelectionGrid, PlantSelectionRooftop}
}

// Valid reports whether p is one of the declared selections.
func (p PlantSelection) Valid() bool { return contains(PlantSelections(), p) }

// AlarmType mirrors the SQL enum alarm_type (migration 00011). It decides
// which of the Alarm struct's threshold groups is meaningful; the schema keeps
// every group nullable rather than splitting the table, so the type is the
// only thing saying which columns to read.
type AlarmType string

// The values of alarm_type.
const (
	AlarmTypeReactiveLimit       AlarmType = "reactive_limit"
	AlarmTypeDataCommunication   AlarmType = "data_communication"
	AlarmTypeCurrentVoltagePower AlarmType = "current_voltage_power"
	AlarmTypeInvoiceIncrease     AlarmType = "invoice_increase"
)

// AlarmTypes is every value of alarm_type.
func AlarmTypes() []AlarmType {
	return []AlarmType{
		AlarmTypeReactiveLimit, AlarmTypeDataCommunication,
		AlarmTypeCurrentVoltagePower, AlarmTypeInvoiceIncrease,
	}
}

// Valid reports whether a is one of the declared alarm types.
func (a AlarmType) Valid() bool { return contains(AlarmTypes(), a) }

// PeriodUnit mirrors the SQL enum period_unit (migration 00011).
type PeriodUnit string

// The values of period_unit.
const (
	PeriodUnitHours PeriodUnit = "hours"
	PeriodUnitDays  PeriodUnit = "days"
)

// PeriodUnits is every value of period_unit.
func PeriodUnits() []PeriodUnit { return []PeriodUnit{PeriodUnitHours, PeriodUnitDays} }

// Valid reports whether u is one of the declared period units.
func (u PeriodUnit) Valid() bool { return contains(PeriodUnits(), u) }

// NotifyChannel mirrors the SQL enum notify_channel (migration 00011).
type NotifyChannel string

// The values of notify_channel.
const (
	NotifyChannelEmail NotifyChannel = "email"
	NotifyChannelSMS   NotifyChannel = "sms"
)

// NotifyChannels is every value of notify_channel.
func NotifyChannels() []NotifyChannel { return []NotifyChannel{NotifyChannelEmail, NotifyChannelSMS} }

// Valid reports whether c is one of the declared channels.
func (c NotifyChannel) Valid() bool { return contains(NotifyChannels(), c) }

// contains is the one-line membership test every Valid method above shares.
// It is generic so that each enum keeps its own type through the call: a
// non-generic []string version would accept a BillStatus where a UserRole was
// meant, which is the confusion these named types exist to prevent.
func contains[T comparable](haystack []T, needle T) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// ReactivePenaltyBasis mirrors the SQL enum reactive_penalty_basis
// (migration 00014): which reactive quantity a penalty is charged on.
type ReactivePenaltyBasis string

// The values of reactive_penalty_basis.
const (
	ReactivePenaltyBasisWholeQuantity   ReactivePenaltyBasis = "whole_quantity"
	ReactivePenaltyBasisExcessOverLimit ReactivePenaltyBasis = "excess_over_limit"
)

// ReactivePenaltyBases is every value of reactive_penalty_basis.
func ReactivePenaltyBases() []ReactivePenaltyBasis {
	return []ReactivePenaltyBasis{ReactivePenaltyBasisWholeQuantity, ReactivePenaltyBasisExcessOverLimit}
}

// Valid reports whether b is one of the declared bases.
func (b ReactivePenaltyBasis) Valid() bool { return contains(ReactivePenaltyBases(), b) }

// TieringMode mirrors the SQL enum tiering_mode (migration 00014, R121).
type TieringMode string

// The values of tiering_mode.
const (
	TieringModeSplitAtThreshold       TieringMode = "split_at_threshold"
	TieringModeWholeConsumptionSwitch TieringMode = "whole_consumption_switch"
)

// TieringModes is every value of tiering_mode.
func TieringModes() []TieringMode {
	return []TieringMode{TieringModeSplitAtThreshold, TieringModeWholeConsumptionSwitch}
}

// Valid reports whether m is one of the declared modes.
func (m TieringMode) Valid() bool { return contains(TieringModes(), m) }

// PriceSource mirrors the SQL enum price_source (migration 00014, R119).
type PriceSource string

// The values of price_source.
const (
	PriceSourceKbk   PriceSource = "kbk"
	PriceSourceFixed PriceSource = "fixed"
)

// PriceSources is every value of price_source.
func PriceSources() []PriceSource { return []PriceSource{PriceSourceKbk, PriceSourceFixed} }

// Valid reports whether p is one of the declared sources.
func (p PriceSource) Valid() bool { return contains(PriceSources(), p) }

// ExtraChargeBasis mirrors the SQL enum extra_charge_basis (migration 00014, R126).
type ExtraChargeBasis string

// The values of extra_charge_basis.
const (
	ExtraChargeBasisPerKwh          ExtraChargeBasis = "per_kwh"
	ExtraChargeBasisPerContractedKw ExtraChargeBasis = "per_contracted_kw"
	ExtraChargeBasisPerMaxDemandKw  ExtraChargeBasis = "per_max_demand_kw"
	ExtraChargeBasisFixedPerPeriod  ExtraChargeBasis = "fixed_per_period"
	ExtraChargeBasisPctOfEnergy     ExtraChargeBasis = "pct_of_energy"
)

// ExtraChargeBases is every value of extra_charge_basis.
func ExtraChargeBases() []ExtraChargeBasis {
	return []ExtraChargeBasis{
		ExtraChargeBasisPerKwh, ExtraChargeBasisPerContractedKw, ExtraChargeBasisPerMaxDemandKw,
		ExtraChargeBasisFixedPerPeriod, ExtraChargeBasisPctOfEnergy,
	}
}

// Valid reports whether b is one of the declared bases.
func (b ExtraChargeBasis) Valid() bool { return contains(ExtraChargeBases(), b) }

// MoneyRoundingMode mirrors the SQL enum money_rounding_mode (migration 00014, R112).
type MoneyRoundingMode string

// The values of money_rounding_mode.
const (
	MoneyRoundingHalfUp   MoneyRoundingMode = "half_up"
	MoneyRoundingHalfEven MoneyRoundingMode = "half_even"
)

// MoneyRoundingModes is every value of money_rounding_mode.
func MoneyRoundingModes() []MoneyRoundingMode {
	return []MoneyRoundingMode{MoneyRoundingHalfUp, MoneyRoundingHalfEven}
}

// Valid reports whether m is one of the declared modes.
func (m MoneyRoundingMode) Valid() bool { return contains(MoneyRoundingModes(), m) }
