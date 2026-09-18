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
export type TariffSummary = S['TariffSummary'];
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
