import type { Actual, Forecast } from './forecast-status';

const H = 3_600_000;
const start = Date.parse('2026-09-23T21:00:00Z'); // 24 Eylül 00:00 İstanbul

export const actuals: Actual[] = Array.from({ length: 48 }, (_, i) => ({
  ts: new Date(start - (48 - i) * H).toISOString().replace('.000', ''),
  value: i === 10 ? null : String(5 + Math.round(4 * Math.sin(i / 3.8))),
}));

export const forecast: Forecast = {
  status: 'ok',
  model_id: 'random_forest',
  model_version: '1.0.0',
  generated_at: '2026-09-24T01:00:00Z',
  used_covariates: ['day_type', 'vacation'],
  points: Array.from({ length: 24 }, (_, i) => ({
    ts: new Date(start + i * H).toISOString().replace('.000', ''),
    median: String(5 + Math.round(4 * Math.sin((48 + i) / 3.8))),
    p10: String(3 + Math.round(4 * Math.sin((48 + i) / 3.8))),
    p90: String(7 + Math.round(4 * Math.sin((48 + i) / 3.8))),
  })),
  gaps: [{ start: '2026-09-22T07:00:00Z', end: '2026-09-22T09:00:00Z', missing_hours: 3 }],
};
