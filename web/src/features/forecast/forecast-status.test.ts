import { describe, expect, it } from 'vitest';

import { chartRows, forecastStatus, totalGapHours } from './forecast-status';

const f = (over: object = {}) => ({ status: 'ok' as const, used_covariates: [], points: [{ ts: '2026-09-24T10:00:00Z', median: '5', p10: '4', p90: '6' }], gaps: [], ...over });

describe('forecastStatus (R380)', () => {
  it('names every state, unavailable first', () => {
    expect(forecastStatus(f(), true)).toBe('unavailable');
    expect(forecastStatus(null, false)).toBe('none');
    expect(forecastStatus(f(), false)).toBe('ok');
    expect(forecastStatus(f({ points: [] }), false)).toBe('empty');
    expect(forecastStatus(f({ status: 'none', points: [] }), false)).toBe('none');
    for (const s of ['insufficient_data', 'no_data', 'model_error'] as const) expect(forecastStatus(f({ status: s, points: [] }), false)).toBe(s);
  });
});

describe('chartRows (R381)', () => {
  it('lays actuals and the forecast on one hourly axis', () => {
    const rows = chartRows([{ ts: '2026-09-24T09:00:00Z', value: '3.5' }, { ts: '2026-09-24T10:00:00Z', value: null }], f());
    expect(rows).toEqual([
      { x: '2026-09-24T09:00:00Z', actual: '3.5', median: null, p10: null, p90: null },
      { x: '2026-09-24T10:00:00Z', actual: null, median: '5', p10: '4', p90: '6' },
    ]);
  });
});

describe('totalGapHours', () => {
  it('sums the missing hours', () => {
    expect(totalGapHours([{ start: '', end: '', missing_hours: 3 }, { start: '', end: '', missing_hours: 2 }])).toBe(5);
  });
});
