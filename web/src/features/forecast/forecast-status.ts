import type { components } from '@/lib/api/schema';

export type Forecast = components['schemas']['Forecast'];
export type ForecastGap = components['schemas']['ForecastGap'];
export type ForecastState = 'ok' | 'empty' | 'none' | 'insufficient_data' | 'no_data' | 'model_error' | 'unavailable';
export type Actual = { ts: string; value: string | null };
export type ChartRow = { x: string; actual: string | null; median: string | null; p10: string | null; p90: string | null };

/** R380: one explicit state for the banner; an unreachable service wins over any stored run. */
export function forecastStatus(forecast: Forecast | null, unavailable: boolean): ForecastState {
  if (unavailable) return 'unavailable';
  if (!forecast || forecast.status === 'none') return 'none';
  if (forecast.status === 'ok') return forecast.points.length > 0 ? 'ok' : 'empty';
  return forecast.status;
}

/** R381: actuals and forecast on one hourly axis, sorted by time. */
export function chartRows(actuals: Actual[], forecast: Forecast | null): ChartRow[] {
  const rows = new Map<string, ChartRow>();
  const row = (x: string) => rows.get(x) ?? rows.set(x, { x, actual: null, median: null, p10: null, p90: null }).get(x)!;
  for (const a of actuals) row(a.ts).actual = a.value;
  for (const p of forecast?.points ?? []) Object.assign(row(p.ts), { median: p.median, p10: p.p10 ?? null, p90: p.p90 ?? null });
  return [...rows.values()].sort((a, b) => Date.parse(a.x) - Date.parse(b.x));
}

export function totalGapHours(gaps: ForecastGap[]): number {
  return gaps.reduce((sum, g) => sum + g.missing_hours, 0);
}

/** API values → camelCase message keys (the i18n key rule). */
export const STATE_KEY = {
  ok: 'ok', empty: 'empty', none: 'none', insufficient_data: 'insufficientData', no_data: 'noData', model_error: 'modelError', unavailable: 'unavailable',
} as const satisfies Record<ForecastState, string>;

export const COVARIATE_KEY: Record<string, 'dayType' | 'vacation' | 'weather'> = { day_type: 'dayType', vacation: 'vacation', weather: 'weather' };
export const METHOD_KEY: Record<string, 'robustZscoreSameHourOfWeek' | 'insufficientHistory'> = {
  robust_zscore_same_hour_of_week: 'robustZscoreSameHourOfWeek', insufficient_history: 'insufficientHistory',
};
