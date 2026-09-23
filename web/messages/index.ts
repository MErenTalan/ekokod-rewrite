// Statically imported catalogues, one file per namespace (plan D7). Add a namespace here and in both locales.
import trApp from './tr/app.json';
import trHealth from './tr/health.json';
import trUnits from './tr/units.json';
import trCommon from './tr/common.json';
import trForms from './tr/forms.json';
import trFeedback from './tr/feedback.json';
import trTable from './tr/table.json';
import trCharts from './tr/charts.json';
import trShell from './tr/shell.json';
import trDomain from './tr/domain.json';
import trMap from './tr/map.json';
import trAuth from './tr/auth.json';
import trDashboard from './tr/dashboard.json';
import trConsumption from './tr/consumption.json';
import trLoadProfile from './tr/loadProfile.json';
import trCalendar from './tr/calendar.json';
import trSettings from './tr/settings.json';
import trAlarms from './tr/alarms.json';
import trMessages from './tr/messages.json';
import trTariffs from './tr/tariffs.json';
import trBills from './tr/bills.json';
import trReports from './tr/reports.json';
import trSolarPlants from './tr/solarPlants.json';
import trWeather from './tr/weather.json';
import trRenewable from './tr/renewable.json';
import trFinancial from './tr/financial.json';
import enApp from './en/app.json';
import enHealth from './en/health.json';
import enUnits from './en/units.json';
import enCommon from './en/common.json';
import enForms from './en/forms.json';
import enFeedback from './en/feedback.json';
import enTable from './en/table.json';
import enCharts from './en/charts.json';
import enShell from './en/shell.json';
import enDomain from './en/domain.json';
import enMap from './en/map.json';
import enAuth from './en/auth.json';
import enDashboard from './en/dashboard.json';
import enConsumption from './en/consumption.json';
import enLoadProfile from './en/loadProfile.json';
import enCalendar from './en/calendar.json';
import enSettings from './en/settings.json';
import enAlarms from './en/alarms.json';
import enMessages from './en/messages.json';
import enTariffs from './en/tariffs.json';
import enBills from './en/bills.json';
import enReports from './en/reports.json';
import enSolarPlants from './en/solarPlants.json';
import enWeather from './en/weather.json';
import enRenewable from './en/renewable.json';
import enFinancial from './en/financial.json';

const tr = {
  app: trApp,
  health: trHealth,
  units: trUnits,
  common: trCommon,
  forms: trForms,
  feedback: trFeedback,
  table: trTable,
  charts: trCharts,
  shell: trShell,
  domain: trDomain,
  map: trMap,
  auth: trAuth,
  dashboard: trDashboard,
  consumption: trConsumption,
  loadProfile: trLoadProfile,
  calendar: trCalendar,
  settings: trSettings,
  alarms: trAlarms,
  messages: trMessages,
  tariffs: trTariffs,
  bills: trBills,
  reports: trReports,
  solarPlants: trSolarPlants,
  weather: trWeather,
  renewable: trRenewable,
  financial: trFinancial,
};
const en = {
  app: enApp,
  health: enHealth,
  units: enUnits,
  common: enCommon,
  forms: enForms,
  feedback: enFeedback,
  table: enTable,
  charts: enCharts,
  shell: enShell,
  domain: enDomain,
  map: enMap,
  auth: enAuth,
  dashboard: enDashboard,
  consumption: enConsumption,
  loadProfile: enLoadProfile,
  calendar: enCalendar,
  settings: enSettings,
  alarms: enAlarms,
  messages: enMessages,
  tariffs: enTariffs,
  bills: enBills,
  reports: enReports,
  solarPlants: enSolarPlants,
  weather: enWeather,
  renewable: enRenewable,
  financial: enFinancial,
};

export type Messages = typeof tr;
export const messages: { tr: Messages; en: Messages } = { tr, en };
