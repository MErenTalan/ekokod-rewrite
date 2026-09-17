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
};

export type Messages = typeof tr;
export const messages: { tr: Messages; en: Messages } = { tr, en };
