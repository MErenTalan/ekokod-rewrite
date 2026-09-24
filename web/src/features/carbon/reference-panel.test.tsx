import { afterEach, describe, expect, it } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { catalogue, me } from './_fixture';
import { ReferencePanel } from './reference-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('ReferencePanel', () => {
  it('renders the mapping from the catalogue route', async () => {
    api = mockApi({ 'GET /api/v1/carbon/activity-catalogue': catalogue });
    const r = renderWithProviders(
      <SessionProvider me={me('building_readonly_admin')}>
        <ReferencePanel kind="iso" />
      </SessionProvider>,
    );
    expect(await r.findByRole('row', { name: /Atık Bertarafı/ })).toBeVisible();
  });
});
