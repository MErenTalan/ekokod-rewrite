import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, catalogue, me } from './_fixture';
import { SelectionPanel } from './selection-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

let sent: unknown = null;
const routes = {
  'GET /api/v1/carbon/activity-catalogue': catalogue,
  'GET /api/v1/carbon/selected-activities': { activity_keys: ['sub_space_heating'] },
  'PUT /api/v1/carbon/selected-activities': (req: Request) =>
    req.json().then((b: { activity_keys: string[] }) => {
      sent = b;
      return Response.json({ activity_keys: b.activity_keys });
    }),
};

describe('SelectionPanel', () => {
  it('saves the declaration for the selected building', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <SelectionPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    await r.user.click(await r.findByRole('checkbox', { name: 'Atık Bertarafı' }));
    await r.user.click(r.getByRole('button', { name: 'Seçimi kaydet' }));
    const put = await vi_waitForPut();
    expect(new URL(put.url).searchParams.get('building_id')).toBe(BUILDING);
    expect(sent).toEqual({ activity_keys: ['sub_space_heating', 'sub_waste_disposal'] });
  });

  it('shows a building admin the declaration read-only', async () => {
    api = mockApi(routes);
    const r = renderWithProviders(
      <SessionProvider me={me('building_admin')}>
        <SelectionPanel buildingId={BUILDING} />
      </SessionProvider>,
    );
    expect(await r.findByRole('checkbox', { name: 'Ortam Isıtması' })).toBeDisabled();
    expect(r.queryByRole('button', { name: 'Seçimi kaydet' })).toBeNull();
  });
});

async function vi_waitForPut(): Promise<Request> {
  const { waitFor } = await import('@testing-library/react');
  let found: Request | undefined;
  await waitFor(() => {
    found = api.calls.find((c) => c.method === 'PUT');
    expect(found).toBeDefined();
  });
  return found!;
}
