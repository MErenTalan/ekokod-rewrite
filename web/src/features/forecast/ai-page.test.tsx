import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { forecast } from './_fixture';
import { AiPage } from './ai-page';

const me: MeResponse = {
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role: 'company_admin', locale: 'tr',
  permissions: fixture.company_admin as MeResponse['permissions'], session_id: 's', company: { id: 'c-own', name: 'Kendi Şirketim' },
};
const SCOPE = {
  'GET /api/v1/buildings': { items: [{ id: 'b-1', name: 'A1 Fabrika', bill_cutoff_day: 1, created_at: '', updated_at: '', activity_status: 'active' }] },
  'GET /api/v1/analyzers': {
    items: [{ id: 'a-1', building_id: 'b-1', installation_number: '1001', provider: 'osos', provider_subtype: 'Baskent', meter_multiplier: '1', is_active: true, activity_status: 'active' }],
  },
};

type Body = Record<string, unknown>;
let api: ReturnType<typeof mockApi>;
const sent: Record<string, Body> = {};
const capture = (path: string, response: () => Response) => ({
  [`POST ${path}`]: (req: Request) => req.json().then((b: Body) => ((sent[path] = b), response())),
});

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-09-23T10:00:00Z')); // Wednesday 13:00 Istanbul
  localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' }));
});
afterEach(() => {
  api.restore();
  vi.useRealTimers();
  localStorage.clear();
});

const render = () => renderWithProviders(<SessionProvider me={me}><AiPage /></SessionProvider>);

describe('AiPage', () => {
  it('daily: runs to the end of tomorrow and totals that day with its weekday (Q-I17)', async () => {
    // The run starts at the current hour, so it also covers the rest of today: those hours are not tomorrow's.
    const today = { ts: '2026-09-23T20:00:00Z', median: '999', p10: '999', p90: '999' };
    api = mockApi({ ...SCOPE, ...capture('/api/v1/forecast/run', () => Response.json({ ...forecast, points: [today, ...forecast.points] })) });
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText(/Perşembe için toplam tahmin/)).toBeInTheDocument();
    expect(r.getByRole('table', { name: 'Günlük tahmin' })).not.toHaveTextContent('999');
    expect(sent['/api/v1/forecast/run']).toEqual({ analyzer_id: 'a-1', horizon_hours: 35 });
    expect(r.getByRole('table', { name: 'Günlük tahmin' })).toHaveTextContent('Toplam');
  });

  it('weekly: asks for next Monday and says the result is not stored', async () => {
    api = mockApi({ ...SCOPE, ...capture('/api/v1/forecast/weekly', () => Response.json(forecast)) });
    const r = render();
    await r.user.click(await r.findByRole('tab', { name: 'Haftalık tahmin' }));
    await r.user.click(r.getByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText('Bu tahmin kaydedilmez.')).toBeInTheDocument();
    expect(sent['/api/v1/forecast/weekly']).toEqual({ analyzer_id: 'a-1', week_start: '2026-09-28' });
  });

  it('monthly: asks for the current month', async () => {
    api = mockApi({ ...SCOPE, ...capture('/api/v1/forecast/monthly', () => Response.json({ ...forecast, points: forecast.points.slice(0, 1) })) });
    const r = render();
    await r.user.click(await r.findByRole('tab', { name: 'Aylık tahmin' }));
    await r.user.click(r.getByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByRole('table', { name: 'Aylık tahmin' })).toBeInTheDocument();
    expect(sent['/api/v1/forecast/monthly']).toEqual({ analyzer_id: 'a-1', month: '2026-09' });
  });

  it('anomaly: checks the previous hour with the stored value when none is typed', async () => {
    api = mockApi({
      ...SCOPE,
      ...capture('/api/v1/anomaly/check', () => Response.json({ available: true, is_anomaly: true, score: 7.5, actual: '40', expected: '10', lower: '4.8', upper: '15.2', method: 'robust_zscore_same_hour_of_week' })),
    });
    const r = render();
    await r.user.click(await r.findByRole('tab', { name: 'Anomali kontrolü' }));
    await r.user.click(r.getByRole('button', { name: 'Kontrol et' }));
    expect(await r.findByText('Anomali')).toBeInTheDocument();
    expect(sent['/api/v1/anomaly/check']).toEqual({ analyzer_id: 'a-1', ts: '2026-09-23T12:00:00+03:00' });
  });

  it('anomaly: says when the service is down', async () => {
    api = mockApi({ ...SCOPE, ...capture('/api/v1/anomaly/check', () => Response.json({ available: false, reason: 'ml_service_unavailable' })) });
    const r = render();
    await r.user.click(await r.findByRole('tab', { name: 'Anomali kontrolü' }));
    await r.user.click(r.getByRole('button', { name: 'Kontrol et' }));
    expect(await r.findByText(/Tahmin servisi şu anda kullanılamıyor/)).toBeInTheDocument();
  });

  it('a forecast refused by the service shows the unavailable state', async () => {
    api = mockApi({ ...SCOPE, ...capture('/api/v1/forecast/run', () => Response.json({ error: { code: 'forecast_unavailable', message: 'x' } }, { status: 503 })) });
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText(/Tahmin servisi şu anda kullanılamıyor/)).toBeInTheDocument();
  });

  it('puts a refused week start on its field', async () => {
    api = mockApi({
      ...SCOPE,
      ...capture('/api/v1/forecast/weekly', () => Response.json({ error: { code: 'validation_failed', message: 'x', details: { week_start: ['out_of_range'] } } }, { status: 422 })),
    });
    const r = render();
    await r.user.click(await r.findByRole('tab', { name: 'Haftalık tahmin' }));
    await r.user.click(r.getByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText('Aralık dışında')).toBeInTheDocument();
  });
});
