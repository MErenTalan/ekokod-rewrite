import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { activity, BUILDING, catalogue, factor, me } from './_fixture';
import { EntryPanel } from './entry-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let sent: Record<string, unknown> | null = null;
const routes = {
  'GET /api/v1/carbon/activity-catalogue': catalogue,
  'GET /api/v1/carbon/selected-activities': { activity_keys: ['sub_space_heating'] },
  'GET /api/v1/carbon/activities': { items: [activity(), activity({ id: 'a-2' })], next_cursor: null },
  'GET /api/v1/carbon/emission-factors': { items: [factor()] },
  'POST /api/v1/carbon/activities': (req: Request) =>
    req.json().then((b: Record<string, unknown>) => {
      sent = b;
      return Response.json(activity(), { status: 201 });
    }),
};

describe('EntryPanel', () => {
  it('counts records per sub-category and creates an entry for the selected building', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <EntryPanel buildingId={BUILDING} go={vi.fn()} />
      </SessionProvider>,
    );
    expect(await r.findByText('2 kayıt')).toBeVisible();
    await r.user.click(r.getByRole('button', { name: 'Veri ekle' }));
    expect(await r.findByRole('dialog')).toBeVisible();
    await waitFor(() => expect(api.calls.some((c) => c.url.includes('sub_category=sub_space_heating'))).toBe(true));
  });

  it('closes an open entry when the building changes (Review Focus 4)', async () => {
    api = mockApi(routes);
    const tree = (b: string) => (
      <SessionProvider me={me('company_admin')}>
        <EntryPanel buildingId={b} go={vi.fn()} />
      </SessionProvider>
    );
    const r = renderWithProviders(tree(BUILDING));
    await r.user.click(await r.findByRole('button', { name: 'Veri ekle' }));
    expect(await r.findByRole('dialog')).toBeVisible();
    r.rerender(tree('b-2'));
    await waitFor(() => expect(r.queryByRole('dialog')).toBeNull());
    expect(sent).toBeNull();
  });
});
