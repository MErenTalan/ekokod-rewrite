import { afterEach, describe, expect, it, vi } from 'vitest';

import { SessionProvider } from '@/lib/session/session-provider';
import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { BUILDING, me, overview } from './_fixture';
import { OverviewPanel } from './overview-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api.restore());

describe('OverviewPanel', () => {
  it('reads the building year and sends "see all" to the status tab', async () => {
    api = mockApi({ 'GET /api/v1/carbon/overview': overview });
    const go = vi.fn();
    const r = renderWithProviders(
      <SessionProvider me={me('company_admin')}>
        <OverviewPanel buildingId={BUILDING} go={go} />
      </SessionProvider>,
    );
    expect(await r.findByText('Toplam karbon ayak izi')).toBeVisible();
    const url = new URL(api.calls[0].url);
    expect(url.searchParams.get('building_id')).toBe(BUILDING);
    expect(url.searchParams.get('year')).toMatch(/^\d{4}$/);
    await r.user.click(r.getByRole('button', { name: 'Tümünü gör' }));
    expect(go).toHaveBeenCalledWith('status');
  });
});
