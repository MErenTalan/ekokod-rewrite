import { waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import { selectionKey } from '@/lib/selection/selection-store';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi, queryOf } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { forecast } from './_fixture';
import { PredictPage } from './predict-page';

const me = (role: 'company_admin' | 'company_readonly_admin'): MeResponse => ({
  id: 'u-1', name: 'Ayşe Kaya', email: 'ayse@ornek.com.tr', role, locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'], session_id: 's', company: { id: 'c-own', name: 'Kendi Şirketim' },
});

const ROUTES = {
  'GET /api/v1/buildings': { items: [{ id: 'b-1', name: 'A1 Fabrika', bill_cutoff_day: 1, created_at: '', updated_at: '', activity_status: 'active' }] },
  'GET /api/v1/analyzers': {
    items: [{ id: 'a-1', building_id: 'b-1', installation_number: '1001', provider: 'osos', provider_subtype: 'Baskent', meter_multiplier: '1', is_active: true, activity_status: 'active' }],
  },
  'GET /api/v1/consumption': { items: [{ period_start: '2026-09-23T20:00:00Z', period_end: '2026-09-23T21:00:00Z', analyzer_id: 'a-1', source: 'raw', partial: false, suspect_registers: [], active_import: '4.5' }] },
  'GET /api/v1/forecast': { status: 'none', used_covariates: [], points: [], gaps: [] },
};

let api: ReturnType<typeof mockApi>;
beforeEach(() => localStorage.setItem(selectionKey('u-1'), JSON.stringify({ buildingId: 'b-1', analyzerId: 'a-1' })));
afterEach(() => {
  api.restore();
  localStorage.clear();
});

function render(role: 'company_admin' | 'company_readonly_admin' = 'company_admin') {
  return renderWithProviders(
    <SessionProvider me={me(role)}>
      <PredictPage />
    </SessionProvider>,
  );
}

describe('PredictPage', () => {
  it('reads hourly actuals and the stored forecast for the horizon', async () => {
    api = mockApi(ROUTES);
    const r = render();
    await waitFor(() => expect(api.calls.some((c) => new URL(c.url).pathname === '/api/v1/forecast')).toBe(true));
    const actual = queryOf(api.calls, 'GET', '/api/v1/consumption');
    expect(actual.get('analyzer_id')).toBe('a-1');
    expect(actual.get('granularity')).toBe('hourly');
    const stored = queryOf(api.calls, 'GET', '/api/v1/forecast');
    expect((Date.parse(stored.get('to')!) - Date.parse(stored.get('from')!)) / 3_600_000).toBe(48);
    expect(await r.findByText(/kayıtlı tahmin yok/)).toBeInTheDocument();
  });

  it('runs a forecast for the chosen horizon and shows its model', async () => {
    let sent: Record<string, unknown> = {};
    api = mockApi({ ...ROUTES, 'POST /api/v1/forecast/run': (req: Request) => req.json().then((b: Record<string, unknown>) => ((sent = b), Response.json(forecast))) });
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText(/Model: random_forest 1\.0\.0/)).toBeInTheDocument();
    expect(sent).toEqual({ analyzer_id: 'a-1', horizon_hours: 48 });
    expect(r.getByText(/Geçmiş veride 1 boşluk var/)).toBeInTheDocument();
  });

  it('degrades when the forecasting service is down (R385)', async () => {
    api = mockApi({ ...ROUTES, 'POST /api/v1/forecast/run': Response.json({ error: { code: 'forecast_unavailable', message: 'x' } }, { status: 503 }) });
    const r = render();
    await r.user.click(await r.findByRole('button', { name: 'Tahmin et' }));
    expect(await r.findByText(/Tahmin servisi şu anda kullanılamıyor/)).toBeInTheDocument();
    expect(r.getByRole('figure', { name: 'Gerçekleşen ve tahmin' })).toBeInTheDocument();
  });

  it('offers read-only roles the stored forecast only', async () => {
    api = mockApi(ROUTES);
    const r = render('company_readonly_admin');
    expect(await r.findByText(/Yeni tahmin çalıştırma yetkiniz yok/)).toBeInTheDocument();
    expect(r.queryByRole('button', { name: 'Tahmin et' })).toBeNull();
  });
});
