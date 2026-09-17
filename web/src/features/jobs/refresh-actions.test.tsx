import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { MeResponse } from '@/lib/api/errors';
import fixture from '@/lib/session/permissions.fixture.json';
import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { RefreshActions } from './refresh-actions';

const me = (role: keyof typeof fixture): MeResponse => ({
  id: `user-${role}`,
  name: 'Burak Şahin',
  email: 'burak@ornek.com.tr',
  role,
  locale: 'tr',
  permissions: fixture[role] as MeResponse['permissions'],
  session_id: 's',
  company: { id: 'c-own', name: 'Kendi Şirketim' },
});

let api: ReturnType<typeof mockApi>;
afterEach(() => {
  api?.restore();
  vi.useRealTimers();
});

describe('RefreshActions', () => {
  it('is invisible to a role that may not refresh', () => {
    api = mockApi({});
    const r = renderWithProviders(
      <SessionProvider me={me('company_readonly_admin')}>
        <RefreshActions analyzerId="a-1" />
      </SessionProvider>,
    );
    expect(r.queryByRole('button', { name: 'Saatlik değerleri yenile' })).toBeNull();
    expect(api.calls).toHaveLength(0);
  });

  it('explains itself and stays disabled without an analyzer', () => {
    api = mockApi({});
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <RefreshActions analyzerId={null} />
      </SessionProvider>,
    );
    expect(r.getByRole('button', { name: 'Saatlik değerleri yenile' })).toBeDisabled();
    expect(r.getByText('Yenilemek için bir analizör seçin')).toBeInTheDocument();
  });

  it('enqueues the pull and then shows the job it started', async () => {
    api = mockApi({
      'POST /api/v1/analyzers/a-1/refresh': Response.json({ job_id: 'job-7' }, { status: 202 }),
      'GET /api/v1/jobs/job-7': { id: 'job-7', type: 'integration.refresh_analyzer', status: 'running' },
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <RefreshActions analyzerId="a-1" />
      </SessionProvider>,
    );
    await r.user.click(r.getByRole('button', { name: 'Saatlik değerleri yenile' }));
    await waitFor(() => expect(r.getByText(/Saatlik değerleri yenile: Çalışıyor/)).toBeInTheDocument());
    const post = api.calls.find((c) => c.method === 'POST');
    expect(await post!.clone().json()).toEqual({ mode: 'hourly' });
  });

  it('shows the API own message when the integration is not configured', async () => {
    api = mockApi({
      'POST /api/v1/analyzers/a-1/refresh': Response.json(
        { error: { code: 'integration_not_configured', message: 'Bu analizör için entegrasyon tanımlı değil.' } },
        { status: 409 },
      ),
    });
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <RefreshActions analyzerId="a-1" />
      </SessionProvider>,
    );
    await r.user.click(r.getByRole('button', { name: 'Enerji değerlerini yenile' }));
    await waitFor(() => expect(r.getByText('Bu analizör için entegrasyon tanımlı değil.')).toBeInTheDocument());
  });
});
