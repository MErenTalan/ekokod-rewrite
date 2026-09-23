// The one place the generated schema is given names (R195): features import
// these aliases, never `components['schemas'][...]` directly.
import type { components } from './schema';

type S = components['schemas'];

export type Building = S['Building'];
export type BuildingDetail = S['BuildingDetail'];
export type BuildingCreateRequest = S['BuildingCreateRequest'];
export type BuildingUpdateRequest = S['BuildingUpdateRequest'];
export type BuildingComparison = S['BuildingComparison'];
export type ComparisonMetric = S['ComparisonMetric'];
export type Contact = S['Contact'];
export type Analyzer = S['Analyzer'];
export type AnalyzerUpdateRequest = S['AnalyzerUpdateRequest'];
export type Provider = Analyzer['provider'];
export type ActivityStatus = Analyzer['activity_status'];

export type ConsumptionRow = S['ConsumptionRow'];
export type ConsumptionSummary = S['ConsumptionSummary'];
export type ConsumptionGrouped = S['ConsumptionGrouped'];
export type GroupedBucket = S['GroupedBucket'];
export type GroupedPeriod = S['GroupedPeriod'];
export type AnomalyCheck = S['AnomalyCheck'];
export type LoadProfiles = S['LoadProfiles'];
export type LoadProfileStatistics = S['LoadProfileStatistics'];
export type ProfileStatistics = S['ProfileStatistics'];
export type LoadProfileConfig = S['LoadProfileConfig'];
export type ReactiveStatus = S['ReactiveStatus'];
export type ReactiveAnalyzer = S['ReactiveAnalyzer'];

export type Bill = S['Bill'];
export type BillDashboard = S['BillDashboard'];
export type BillDashboardBuilding = S['BillDashboardBuilding'];
export type BillDashboardRow = S['BillDashboardRow'];
export type BillDashboardNetting = S['BillDashboardNetting'];
export type TariffSummary = S['TariffSummary'];
export type Tariff = S['Tariff'];
export type TariffFields = S['TariffFields'];
export type TariffSummaryItem = S['TariffSummaryItem'];
export type TariffTax = S['TariffTax'];
export type TariffExtraCharge = S['TariffExtraCharge'];
export type TariffManualYekdem = S['TariffManualYekdem'];
export type ExtraChargeBasis = TariffExtraCharge['basis'];
export type TariffTemplate = S['TariffTemplate'];
export type BuildingTariffState = S['BuildingTariffState'];
export type BulkTariffAssignment = S['BulkTariffAssignment'];
export type BulkTariffResult = S['BulkTariffResult'];
export type IcmalImport = S['IcmalImport'];
export type IcmalAnalysis = S['IcmalAnalysis'];
export type IcmalCoefficient = S['IcmalCoefficient'];
export type SolarTariff = S['SolarTariff'];
export type NationalTariff = S['NationalTariff'];
export type NationalTariffFields = S['NationalTariffFields'];
export type SolarTariffFields = S['SolarTariffFields'];
export type Job = S['Job'];
export type JobStatus = Job['status'];

export type CalendarEvent = S['CalendarEvent'];
export type CalendarEventFields = S['CalendarEventFields'];
export type Vacations = S['Vacations'];
export type VacationPeriod = S['VacationPeriod'];
export type VacationsPutRequest = S['VacationsPutRequest'];

export type Company = S['Company'];
export type CompanyDetail = S['CompanyDetail'];
export type CompanyCreateRequest = S['CompanyCreateRequest'];
export type CompanyUpdateRequest = S['CompanyUpdateRequest'];
export type User = S['User'];
export type UserCreateRequest = S['UserCreateRequest'];
export type UserUpdateRequest = S['UserUpdateRequest'];
export type Role = S['Role'];
export type Session = S['Session'];
export type Me = S['Me'];
export type ProfileUpdateRequest = S['ProfileUpdateRequest'];
export type SMTPSettings = S['SMTPSettings'];
export type SMTPPutRequest = S['SMTPPutRequest'];

export type Plant = S['Plant'];
export type PlantDetail = S['PlantDetail'];
export type PlantDevice = S['PlantDevice'];
export type PlantCreateRequest = S['PlantCreateRequest'];
export type PlantUpdateRequest = S['PlantUpdateRequest'];
export type PlantRealtime = S['PlantRealtime'];
export type PlantProductionSeries = S['PlantProductionSeries'];
export type PlantProductionPoint = S['PlantProductionPoint'];
export type PlantDeviceView = S['PlantDeviceView'];
export type PlantFault = S['PlantFault'];
export type PlantFaultPage = S['PlantFaultPage'];
export type PlantRevenue = S['PlantRevenue'];
export type RevenuePeriod = S['RevenuePeriod'];
export type MoneyAmount = S['MoneyAmount'];
export type ISolarAccountPlant = S['ISolarAccountPlant'];
export type Weather = S['Weather'];
export type WeatherDay = S['WeatherDay'];
export type RenewableOverview = S['RenewableOverview'];
export type RenewableRealtime = S['RenewableRealtime'];
export type RenewableGridInteraction = S['RenewableGridInteraction'];
export type RenewableEnvironmental = S['RenewableEnvironmental'];
export type RenewableEfficiency = S['RenewableEfficiency'];
export type RenewableForecast = S['RenewableForecast'];
export type RenewableAnalytics = S['RenewableAnalytics'];
export type RenewableFinancial = S['RenewableFinancial'];
export type RenewableSystemStatus = S['RenewableSystemStatus'];
export type RenewablePoint = S['RenewablePoint'];
export type EquivalenceFactor = S['EquivalenceFactor'];
export type EnergyBalance = S['EnergyBalance'];
export type BalanceRow = S['BalanceRow'];
export type FinancialSummary = S['FinancialSummary'];
export type FinancialMonthly = S['FinancialMonthly'];
export type FinancialMonth = S['FinancialMonth'];
export type FinancialTariffs = S['FinancialTariffs'];

export type IntegrationDefinition = S['IntegrationDefinition'];
export type IntegrationCredential = S['IntegrationCredential'];
export type IntegrationCredentialCreateRequest = S['IntegrationCredentialCreateRequest'];
export type IntegrationCredentialUpdateRequest = S['IntegrationCredentialUpdateRequest'];
export type BackfillRequest = S['BackfillRequest'];

/** A next-intl namespace function, narrowed to what presentational views need. */
export type Translator = (key: string, values?: Record<string, string | number>) => string;

// F7 alarms and messages (05 §10).
export type Alarm = S['Alarm'];
export type AlarmFields = S['AlarmFields'];
export type AlarmSettings = S['AlarmSettings'];
export type AlarmChannel = S['AlarmChannel'];
export type AlarmAnalyzer = S['AlarmAnalyzer'];
export type AlarmEvent = S['AlarmEvent'];
export type AlarmEvaluation = S['AlarmEvaluation'];
export type AlarmEvaluationAnalyzer = S['AlarmEvaluationAnalyzer'];
export type AlarmEvaluationBreach = S['AlarmEvaluationBreach'];
export type OperationalMessage = S['Message'];
export type JobRun = S['JobRun'];

// F8b reports (05 §11).
export type ReportPayload = S['ReportPayload'];
export type ReportMonthly = S['ReportMonthly'];
export type ReportYearly = S['ReportYearly'];
export type ReportFigure = S['ReportFigure'];
export type ReportMoney = S['ReportMoney'];
export type ReportPriceRange = S['ReportPriceRange'];
export type ReportDelta = S['ReportDelta'];
export type ReportCurrencyDelta = S['ReportCurrencyDelta'];
export type ReportMonthPoint = S['ReportMonthPoint'];
export type ReportSummary = S['ReportSummary'];
export type ReportPage = S['ReportPage'];
export type ReportGenerateItem = S['ReportGenerateItem'];
