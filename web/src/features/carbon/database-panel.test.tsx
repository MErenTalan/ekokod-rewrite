import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { factor, me } from './_fixture';
import { DatabasePanel } from './database-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

const routes = {
  'GET /api/v1/carbon/emission-factors': { items: [factor()] },
  'POST /api/v1/carbon/emission-factors/reset': new Response(null, { status: 204 }),
};

describe('DatabasePanel', () => {
  it('searches on the server', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <DatabasePanel />
      </SessionProvider>,
    );
    await r.user.type(await r.findByRole('searchbox', { name: 'Faktör ara' }), 'gaz');
    await waitFor(() => expect(api.calls.some((c) => new URL(c.url).searchParams.get('q') === 'gaz')).toBe(true));
  });

  it('resets only after the confirmation', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <DatabasePanel />
      </SessionProvider>,
    );
    await r.user.click(await r.findByRole('button', { name: 'Varsayılana dön' }));
    expect(await r.findByRole('dialog', { name: 'Katalog varsayılana dönsün mü?' })).toBeVisible();
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false);
    await r.user.click(r.getAllByRole('button', { name: 'Varsayılana dön' }).at(-1)!);
    await waitFor(() => expect(api.calls.some((c) => c.method === 'POST')).toBe(true));
  });
});
